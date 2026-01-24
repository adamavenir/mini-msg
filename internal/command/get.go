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

// NewGetCmd creates the get command.
func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [path]",
		Short: "Get messages from room, thread, or path",
		Long: `Get messages from various sources.

Paths:
  fray get                    Room + notifications (default, requires --as)
  fray get meta               Project meta thread
  fray get opus/notes         Agent's notes thread
  fray get design-thread      Specific thread by name
  fray get notifs             Notifications only (@mentions + followed threads)
  fray get msg-abc            Specific message (shorthand: fray msg-abc)

Legacy (deprecated):
  fray get <agent>            Still works for agent-based room + mentions`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			last, _ := cmd.Flags().GetString("last")
			since, _ := cmd.Flags().GetString("since")
			before, _ := cmd.Flags().GetString("before")
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			all, _ := cmd.Flags().GetBool("all")
			room, _ := cmd.Flags().GetString("room")
			mentions, _ := cmd.Flags().GetString("mentions")
			unfiltered, _ := cmd.Flags().GetBool("unfiltered")
			archived, _ := cmd.Flags().GetBool("archived")
			hideEvents, _ := cmd.Flags().GetBool("hide-events")
			showEvents, _ := cmd.Flags().GetBool("show-events")
			showAllMessages, _ := cmd.Flags().GetBool("show-all")
			asRef, _ := cmd.Flags().GetString("as")
			if showEvents {
				hideEvents = false
			}

			projectName := GetProjectName(ctx.Project.Root)
			var agentBases map[string]struct{}
			if !ctx.JSONMode {
				agentBases, err = db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			// Determine what we're getting
			var target string
			if len(args) > 0 {
				target = args[0]
			}

			// Handle --session flag: filter messages by session ID
			sessionFilter, _ := cmd.Flags().GetString("session")
			if sessionFilter != "" {
				limitVal := 0
				if last != "" {
					var err error
					limitVal, err = strconv.Atoi(last)
					if err != nil {
						return writeCommandError(cmd, fmt.Errorf("invalid --last value"))
					}
				}
				messages, err := db.GetMessagesBySession(ctx.DB, sessionFilter, limitVal)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if hideEvents {
					messages = filterEventMessages(messages)
				}

				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(messages)
				}

				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Session %s:\n\n", sessionFilter)
				if len(messages) == 0 {
					fmt.Fprintln(out, "No messages from this session")
					return nil
				}
				lines := FormatMessageListAccordion(messages, AccordionOptions{
					ShowAll:     showAllMessages,
					ProjectName: projectName,
					AgentBases:  agentBases,
				})
				for _, line := range lines {
					fmt.Fprintln(out, line)
				}
				return nil
			}

			// Handle special path: "notifs"
			if target == "notifs" {
				return getNotifications(cmd, ctx, asRef, projectName, agentBases, showAllMessages)
			}

			// Try to resolve as thread path first
			if target != "" && !strings.HasPrefix(target, "msg-") {
				thread, err := resolveThreadRef(ctx.DB, target)
				if err == nil && thread != nil {
					pinnedOnly, _ := cmd.Flags().GetBool("pinned")
					anchorsOnly, _ := cmd.Flags().GetBool("anchors")
					byAgent, _ := cmd.Flags().GetString("by")
					withText, _ := cmd.Flags().GetString("with")
					reactionsOnly, _ := cmd.Flags().GetBool("reactions")
					if anchorsOnly {
						return getThreadAnchors(cmd, ctx, thread, projectName, agentBases)
					}
					return getThread(cmd, ctx, thread, last, since, showAllMessages, projectName, agentBases, hideEvents, pinnedOnly, byAgent, withText, reactionsOnly)
				}
			}

			// Try to resolve as message ID
			if target != "" && (strings.HasPrefix(target, "msg-") || len(target) <= 12) {
				msg, err := resolveMessageRef(ctx.DB, target)
				if err == nil && msg != nil {
					return getMessage(cmd, ctx, msg, projectName, agentBases)
				}
			}

			// Query mode when using explicit range/limit flags
			isQueryMode := (last != "" && len(args) == 0) || since != "" || before != "" || from != "" || to != "" || all

			// Legacy: try to resolve as agent ID for backward compatibility
			var resolvedAgentID string
			if target != "" {
				resolvedAgentID, err = resolveAgentRef(ctx, target)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("unknown path, thread, or agent: %s", target))
				}
			} else if asRef != "" {
				resolvedAgentID, err = resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			var filter *types.Filter
			if resolvedAgentID != "" && !unfiltered {
				filter, err = db.GetFilter(ctx.DB, resolvedAgentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if isQueryMode {
				var options types.MessageQueryOptions
				options.Filter = filter
				options.IncludeArchived = archived

				if all {
					// no limits
				} else if since != "" || before != "" || from != "" || to != "" {
					if since != "" && from != "" {
						return writeCommandError(cmd, fmt.Errorf("use --since or --from, not both"))
					}
					if before != "" && to != "" {
						return writeCommandError(cmd, fmt.Errorf("use --before or --to, not both"))
					}

					start := since
					if start == "" {
						start = from
					}
					end := before
					if end == "" {
						end = to
					}

					if start != "" {
						cursor, err := core.ParseTimeExpression(ctx.DB, start, "since")
						if err != nil {
							return writeCommandError(cmd, err)
						}
						options.Since = cursor
					}
					if end != "" {
						cursor, err := core.ParseTimeExpression(ctx.DB, end, "before")
						if err != nil {
							return writeCommandError(cmd, err)
						}
						options.Before = cursor
					}
				} else {
					limit, err := strconv.Atoi(last)
					if err != nil {
						return writeCommandError(cmd, fmt.Errorf("invalid --last value"))
					}
					options.Limit = limit
				}

				messages, err := db.GetMessages(ctx.DB, &options)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if hideEvents {
					messages = filterEventMessages(messages)
				}

				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(messages)
				}

				out := cmd.OutOrStdout()
				if len(messages) == 0 {
					fmt.Fprintln(out, "No messages")
					return nil
				}
				lines := FormatMessageListAccordion(messages, AccordionOptions{
					ShowAll:     showAllMessages,
					ProjectName: projectName,
					AgentBases:  agentBases,
				})
				for _, line := range lines {
					fmt.Fprintln(out, line)
				}
				return nil
			}

			if resolvedAgentID != "" {
				mentionsLimit := parseOptionalInt(mentions, 3)

				agentBase := resolvedAgentID
				if strings.Contains(resolvedAgentID, ".") {
					idx := strings.LastIndex(resolvedAgentID, ".")
					agentBase = resolvedAgentID[:idx]
				}

				// Get watermark for this agent
				watermark, err := db.GetReadTo(ctx.DB, agentBase, "room")
				if err != nil {
					return writeCommandError(cmd, err)
				}

				var roomMessages []types.Message
				roomCap := parseOptionalInt(room, 10)
				if last != "" {
					// Explicit --last flag: use that limit
					roomLimit, err := strconv.Atoi(last)
					if err != nil {
						return writeCommandError(cmd, fmt.Errorf("invalid --last value"))
					}
					roomMessages, err = db.GetMessages(ctx.DB, &types.MessageQueryOptions{Limit: roomLimit, Filter: filter, IncludeArchived: archived})
				} else if watermark != nil {
					// Has watermark: get only unread messages (since watermark), capped
					roomMessages, err = db.GetMessages(ctx.DB, &types.MessageQueryOptions{
						Since:           &types.MessageCursor{GUID: watermark.MessageGUID, TS: watermark.MessageTS},
						Limit:           roomCap,
						Filter:          filter,
						IncludeArchived: archived,
					})
				} else {
					// No watermark: check for ghost cursor, else use last N
					ghostCursor, _ := db.GetGhostCursor(ctx.DB, agentBase, "room")
					if ghostCursor != nil {
						msg, msgErr := db.GetMessage(ctx.DB, ghostCursor.MessageGUID)
						if msgErr == nil && msg != nil {
							roomMessages, err = db.GetMessages(ctx.DB, &types.MessageQueryOptions{
								Since:           &types.MessageCursor{GUID: msg.ID, TS: msg.TS},
								Limit:           roomCap,
								Filter:          filter,
								IncludeArchived: archived,
							})
							// Auto-clear ghost cursor after first use (one-time handoff)
							_ = db.DeleteGhostCursor(ctx.DB, agentBase, "room")
							_ = db.AppendCursorClear(ctx.Project.DBPath, agentBase, "room", time.Now().UnixMilli())
						}
					}
					if roomMessages == nil {
						roomMessages, err = db.GetMessages(ctx.DB, &types.MessageQueryOptions{Limit: roomCap, Filter: filter, IncludeArchived: archived})
					}
				}
				if err != nil {
					return writeCommandError(cmd, err)
				}
				roomMessages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, roomMessages)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if hideEvents {
					roomMessages = filterEventMessages(roomMessages)
				}

				// Check ghost cursor for session-aware unread logic
				allHomes := ""
				mentionOpts := &types.MessageQueryOptions{
					Limit:                 mentionsLimit + len(roomMessages),
					IncludeArchived:       archived,
					IncludeRepliesToAgent: agentBase,
					AgentPrefix:           agentBase,
					Home:                  &allHomes,
				}

				useGhostCursorBoundary := false
				var mentionGhostCursor *types.GhostCursor
				mentionGhostCursor, _ = db.GetGhostCursor(ctx.DB, agentBase, "room")
				if mentionGhostCursor != nil && mentionGhostCursor.SessionAckAt == nil {
					// Ghost cursor exists and not yet acked this session
					msg, msgErr := db.GetMessage(ctx.DB, mentionGhostCursor.MessageGUID)
					if msgErr == nil && msg != nil {
						mentionOpts.Since = &types.MessageCursor{GUID: msg.ID, TS: msg.TS}
						useGhostCursorBoundary = true
					}
				}
				// Fall back to watermark-based boundary if no ghost cursor
				if !useGhostCursorBoundary {
					mentionWatermark, _ := db.GetReadTo(ctx.DB, agentBase, "mentions")
					if mentionWatermark != nil {
						mentionOpts.Since = &types.MessageCursor{GUID: mentionWatermark.MessageGUID, TS: mentionWatermark.MessageTS}
					} else {
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
				if hideEvents {
					mentionMessages = filterEventMessages(mentionMessages)
				}

				roomIDs := map[string]struct{}{}
				for _, msg := range roomMessages {
					roomIDs[msg.ID] = struct{}{}
				}

				filtered := make([]types.Message, 0, len(mentionMessages))
				for _, msg := range mentionMessages {
					if _, ok := roomIDs[msg.ID]; ok {
						continue
					}
					filtered = append(filtered, msg)
					if len(filtered) == mentionsLimit {
						break
					}
				}

				if len(filtered) > 0 {
					ids := make([]string, 0, len(filtered))
					for _, msg := range filtered {
						ids = append(ids, msg.ID)
					}
					if err := db.MarkMessagesRead(ctx.DB, ids, agentBase); err != nil {
						return writeCommandError(cmd, err)
					}
					// Also set mentions watermark for durable read state
					lastMentionMsg := filtered[len(filtered)-1]
					_ = db.SetReadTo(ctx.DB, agentBase, "mentions", lastMentionMsg.ID, lastMentionMsg.TS)
				}

				// Ack ghost cursor if we used it as boundary (first view this session)
				if useGhostCursorBoundary && mentionGhostCursor != nil {
					now := time.Now().Unix()
					_ = db.AckGhostCursor(ctx.DB, agentBase, "room", now)
				}

				// Set watermark to the latest message viewed
				if len(roomMessages) > 0 {
					lastMsg := roomMessages[len(roomMessages)-1]
					if err := db.SetReadTo(ctx.DB, agentBase, "room", lastMsg.ID, lastMsg.TS); err != nil {
						return writeCommandError(cmd, err)
					}
				}

				if ctx.JSONMode {
					readTo, _ := db.GetReadToForHome(ctx.DB, "room")
					threadHints, _ := getThreadActivityHints(ctx, agentBase)
					// Convert to compact JSON format to save tokens
					compactThreads := make([]ThreadActivityHintJSON, len(threadHints))
					for i, h := range threadHints {
						compactThreads[i] = h.toJSON()
					}
					payload := map[string]any{
						"project":       projectName,
						"room_messages": roomMessages,
						"mentions":      filtered,
						"read_to":       readTo,
						"threads":       compactThreads,
					}
					return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
				}

				out := cmd.OutOrStdout()
				if len(roomMessages) == 0 {
					fmt.Fprintln(out, "ROOM: (no messages yet)")
				} else {
					fmt.Fprintln(out, "ROOM:")
					lines := FormatMessageListAccordion(roomMessages, AccordionOptions{
						ShowAll:     showAllMessages,
						ProjectName: projectName,
						AgentBases:  agentBases,
					})
					for _, line := range lines {
						fmt.Fprintln(out, line)
					}
				}

				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "---")
				fmt.Fprintln(out, "")

				if len(filtered) == 0 {
					fmt.Fprintf(out, "@%s: (no additional mentions)\n", agentBase)
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
							for _, reactionLine := range formatReactionEvents(msg) {
								fmt.Fprintf(out, "  %s\n", reactionLine)
							}
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
						fmt.Fprintf(out, "@%s: (no additional mentions)\n", agentBase)
					}
				}

				// Show thread activity hints
				threadHints, _ := getThreadActivityHints(ctx, agentBase)
				if len(threadHints) > 0 {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "---")
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Threads:")
					for _, hint := range threadHints {
						fmt.Fprintln(out, formatThreadHint(hint))
					}
				}

				claims, err := db.GetAllClaims(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				if len(claims) > 0 {
					claimsByAgent := make(map[string][]string)
					for _, claim := range claims {
						pattern := claim.Pattern
						if claim.ClaimType != types.ClaimTypeFile {
							pattern = fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern)
						}
						claimsByAgent[claim.AgentID] = append(claimsByAgent[claim.AgentID], pattern)
					}

					claimParts := make([]string, 0, len(claimsByAgent))
					for agentID, patterns := range claimsByAgent {
						claimParts = append(claimParts, fmt.Sprintf("@%s (%s)", agentID, strings.Join(patterns, ", ")))
					}

					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "---")
					fmt.Fprintf(out, "Active claims: %s\n", strings.Join(claimParts, ", "))
				}

				return nil
			}

			return writeCommandError(cmd, fmt.Errorf("usage: fray get <agent>           Unread room + @mentions (default)\n       fray get <agent> --last <n> Last N room messages\n       fray get --last <n>         Last N messages (no agent)\n       fray get --since <guid>     Messages after GUID\n       fray get --all              All messages"))
		},
	}

	cmd.Flags().String("last", "", "show last N messages")
	cmd.Flags().String("since", "", "show messages after time or GUID")
	cmd.Flags().String("before", "", "show messages before time or GUID")
	cmd.Flags().String("from", "", "range start (time or GUID)")
	cmd.Flags().String("to", "", "range end (time or GUID)")
	cmd.Flags().Bool("all", false, "show all messages")
	cmd.Flags().String("room", "10", "number of room messages in combined view")
	cmd.Flags().String("mentions", "3", "number of @mentions in combined view")
	cmd.Flags().Bool("unfiltered", false, "bypass saved filter, show all messages")
	cmd.Flags().Bool("archived", false, "include archived messages")
	cmd.Flags().Bool("hide-events", false, "hide event messages")
	cmd.Flags().Bool("show-events", false, "show event messages")
	cmd.Flags().Bool("show-all", false, "disable accordion, show all messages fully")
	cmd.Flags().String("as", "", "agent identity (uses FRAY_AGENT_ID if not set)")
	cmd.Flags().Bool("replies", false, "show message with reply chain")

	// Within-thread filters
	cmd.Flags().Bool("pinned", false, "show only pinned messages (threads only)")
	cmd.Flags().Bool("anchors", false, "show only anchor messages from child threads")
	cmd.Flags().String("by", "", "filter messages by agent")
	cmd.Flags().String("with", "", "filter messages containing text")
	cmd.Flags().Bool("reactions", false, "show only messages with reactions")
	cmd.Flags().String("session", "", "filter messages by session ID")

	return cmd
}

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

func parseOptionalInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// ThreadActivityHint represents unread activity in a subscribed thread.
type ThreadActivityHint struct {
	ThreadGUID  string
	ThreadName  string
	NewCount    int
	LastMessage *types.Message `json:"-"` // Exclude from JSON to save tokens
	MustRead    bool           // from ghost cursor
}

// ThreadActivityHintJSON is a compact version for JSON output.
type ThreadActivityHintJSON struct {
	ThreadGUID   string `json:"thread_guid"`
	ThreadName   string `json:"thread_name"`
	NewCount     int    `json:"new_count"`
	LastMsgID    string `json:"last_msg_id,omitempty"`
	LastMsgFrom  string `json:"last_msg_from,omitempty"`
	LastMsgShort string `json:"last_msg_short,omitempty"` // First 80 chars
	MustRead     bool   `json:"must_read,omitempty"`
}

// toJSON converts a ThreadActivityHint to its compact JSON representation.
func (h *ThreadActivityHint) toJSON() ThreadActivityHintJSON {
	result := ThreadActivityHintJSON{
		ThreadGUID: h.ThreadGUID,
		ThreadName: h.ThreadName,
		NewCount:   h.NewCount,
		MustRead:   h.MustRead,
	}
	if h.LastMessage != nil {
		result.LastMsgID = h.LastMessage.ID
		result.LastMsgFrom = h.LastMessage.FromAgent
		body := h.LastMessage.Body
		if len(body) > 80 {
			body = body[:77] + "..."
		}
		result.LastMsgShort = body
	}
	return result
}

