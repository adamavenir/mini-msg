package hooks

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/gobwas/glob"
	"github.com/spf13/cobra"
)

// NewHookPrecommitCmd implements git pre-commit claim checks.
func NewHookPrecommitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-precommit",
		Short: "Git pre-commit hook for file claim conflict detection",
		RunE: func(cmd *cobra.Command, args []string) error {
			exitCode := runHookPrecommit(cmd)
			os.Exit(exitCode)
			return nil
		},
	}

	return cmd
}

func runHookPrecommit(cmd *cobra.Command) int {
	agentID := os.Getenv("FRAY_AGENT_ID")

	projectPath := os.Getenv("CLAUDE_PROJECT_DIR")
	project, err := core.DiscoverProject(projectPath)
	if err != nil {
		return 0
	}

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return 0
	}
	defer dbConn.Close()
	if err := db.InitSchema(dbConn); err != nil {
		return 0
	}

	stagedFiles, err := gitStagedFiles(project.Root)
	if err != nil || len(stagedFiles) == 0 {
		return 0
	}

	conflicts, err := db.FindConflictingFileClaims(dbConn, stagedFiles, agentID)
	if err != nil || len(conflicts) == 0 {
		return 0
	}

	byAgent := groupClaimsByAgent(conflicts, stagedFiles)
	printPrecommitConflicts(cmd.ErrOrStderr(), byAgent)

	strictMode := false
	if raw, err := db.GetConfig(dbConn, "precommit_strict"); err == nil {
		strictMode = raw == "true"
	}

	if strictMode {
		fmt.Fprintln(cmd.ErrOrStderr(), "Commit blocked (precommit_strict mode enabled).")
		fmt.Fprintln(cmd.ErrOrStderr(), "Use \"fray config precommit_strict false\" to disable strict mode.")
		fmt.Fprintln(cmd.ErrOrStderr(), "")
		return 1
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "Proceeding with commit (advisory mode).")
	fmt.Fprintln(cmd.ErrOrStderr(), "Use \"fray config precommit_strict true\" to block commits with conflicts.")
	fmt.Fprintln(cmd.ErrOrStderr(), "")
	return 0
}

type claimMatch struct {
	Pattern string
	Files   []string
}

func groupClaimsByAgent(conflicts []types.Claim, stagedFiles []string) map[string][]claimMatch {
	byAgent := make(map[string][]claimMatch)
	for _, claim := range conflicts {
		matches := matchClaimFiles(claim.Pattern, stagedFiles)
		byAgent[claim.AgentID] = append(byAgent[claim.AgentID], claimMatch{
			Pattern: claim.Pattern,
			Files:   matches,
		})
	}
	return byAgent
}

func matchClaimFiles(pattern string, files []string) []string {
	matcher, err := glob.Compile(pattern)
	if err != nil {
		return nil
	}
	matched := make([]string, 0, len(files))
	for _, file := range files {
		if matcher.Match(file) {
			matched = append(matched, file)
		}
	}
	return matched
}

func printPrecommitConflicts(errOut io.Writer, byAgent map[string][]claimMatch) {
	fmt.Fprintln(errOut, "")
	fmt.Fprintln(errOut, "FILE CLAIM CONFLICTS DETECTED")
	fmt.Fprintln(errOut, "")
	fmt.Fprintln(errOut, "The following staged files are claimed by other agents:")
	fmt.Fprintln(errOut, "")

	for agent, claims := range byAgent {
		fmt.Fprintf(errOut, "  @%s:\n", agent)
		for _, claim := range claims {
			if len(claim.Files) > 0 {
				for _, file := range claim.Files {
					fmt.Fprintf(errOut, "    %s (claimed via %s)\n", file, claim.Pattern)
				}
				continue
			}
			fmt.Fprintf(errOut, "    pattern: %s\n", claim.Pattern)
		}
	}

	fmt.Fprintln(errOut, "")
	fmt.Fprintln(errOut, "Consider coordinating with these agents before committing.")
	fmt.Fprintln(errOut, "Use \"fray claims\" to see all active claims.")
	fmt.Fprintln(errOut, "")
}

func gitStagedFiles(projectRoot string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = projectRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		files = append(files, trimmed)
	}
	return files, nil
}
