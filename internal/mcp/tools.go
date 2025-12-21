package mcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContext struct {
	AgentID string
	DB      *sql.DB
	Project core.Project
}

func toolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "mm_post",
			Description: "Post a message to the mm room. Use @mentions to direct messages to specific agents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"body": map[string]any{
						"type":        "string",
						"description": "Message body. Supports @mentions like @alice.1 or @all for broadcast.",
					},
				},
				"required": []string{"body"},
			},
		},
		{
			Name:        "mm_get",
			Description: "Get recent room messages. Use for catching up on conversation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since": map[string]any{
						"type":        "string",
						"description": "Get messages after this GUID (for polling new messages)",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Maximum number of messages to return (default: 10)",
					},
				},
			},
		},
		{
			Name:        "mm_mentions",
			Description: "Get messages that mention me. Use to check for messages directed at you.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"since": map[string]any{
						"type":        "string",
						"description": "Get mentions after this GUID (for polling)",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Maximum number of mentions to return (default: 10)",
					},
				},
			},
		},
		{
			Name:        "mm_here",
			Description: "List active agents in the room. See who is available to collaborate.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "mm_whoami",
			Description: "Show my agent identity. Returns your agent ID and status.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "mm_claim",
			Description: "Claim resources (files, bd issues, GitHub issues) to prevent collision with other agents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"files": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "File paths or glob patterns to claim (e.g., [\"src/auth.ts\", \"lib/*.ts\"])",
					},
					"bd": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Beads issue IDs to claim (e.g., [\"xyz-123\"])",
					},
					"issues": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "GitHub issue numbers to claim (e.g., [\"456\"])",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Reason for claim (optional)",
					},
					"ttl_minutes": map[string]any{
						"type":        "number",
						"description": "Time to live in minutes (optional, default: no expiry)",
					},
				},
			},
		},
		{
			Name:        "mm_clear",
			Description: "Clear claims. Clear all your claims or specific ones.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file": map[string]any{
						"type":        "string",
						"description": "Specific file claim to clear (optional)",
					},
					"bd": map[string]any{
						"type":        "string",
						"description": "Specific bd issue claim to clear (optional)",
					},
					"issue": map[string]any{
						"type":        "string",
						"description": "Specific GitHub issue claim to clear (optional)",
					},
				},
			},
		},
		{
			Name:        "mm_claims",
			Description: "List active claims. Shows who has claimed what resources.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent": map[string]any{
						"type":        "string",
						"description": "Filter by agent ID (optional, defaults to all agents)",
					},
				},
			},
		},
		{
			Name:        "mm_status",
			Description: "Update your status with optional resource claims. Sets your status and claims resources in one operation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "Status message (your current task/focus)",
					},
					"files": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "File paths or glob patterns to claim",
					},
					"bd": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Beads issue IDs to claim",
					},
					"issues": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "GitHub issue numbers to claim",
					},
					"ttl_minutes": map[string]any{
						"type":        "number",
						"description": "Time to live in minutes for claims",
					},
					"clear": map[string]any{
						"type":        "boolean",
						"description": "Clear all claims and reset status",
					},
				},
			},
		},
	}
}

func handleToolCall(ctx ToolContext, name string, args json.RawMessage) (ToolResult, error) {
	switch name {
	case "mm_post":
		var params struct {
			Body string `json:"body"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handlePost(ctx, params.Body)
	case "mm_get":
		var params struct {
			Since string `json:"since"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handleGet(ctx, params.Since, params.Limit)
	case "mm_mentions":
		var params struct {
			Since string `json:"since"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handleMentions(ctx, params.Since, params.Limit)
	case "mm_here":
		return handleHere(ctx)
	case "mm_whoami":
		return handleWhoami(ctx)
	case "mm_claim":
		var params struct {
			Files      []string `json:"files"`
			BD         []string `json:"bd"`
			Issues     []string `json:"issues"`
			Reason     string   `json:"reason"`
			TTLMinutes int      `json:"ttl_minutes"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handleClaim(ctx, params.Files, params.BD, params.Issues, params.Reason, params.TTLMinutes)
	case "mm_clear":
		var params struct {
			File  string `json:"file"`
			BD    string `json:"bd"`
			Issue string `json:"issue"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handleClear(ctx, params.File, params.BD, params.Issue)
	case "mm_claims":
		var params struct {
			Agent string `json:"agent"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handleClaims(ctx, params.Agent)
	case "mm_status":
		var params struct {
			Message    string   `json:"message"`
			Files      []string `json:"files"`
			BD         []string `json:"bd"`
			Issues     []string `json:"issues"`
			TTLMinutes int      `json:"ttl_minutes"`
			Clear      bool     `json:"clear"`
		}
		if err := decodeArgs(args, &params); err != nil {
			return errorResult(err), nil
		}
		return handleStatus(ctx, params.Message, params.Files, params.BD, params.Issues, params.TTLMinutes, params.Clear)
	default:
		return ToolResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}}, IsError: true}, nil
	}
}