// getThreadActivityHints returns activity hints for subscribed threads.
func getThreadActivityHints(ctx *CommandContext, agentID string) ([]ThreadActivityHint, error) {
	// Get subscribed threads (excluding muted)
	threads, err := db.GetThreads(ctx.DB, &types.ThreadQueryOptions{
		SubscribedAgent: &agentID,
	})
	if err != nil {
		return nil, err
	}
	if len(threads) == 0 {
		return nil, nil
	}

	// Get muted threads to filter
	mutedGUIDs, err := db.GetMutedThreadGUIDs(ctx.DB, agentID)
	if err != nil {
		mutedGUIDs = map[string]bool{}
	}

	var hints []ThreadActivityHint
	for _, thread := range threads {
		if mutedGUIDs[thread.GUID] {
			continue
		}

		// Determine read position - check ghost cursor first, then read_to
		var sinceCursor *types.MessageCursor
		mustRead := false

		ghostCursor, _ := db.GetGhostCursor(ctx.DB, agentID, thread.GUID)
		if ghostCursor != nil {
			msg, err := db.GetMessage(ctx.DB, ghostCursor.MessageGUID)
			if err == nil && msg != nil {
				sinceCursor = &types.MessageCursor{GUID: msg.ID, TS: msg.TS}
				mustRead = ghostCursor.MustRead
			}
		}

		if sinceCursor == nil {
			readTo, _ := db.GetReadTo(ctx.DB, agentID, thread.GUID)
			if readTo != nil {
				sinceCursor = &types.MessageCursor{GUID: readTo.MessageGUID, TS: readTo.MessageTS}
			}
		}

		// Get messages since read position
		home := thread.GUID
		messages, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{
			Home:  &home,
			Since: sinceCursor,
		})
		if err != nil {
			continue
		}

		if len(messages) == 0 {
			continue
		}

		hint := ThreadActivityHint{
			ThreadGUID:  thread.GUID,
			ThreadName:  thread.Name,
			NewCount:    len(messages),
			LastMessage: &messages[len(messages)-1],
			MustRead:    mustRead,
		}
		hints = append(hints, hint)
	}

	return hints, nil
}

