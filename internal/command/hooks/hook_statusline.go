package hooks

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewHookStatuslineCmd provides compact status for Claude Code statusline.
func NewHookStatuslineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-statusline",
		Short: "Output compact status for Claude Code statusline",
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := os.Getenv("FRAY_AGENT_ID")

			projectPath := os.Getenv("CLAUDE_PROJECT_DIR")
			project, err := core.DiscoverProject(projectPath)
			if err != nil {
				return nil
			}

			dbConn, err := db.OpenDatabase(project)
			if err != nil {
				return nil
			}
			defer dbConn.Close()
			if err := db.InitSchema(dbConn); err != nil {
				return nil
			}

			statusline := buildStatusline(dbConn, agentID)
			if statusline != "" {
				fmt.Fprintln(cmd.OutOrStdout(), statusline)
			}
			return nil
		},
	}

	return cmd
}

func buildStatusline(dbConn *sql.DB, agentID string) string {
	var parts []string

	// Questions: asked (open) and wondered (unasked)
	asked, wondered := countQuestions(dbConn)
	if asked > 0 || wondered > 0 {
		var qParts []string
		if asked > 0 {
			qParts = append(qParts, fmt.Sprintf("asked:%d", asked))
		}
		if wondered > 0 {
			qParts = append(qParts, fmt.Sprintf("wondered:%d", wondered))
		}
		parts = append(parts, strings.Join(qParts, " "))
	}

	// Claims by agent
	claimsSummary := buildClaimsSummary(dbConn, agentID)
	if claimsSummary != "" {
		parts = append(parts, claimsSummary)
	}

	if len(parts) == 0 {
		return ""
	}

	prefix := "[fray]"
	if agentID != "" {
		prefix = fmt.Sprintf("[fray %s]", agentID)
	}

	return fmt.Sprintf("%s %s", prefix, strings.Join(parts, " | "))
}

func countQuestions(dbConn *sql.DB) (asked, wondered int) {
	openQ, _ := db.GetQuestions(dbConn, &types.QuestionQueryOptions{
		Statuses: []types.QuestionStatus{types.QuestionStatusOpen},
	})
	asked = len(openQ)

	unaskedQ, _ := db.GetQuestions(dbConn, &types.QuestionQueryOptions{
		Statuses: []types.QuestionStatus{types.QuestionStatusUnasked},
	})
	wondered = len(unaskedQ)

	return
}

func buildClaimsSummary(dbConn *sql.DB, currentAgent string) string {
	claims, err := db.GetAllClaims(dbConn)
	if err != nil || len(claims) == 0 {
		return ""
	}

	// Group by agent
	byAgent := make(map[string][]types.Claim)
	for _, c := range claims {
		byAgent[c.AgentID] = append(byAgent[c.AgentID], c)
	}

	// Sort agents, current agent first if present
	agents := make([]string, 0, len(byAgent))
	for agent := range byAgent {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		if agents[i] == currentAgent {
			return true
		}
		if agents[j] == currentAgent {
			return false
		}
		return agents[i] < agents[j]
	})

	// Build summary
	var summaries []string
	for _, agent := range agents {
		agentClaims := byAgent[agent]
		fileClaims := filterFileClaimsShort(agentClaims)
		if len(fileClaims) > 0 {
			summaries = append(summaries, fmt.Sprintf("@%s (%s)", agent, strings.Join(fileClaims, ", ")))
		} else {
			summaries = append(summaries, fmt.Sprintf("@%s (%d)", agent, len(agentClaims)))
		}
	}

	if len(summaries) == 0 {
		return ""
	}
	return "claims: " + strings.Join(summaries, " ")
}

func filterFileClaimsShort(claims []types.Claim) []string {
	var files []string
	for _, c := range claims {
		if c.ClaimType == types.ClaimTypeFile {
			// Show just the basename or short pattern
			pattern := c.Pattern
			if idx := strings.LastIndex(pattern, "/"); idx != -1 {
				pattern = pattern[idx+1:]
			}
			// Truncate long patterns
			if len(pattern) > 20 {
				pattern = pattern[:17] + "..."
			}
			files = append(files, pattern)
		}
	}
	// Limit to 3 files
	if len(files) > 3 {
		files = append(files[:3], fmt.Sprintf("+%d", len(files)-3))
	}
	return files
}