func decodeArgs(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func handlePost(ctx ToolContext, body string) (ToolResult, error) {
	if strings.TrimSpace(body) == "" {
		return textResult("Error: Message body cannot be empty"), nil
	}

	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return errorResult(err), nil
	}
	if agent == nil {
		return textResult(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID)), nil
	}
	if agent.LeftAt != nil {
		return textResult(fmt.Sprintf("Error: Agent %s has left the room", ctx.AgentID)), nil
	}

	bases, err := db.GetAgentBases(ctx.DB)
	if err != nil {
		return errorResult(err), nil
	}
	mentions := core.ExtractMentions(body, bases)

	now := time.Now().Unix()
	created, err := db.CreateMessage(ctx.DB, types.Message{
		TS:        now,
		FromAgent: ctx.AgentID,
		Body:      strings.TrimSpace(body),
		Mentions:  mentions,
	})
	if err != nil {
		return errorResult(err), nil
	}
	_ = db.AppendMessage(ctx.Project.DBPath, created)

	updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
	_ = db.UpdateAgent(ctx.DB, ctx.AgentID, updates)

	mentionInfo := ""
	if len(mentions) > 0 {
		mentionInfo = fmt.Sprintf(" (mentioned: %s)", strings.Join(mentions, ", "))
	}
	return textResult(fmt.Sprintf("Posted message #%s%s", created.ID, mentionInfo)), nil
}

func handleGet(ctx ToolContext, since string, limit int) (ToolResult, error) {
	if limit <= 0 {
		limit = 10
	}
	options := &types.MessageQueryOptions{Limit: limit}
	if since != "" {
		options.SinceID = sanitizeMessageID(since)
	}

	messages, err := db.GetMessages(ctx.DB, options)
	if err != nil {
		return errorResult(err), nil
	}
	if len(messages) == 0 {
		qualifier := ""
		if since != "" {
			qualifier = fmt.Sprintf(" after message #%s", sanitizeMessageID(since))
		}
		return textResult(fmt.Sprintf("No messages%s", qualifier)), nil
	}

	formatted := formatMessages(messages)
	header := fmt.Sprintf("Recent messages (%d):", len(messages))
	if since != "" {
		header = fmt.Sprintf("Messages after #%s (%d):", sanitizeMessageID(since), len(messages))
	}
	return textResult(fmt.Sprintf("%s\n\n%s", header, formatted)), nil
}

func handleMentions(ctx ToolContext, since string, limit int) (ToolResult, error) {
	if limit <= 0 {
		limit = 10
	}

	prefix := ctx.AgentID
	if parsed, err := core.ParseAgentID(ctx.AgentID); err == nil {
		prefix = parsed.Base
	}
	options := &types.MessageQueryOptions{Limit: limit}
	if since != "" {
		options.SinceID = sanitizeMessageID(since)
	}

	messages, err := db.GetMessagesWithMention(ctx.DB, prefix, options)
	if err != nil {
		return errorResult(err), nil
	}
	if len(messages) == 0 {
		qualifier := ""
		if since != "" {
			qualifier = fmt.Sprintf(" after message #%s", sanitizeMessageID(since))
		}
		return textResult(fmt.Sprintf("No mentions%s", qualifier)), nil
	}

	formatted := formatMessages(messages)
	header := fmt.Sprintf("Recent mentions (%d):", len(messages))
	if since != "" {
		header = fmt.Sprintf("Mentions after #%s (%d):", sanitizeMessageID(since), len(messages))
	}
	return textResult(fmt.Sprintf("%s\n\n%s", header, formatted)), nil
}