// formatThreadHint formats a single thread activity hint.
func formatThreadHint(hint ThreadActivityHint) string {
	suffix := ""
	if hint.MustRead {
		suffix = " [must-read]"
	}

	// Extract first line, truncate to ~30 chars
	context := ""
	if hint.LastMessage != nil {
		body := strings.TrimSpace(hint.LastMessage.Body)
		// Get first line only
		if idx := strings.Index(body, "\n"); idx > 0 {
			body = body[:idx]
		}
		body = strings.TrimSpace(body)
		if len(body) > 30 {
			body = body[:27] + "..."
		}
		if body != "" {
			context = fmt.Sprintf(" (last: @%s on %s)", hint.LastMessage.FromAgent, body)
		}
	}

	return fmt.Sprintf("  %s: %d new%s%s", hint.ThreadName, hint.NewCount, context, suffix)
}

// isDirectMention checks if the message body starts with @agent (direct address).
func isDirectMention(body, agentBase string) bool {
	body = strings.TrimSpace(body)
	// Check for @agent or @agent.* at start
	if strings.HasPrefix(body, "@"+agentBase+" ") || strings.HasPrefix(body, "@"+agentBase+"\n") {
		return true
	}
	if strings.HasPrefix(body, "@"+agentBase+".") {
		// Could be @agent.1 or @agent.something
		return true
	}
	if body == "@"+agentBase {
		return true
	}
	return false
}
