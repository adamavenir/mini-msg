package command

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// getThread displays messages from a thread.
func getThread(cmd *cobra.Command, ctx *CommandContext, thread *types.Thread, last, since string, showAll bool, projectName string, agentBases map[string]struct{}, hideEvents bool, pinnedOnly bool, byAgent, withText string, reactionsOnly bool) error {
	var messages []types.Message
	var err error

	// Handle --pinned: use dedicated query for pinned messages
	if pinnedOnly {
		messages, err = db.GetPinnedMessages(ctx.DB, thread.GUID)
		if err != nil {
			return writeCommandError(cmd, err)
		}
	} else {
		messages, err = db.GetThreadMessages(ctx.DB, thread.GUID)
		if err != nil {
			return writeCommandError(cmd, err)
		}
	}
	messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	messages = filterDeletedMessages(messages)

	// Apply --since filter
	if since != "" {
		cursor, err := core.ParseTimeExpression(ctx.DB, since, "since")
		if err != nil {
			return writeCommandError(cmd, err)
		}
		var filtered []types.Message
		for _, msg := range messages {
			if cursor.GUID != "" && msg.ID > cursor.GUID {
				filtered = append(filtered, msg)
			} else if cursor.GUID == "" && msg.TS > cursor.TS {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	// Apply --by filter (filter by agent)
	if byAgent != "" {
		agentID := ResolveAgentRef(byAgent, ctx.ProjectConfig)
		var filtered []types.Message
		for _, msg := range messages {
			if msg.FromAgent == agentID || strings.HasPrefix(msg.FromAgent, agentID+".") {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	// Apply --with filter (text search)
	if withText != "" {
		searchLower := strings.ToLower(withText)
		var filtered []types.Message
		for _, msg := range messages {
			if strings.Contains(strings.ToLower(msg.Body), searchLower) {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	// Apply --reactions filter
	if reactionsOnly {
		var filtered []types.Message
		for _, msg := range messages {
			if len(msg.Reactions) > 0 {
				filtered = append(filtered, msg)
			}
		}
		messages = filtered
	}

	// Apply --last limit
	if last != "" {
		limit, err := strconv.Atoi(last)
		if err != nil {
			return writeCommandError(cmd, fmt.Errorf("invalid --last value: %s", last))
		}
		if limit > 0 && len(messages) > limit {
			messages = messages[len(messages)-limit:]
		}
	}

	if hideEvents {
		messages = filterEventMessages(messages)
	}

	path, _ := buildThreadPath(ctx.DB, thread)

	if ctx.JSONMode {
		payload := map[string]any{
			"thread":   thread,
			"path":     path,
			"messages": messages,
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Thread %s (%s)\n\n", path, thread.GUID)

	if len(messages) == 0 {
		fmt.Fprintln(out, "No messages in thread")
		return nil
	}

	quotedMsgs := CollectQuotedMessages(ctx.DB, messages)

	// Build set of pinned message GUIDs for accordion expansion
	pinnedGUIDs := make(map[string]bool)
	if pinnedMsgs, err := db.GetPinnedMessages(ctx.DB, thread.GUID); err == nil {
		for _, pm := range pinnedMsgs {
			pinnedGUIDs[pm.ID] = true
		}
	}

	lines := FormatMessageListAccordion(messages, AccordionOptions{
		ShowAll:     showAll,
		ProjectName: projectName,
		AgentBases:  agentBases,
		QuotedMsgs:  quotedMsgs,
		PinnedGUIDs: pinnedGUIDs,
	})
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return nil
}

// getThreadAnchors displays anchor messages from child threads.
func getThreadAnchors(cmd *cobra.Command, ctx *CommandContext, thread *types.Thread, projectName string, agentBases map[string]struct{}) error {
	// Get child threads
	childThreads, err := db.GetThreads(ctx.DB, &types.ThreadQueryOptions{
		ParentThread: &thread.GUID,
	})
	if err != nil {
		return writeCommandError(cmd, err)
	}

	type anchorEntry struct {
		Thread  types.Thread
		Path    string
		Message *types.Message
	}
	var entries []anchorEntry

	for _, child := range childThreads {
		path, _ := buildThreadPath(ctx.DB, &child)
		var msg *types.Message
		if child.AnchorMessageGUID != nil {
			msg, _ = db.GetMessage(ctx.DB, *child.AnchorMessageGUID)
		}
		entries = append(entries, anchorEntry{
			Thread:  child,
			Path:    path,
			Message: msg,
		})
	}

	parentPath, _ := buildThreadPath(ctx.DB, thread)

	if ctx.JSONMode {
		type jsonEntry struct {
			Thread  types.Thread   `json:"thread"`
			Path    string         `json:"path"`
			Message *types.Message `json:"anchor,omitempty"`
		}
		jsonEntries := make([]jsonEntry, len(entries))
		for i, e := range entries {
			jsonEntries[i] = jsonEntry{Thread: e.Thread, Path: e.Path, Message: e.Message}
		}
		payload := map[string]any{
			"parent":  thread,
			"path":    parentPath,
			"anchors": jsonEntries,
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Anchors in %s (%s)\n\n", parentPath, thread.GUID)

	if len(entries) == 0 {
		fmt.Fprintln(out, "No child threads")
		return nil
	}

	for _, entry := range entries {
		if entry.Message != nil {
			fmt.Fprintf(out, "## %s\n", entry.Path)
			fmt.Fprintln(out, FormatMessageFull(*entry.Message, projectName, agentBases))
			fmt.Fprintln(out)
		} else {
			fmt.Fprintf(out, "## %s (no anchor)\n\n", entry.Path)
		}
	}

	return nil
}

// getMessage displays a single message.
func getMessage(cmd *cobra.Command, ctx *CommandContext, msg *types.Message, projectName string, agentBases map[string]struct{}) error {
	showReplies, _ := cmd.Flags().GetBool("replies")

	if ctx.JSONMode {
		if showReplies {
			replies, _ := db.GetReplies(ctx.DB, msg.ID)
			payload := map[string]any{
				"message": msg,
				"replies": replies,
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(msg)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, FormatMessageFull(*msg, projectName, agentBases))

	if showReplies {
		replies, err := db.GetReplies(ctx.DB, msg.ID)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		if len(replies) > 0 {
			fmt.Fprintln(out, "\nReplies:")
			for _, reply := range replies {
				fmt.Fprintln(out, FormatMessage(reply, projectName, agentBases))
			}
		}
	}
	return nil
}

// getNotifications displays notifications for an agent.
func getNotifications(cmd *cobra.Command, ctx *CommandContext, asRef, projectName string, agentBases map[string]struct{}, showAll bool) error {
	agentID, err := resolveSubscriptionAgent(ctx, asRef)
	if err != nil {
		return writeCommandError(cmd, fmt.Errorf("--as is required for notifications"))
	}

	agentBase := agentID
	if strings.Contains(agentID, ".") {
		idx := strings.LastIndex(agentID, ".")
		agentBase = agentID[:idx]
	}

	// Get @mentions
	allHomes := ""
	mentionOpts := &types.MessageQueryOptions{
		Limit:                 20,
		IncludeRepliesToAgent: agentBase,
		AgentPrefix:           agentBase,
		Home:                  &allHomes,
	}

	// Check ghost cursor for session-aware unread logic
	useGhostCursorBoundary := false
	mentionGhostCursor, _ := db.GetGhostCursor(ctx.DB, agentBase, "room")
	if mentionGhostCursor != nil && mentionGhostCursor.SessionAckAt == nil {
		msg, msgErr := db.GetMessage(ctx.DB, mentionGhostCursor.MessageGUID)
		if msgErr == nil && msg != nil {
			mentionOpts.Since = &types.MessageCursor{GUID: msg.ID, TS: msg.TS}
			useGhostCursorBoundary = true
		}
	}
	// Fall back to watermark-based boundary if no ghost cursor
	// This handles users and agents without ghost cursors
	if !useGhostCursorBoundary {
		mentionWatermark, _ := db.GetReadTo(ctx.DB, agentBase, "mentions")
		if mentionWatermark != nil {
			mentionOpts.Since = &types.MessageCursor{GUID: mentionWatermark.MessageGUID, TS: mentionWatermark.MessageTS}
		} else {
			// No watermark either - use UnreadOnly as last resort
			mentionOpts.UnreadOnly = true
		}
	}

	mentionMessages, err := db.GetMessagesWithMention(ctx.DB, agentBase, mentionOpts)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	mentionMessages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, mentionMessages)
	if err != nil {
		return writeCommandError(cmd, err)
	}

	// Filter out self-mentions
	filtered := make([]types.Message, 0, len(mentionMessages))
	for _, msg := range mentionMessages {
		parsed, err := core.ParseAgentID(msg.FromAgent)
		if err != nil {
			filtered = append(filtered, msg)
			continue
		}
		if parsed.Base != agentBase {
			filtered = append(filtered, msg)
		}
	}

	// Get thread activity hints
	threadHints, _ := getThreadActivityHints(ctx, agentBase)

	if ctx.JSONMode {
		// Convert to compact JSON format to save tokens
		compactThreads := make([]ThreadActivityHintJSON, len(threadHints))
		for i, h := range threadHints {
			compactThreads[i] = h.toJSON()
		}
		payload := map[string]any{
			"mentions": filtered,
			"threads":  compactThreads,
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Notifications for @%s\n\n", agentBase)

	if len(filtered) == 0 {
		fmt.Fprintln(out, "@mentions: (none)")
	} else {
		// Categorize mentions: direct vs FYI vs stale
		now := time.Now().Unix()
		staleThreshold := now - 2*60*60 // 2 hours
		var direct, fyi, stale []types.Message
		for _, msg := range filtered {
			if msg.TS < staleThreshold {
				stale = append(stale, msg)
			} else if isDirectMention(msg.Body, agentBase) {
				direct = append(direct, msg)
			} else {
				fyi = append(fyi, msg)
			}
		}

		if len(direct) > 0 {
			fmt.Fprintf(out, "Recent @%s:\n", agentBase)
			for _, msg := range direct {
				fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
			}
		}

		if len(fyi) > 0 {
			if len(direct) > 0 {
				fmt.Fprintln(out, "")
			}
			fmt.Fprintln(out, "You were FYI'd here:")
			for _, msg := range fyi {
				fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
			}
		}

		if len(stale) > 0 {
			if len(direct) > 0 || len(fyi) > 0 {
				fmt.Fprintln(out, "")
			}
			fmt.Fprintf(out, "%d likely stale (>2h):\n", len(stale))
			for _, msg := range stale {
				fmt.Fprintln(out, FormatMessagePreview(msg, projectName))
			}
		}

		if len(direct) == 0 && len(fyi) == 0 && len(stale) == 0 {
			fmt.Fprintln(out, "@mentions: (none)")
		}
	}

	if len(threadHints) > 0 {
		fmt.Fprintln(out, "\nThreads:")
		for _, hint := range threadHints {
			fmt.Fprintln(out, formatThreadHint(hint))
		}
	}

	// Mark messages as read and update watermark
	if len(filtered) > 0 {
		ids := make([]string, 0, len(filtered))
		for _, msg := range filtered {
			ids = append(ids, msg.ID)
		}
		if err := db.MarkMessagesRead(ctx.DB, ids, agentBase); err != nil {
			return writeCommandError(cmd, err)
		}
		// Also set watermark to the latest mention for durable read state
		// This survives DB rebuilds unlike read receipts
		lastMsg := filtered[len(filtered)-1]
		_ = db.SetReadTo(ctx.DB, agentBase, "mentions", lastMsg.ID, lastMsg.TS)
	}

	// Ack ghost cursor if we used it as boundary
	if useGhostCursorBoundary && mentionGhostCursor != nil {
		now := time.Now().Unix()
		_ = db.AckGhostCursor(ctx.DB, agentBase, "room", now)
	}

	return nil
}