func handleHere(ctx ToolContext) (ToolResult, error) {
	staleHours := 4
	if raw, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil {
		staleHours = parseStaleHours(raw, 4)
	}

	agents, err := db.GetActiveAgents(ctx.DB, staleHours)
	if err != nil {
		return errorResult(err), nil
	}
	if len(agents) == 0 {
		return textResult("No active agents"), nil
	}

	lines := make([]string, 0, len(agents))
	for _, agent := range agents {
		status := ""
		if agent.Status != nil && *agent.Status != "" {
			status = fmt.Sprintf(": \"%s\"", *agent.Status)
		}
		lines = append(lines, fmt.Sprintf("  %s%s", agent.AgentID, status))
	}
	return textResult(fmt.Sprintf("Active agents (%d):\n%s", len(agents), strings.Join(lines, "\n"))), nil
}

func handleWhoami(ctx ToolContext) (ToolResult, error) {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return errorResult(err), nil
	}
	if agent == nil {
		return textResult(fmt.Sprintf("Agent ID: %s (not found in database)", ctx.AgentID)), nil
	}

	activeStatus := "active"
	if agent.LeftAt != nil {
		activeStatus = "left"
	}
	statusLine := ""
	if agent.Status != nil && *agent.Status != "" {
		statusLine = fmt.Sprintf("\nStatus: %s", *agent.Status)
	}
	purposeLine := ""
	if agent.Purpose != nil && *agent.Purpose != "" {
		purposeLine = fmt.Sprintf("\nPurpose: %s", *agent.Purpose)
	}
	return textResult(fmt.Sprintf("Agent ID: %s\nActive: %s%s%s", ctx.AgentID, activeStatus, statusLine, purposeLine)), nil
}

func handleClaim(ctx ToolContext, files, bdClaims, issues []string, reason string, ttlMinutes int) (ToolResult, error) {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return errorResult(err), nil
	}
	if agent == nil {
		return textResult(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID)), nil
	}

	_, _ = db.PruneExpiredClaims(ctx.DB)

	var expiresAt *int64
	if ttlMinutes > 0 {
		value := time.Now().Unix() + int64(ttlMinutes*60)
		expiresAt = &value
	}

	claims := make([]types.ClaimInput, 0, len(files)+len(bdClaims)+len(issues))
	for _, pattern := range files {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeFile, Pattern: pattern})
	}
	for _, entry := range bdClaims {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeBD, Pattern: stripHash(entry)})
	}
	for _, entry := range issues {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeIssue, Pattern: stripHash(entry)})
	}

	if len(claims) == 0 {
		return textResult("Error: No claims specified. Provide files, bd, or issues."), nil
	}

	created := make([]types.Claim, 0, len(claims))
	var errors []string
	for _, claim := range claims {
		createdClaim, err := db.CreateClaim(ctx.DB, types.ClaimInput{
			AgentID:   ctx.AgentID,
			ClaimType: claim.ClaimType,
			Pattern:   claim.Pattern,
			Reason:    optionalString(reason),
			ExpiresAt: expiresAt,
		})
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		created = append(created, *createdClaim)
	}

	if len(created) > 0 {
		body := fmt.Sprintf("claimed: %s", buildClaimList(created))
		msg, err := db.CreateMessage(ctx.DB, types.Message{
			TS:        time.Now().Unix(),
			FromAgent: ctx.AgentID,
			Body:      body,
			Mentions:  []string{},
		})
		if err == nil {
			_ = db.AppendMessage(ctx.Project.DBPath, msg)
		}
	}

	result := fmt.Sprintf("Claimed %d resource", len(created))
	if len(created) != 1 {
		result += "s"
	}
	if len(created) > 0 {
		list := make([]string, 0, len(created))
		for _, claim := range created {
			if claim.ClaimType == types.ClaimTypeFile {
				list = append(list, claim.Pattern)
			} else {
				list = append(list, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
			}
		}
		result += ":\n  " + strings.Join(list, "\n  ")
	}
	if len(errors) > 0 {
		result += "\n\nErrors:\n  " + strings.Join(errors, "\n  ")
	}
	if expiresAt != nil {
		result += fmt.Sprintf("\n\nExpires in %d minutes", ttlMinutes)
	}

	return textResult(result), nil
}

func handleClear(ctx ToolContext, file, bdEntry, issue string) (ToolResult, error) {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return errorResult(err), nil
	}
	if agent == nil {
		return textResult(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID)), nil
	}

	cleared := 0
	var clearedItems []string

	if file != "" {
		if ok, _ := db.DeleteClaim(ctx.DB, types.ClaimTypeFile, file); ok {
			cleared++
			clearedItems = append(clearedItems, file)
		}
	}
	if bdEntry != "" {
		pattern := stripHash(bdEntry)
		if ok, _ := db.DeleteClaim(ctx.DB, types.ClaimTypeBD, pattern); ok {
			cleared++
			clearedItems = append(clearedItems, fmt.Sprintf("bd:%s", pattern))
		}
	}
	if issue != "" {
		pattern := stripHash(issue)
		if ok, _ := db.DeleteClaim(ctx.DB, types.ClaimTypeIssue, pattern); ok {
			cleared++
			clearedItems = append(clearedItems, fmt.Sprintf("issue:%s", pattern))
		}
	}

	if file == "" && bdEntry == "" && issue == "" {
		existing, err := db.GetClaimsByAgent(ctx.DB, ctx.AgentID)
		if err == nil {
			for _, claim := range existing {
				if claim.ClaimType == types.ClaimTypeFile {
					clearedItems = append(clearedItems, claim.Pattern)
				} else {
					clearedItems = append(clearedItems, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
				}
			}
		}
		count, err := db.DeleteClaimsByAgent(ctx.DB, ctx.AgentID)
		if err == nil {
			cleared = int(count)
		}
	}

	if cleared > 0 {
		body := fmt.Sprintf("cleared claims: %s", strings.Join(clearedItems, ", "))
		msg, err := db.CreateMessage(ctx.DB, types.Message{
			TS:        time.Now().Unix(),
			FromAgent: ctx.AgentID,
			Body:      body,
			Mentions:  []string{},
		})
		if err == nil {
			_ = db.AppendMessage(ctx.Project.DBPath, msg)
		}
		return textResult(fmt.Sprintf("Cleared %d claim", cleared) + pluralSuffix(cleared) + ":\n  " + strings.Join(clearedItems, "\n  ")), nil
	}

	return textResult("No claims to clear"), nil
}

func handleClaims(ctx ToolContext, agent string) (ToolResult, error) {
	var claims []types.Claim
	var err error
	if agent != "" {
		claims, err = db.GetClaimsByAgent(ctx.DB, agent)
	} else {
		claims, err = db.GetAllClaims(ctx.DB)
	}
	if err != nil {
		return errorResult(err), nil
	}
	if len(claims) == 0 {
		qualifier := ""
		if agent != "" {
			qualifier = fmt.Sprintf(" for @%s", agent)
		}
		return textResult(fmt.Sprintf("No active claims%s", qualifier)), nil
	}

	byAgent := make(map[string][]types.Claim)
	for _, claim := range claims {
		byAgent[claim.AgentID] = append(byAgent[claim.AgentID], claim)
	}

	lines := []string{fmt.Sprintf("Active claims (%d):", len(claims))}
	for agentID, entries := range byAgent {
		lines = append(lines, fmt.Sprintf("\n@%s:", agentID))
		for _, claim := range entries {
			typePrefix := ""
			if claim.ClaimType != types.ClaimTypeFile {
				typePrefix = fmt.Sprintf("%s:", claim.ClaimType)
			}
			reason := ""
			if claim.Reason != nil && *claim.Reason != "" {
				reason = fmt.Sprintf(" - %s", *claim.Reason)
			}
			lines = append(lines, fmt.Sprintf("  %s%s%s", typePrefix, claim.Pattern, reason))
		}
	}

	return textResult(strings.Join(lines, "\n")), nil
}

func handleStatus(ctx ToolContext, message string, files, bdClaims, issues []string, ttlMinutes int, clear bool) (ToolResult, error) {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return errorResult(err), nil
	}
	if agent == nil {
		return textResult(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID)), nil
	}

	if clear {
		count, err := db.DeleteClaimsByAgent(ctx.DB, ctx.AgentID)
		if err != nil {
			return errorResult(err), nil
		}

		now := time.Now().Unix()
		updates := db.AgentUpdates{
			Status:   types.OptionalString{Set: true, Value: nil},
			LastSeen: types.OptionalInt64{Set: true, Value: &now},
		}
		_ = db.UpdateAgent(ctx.DB, ctx.AgentID, updates)
		if updated, err := db.GetAgent(ctx.DB, ctx.AgentID); err == nil && updated != nil {
			_ = db.AppendAgent(ctx.Project.DBPath, *updated)
		}

		body := "status cleared"
		if count > 0 {
			body = fmt.Sprintf("status cleared (released %d claim", count) + pluralSuffix(int(count)) + ")"
		}
		msg, err := db.CreateMessage(ctx.DB, types.Message{
			TS:        now,
			FromAgent: ctx.AgentID,
			Body:      body,
			Mentions:  []string{},
		})
		if err == nil {
			_ = db.AppendMessage(ctx.Project.DBPath, msg)
		}

		result := "Status cleared"
		if count > 0 {
			result += fmt.Sprintf(", released %d claim", count) + pluralSuffix(int(count))
		}
		return textResult(result), nil
	}

	_, _ = db.PruneExpiredClaims(ctx.DB)

	var expiresAt *int64
	if ttlMinutes > 0 {
		value := time.Now().Unix() + int64(ttlMinutes*60)
		expiresAt = &value
	}

	claims := make([]types.ClaimInput, 0, len(files)+len(bdClaims)+len(issues))
	for _, pattern := range files {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeFile, Pattern: pattern})
	}
	for _, entry := range bdClaims {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeBD, Pattern: stripHash(entry)})
	}
	for _, entry := range issues {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeIssue, Pattern: stripHash(entry)})
	}

	created := make([]types.Claim, 0, len(claims))
	var errors []string
	for _, claim := range claims {
		createdClaim, err := db.CreateClaim(ctx.DB, types.ClaimInput{
			AgentID:   ctx.AgentID,
			ClaimType: claim.ClaimType,
			Pattern:   claim.Pattern,
			Reason:    optionalString(message),
			ExpiresAt: expiresAt,
		})
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		created = append(created, *createdClaim)
	}

	now := time.Now().Unix()
	updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
	if message != "" {
		updates.Status = types.OptionalString{Set: true, Value: &message}
	}
	_ = db.UpdateAgent(ctx.DB, ctx.AgentID, updates)
	if updated, err := db.GetAgent(ctx.DB, ctx.AgentID); err == nil && updated != nil {
		_ = db.AppendAgent(ctx.Project.DBPath, *updated)
	}

	body := message
	if body == "" {
		body = "status update"
	}
	if len(created) > 0 {
		claimList := buildClaimList(created)
		if message != "" {
			body = fmt.Sprintf("%s [claimed: %s]", message, claimList)
		} else {
			body = fmt.Sprintf("claimed: %s", claimList)
		}
	}

	msg, err := db.CreateMessage(ctx.DB, types.Message{
		TS:        now,
		FromAgent: ctx.AgentID,
		Body:      body,
		Mentions:  []string{},
	})
	if err == nil {
		_ = db.AppendMessage(ctx.Project.DBPath, msg)
	}

	result := "Status updated"
	if message != "" {
		result = fmt.Sprintf("Status: %s", message)
	}
	if len(created) > 0 {
		list := make([]string, 0, len(created))
		for _, claim := range created {
			if claim.ClaimType == types.ClaimTypeFile {
				list = append(list, claim.Pattern)
			} else {
				list = append(list, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
			}
		}
		result += "\n\nClaimed:\n  " + strings.Join(list, "\n  ")
	}
	if len(errors) > 0 {
		result += "\n\nErrors:\n  " + strings.Join(errors, "\n  ")
	}

	return textResult(result), nil
}

func formatMessages(messages []types.Message) string {
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		mentions := ""
		if len(msg.Mentions) > 0 {
			mentions = fmt.Sprintf(" [mentions: %s]", strings.Join(msg.Mentions, ", "))
		}
		lines = append(lines, fmt.Sprintf("[#%s] @%s: %s%s", msg.ID, msg.FromAgent, msg.Body, mentions))
	}
	return strings.Join(lines, "\n")
}

func sanitizeMessageID(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "@")
	trimmed = strings.TrimPrefix(trimmed, "#")
	return trimmed
}

func textResult(text string) ToolResult {
	return ToolResult{Content: []ToolContent{{Type: "text", Text: text}}}
}

func errorResult(err error) ToolResult {
	return ToolResult{Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error: %s", err.Error())}}, IsError: true}
}

func stripHash(value string) string {
	return strings.TrimPrefix(value, "#")
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func buildClaimList(claims []types.Claim) string {
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		if claim.ClaimType == types.ClaimTypeFile {
			parts = append(parts, claim.Pattern)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
	}
	return strings.Join(parts, ", ")
}

func parseStaleHours(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
