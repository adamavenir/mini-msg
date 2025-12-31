package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	diffDeletedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
	diffAddedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
)

// NewVersionsCmd creates the versions command.
func NewVersionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <message-id>",
		Short: "Show edit history for a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			last, _ := cmd.Flags().GetInt("last")
			showDiff, _ := cmd.Flags().GetBool("diff")
			if last < 0 {
				return writeCommandError(cmd, fmt.Errorf("--last must be >= 0"))
			}

			msg, err := resolveMessageRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			history, err := db.GetMessageVersions(ctx.Project.DBPath, msg.ID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := *history
				payload.Versions = applyVersionLimit(history.Versions, last)
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			if showDiff {
				return printVersionDiffs(cmd, history, last)
			}

			return printVersions(cmd, history, last)
		},
	}

	cmd.Flags().Int("last", 0, "show only last N versions")
	cmd.Flags().Bool("diff", false, "show diffs between versions")

	return cmd
}

func printVersions(cmd *cobra.Command, history *types.MessageVersionHistory, last int) error {
	out := cmd.OutOrStdout()
	versions := applyVersionLimit(history.Versions, last)

	header := fmt.Sprintf("%s (%d version", history.MessageID, history.VersionCount)
	if history.VersionCount != 1 {
		header += "s"
	}
	if last > 0 && last < history.VersionCount {
		header += fmt.Sprintf(", showing last %d", last)
	}
	if history.IsArchived {
		header += ") [archived]"
	} else {
		header += ")"
	}

	fmt.Fprintln(out, header)
	fmt.Fprintln(out, "")

	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		label := versionLabel(version)
		timestamp := formatVersionTime(version.Timestamp)
		fmt.Fprintf(out, "v%d%s %s\n", version.Version, label, timestamp)
		if version.Version > 1 && version.Reason != "" {
			fmt.Fprintf(out, "  \"%s\"\n", version.Reason)
		}
		printBody(out, version.Body)
		fmt.Fprintln(out, "")
	}

	return nil
}

func printVersionDiffs(cmd *cobra.Command, history *types.MessageVersionHistory, last int) error {
	out := cmd.OutOrStdout()
	versions := history.Versions
	if len(versions) < 2 {
		fmt.Fprintf(out, "%s has no edits\n", history.MessageID)
		return nil
	}

	diffCount := len(versions) - 1
	start := 1
	if last > 0 && last < diffCount {
		start = len(versions) - last
	}

	header := fmt.Sprintf("%s edit history", history.MessageID)
	if last == 1 {
		header += " (last change)"
	} else if last > 1 && last < diffCount {
		header += fmt.Sprintf(" (last %d changes)", last)
	}
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, "")

	for i := start; i < len(versions); i++ {
		prev := versions[i-1]
		next := versions[i]
		timestamp := formatVersionTime(next.Timestamp)
		fmt.Fprintf(out, "v%d -> v%d (%s)\n", prev.Version, next.Version, timestamp)
		fmt.Fprintln(out, diffVersions(prev, next))
		if i < len(versions)-1 {
			fmt.Fprintln(out, "")
		}
	}
	return nil
}

func applyVersionLimit(versions []types.MessageVersion, last int) []types.MessageVersion {
	if last <= 0 || last >= len(versions) {
		return versions
	}
	start := len(versions) - last
	return versions[start:]
}

func versionLabel(version types.MessageVersion) string {
	labels := make([]string, 0, 2)
	if version.IsOriginal {
		labels = append(labels, "original")
	}
	if version.IsCurrent {
		labels = append(labels, "current")
	}
	if len(labels) == 0 {
		return ""
	}
	return " (" + strings.Join(labels, ", ") + ")"
}

func formatVersionTime(ts int64) string {
	return time.Unix(ts, 0).Local().Format("2006-01-02 15:04")
}

func printBody(out interface{ Write([]byte) (int, error) }, body string) {
	if body == "" {
		fmt.Fprintln(out, "  ")
		return
	}
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		fmt.Fprintf(out, "  %s\n", line)
	}
}

func diffVersions(prev, next types.MessageVersion) string {
	if !strings.Contains(prev.Body, "\n") && !strings.Contains(next.Body, "\n") {
		deleted := diffDeletedStyle.Render("- " + prev.Body)
		added := diffAddedStyle.Render("+ " + next.Body)
		return deleted + "\n" + added
	}

	oldLines := strings.Split(prev.Body, "\n")
	newLines := strings.Split(next.Body, "\n")

	var builder strings.Builder
	for _, line := range oldLines {
		builder.WriteString(diffDeletedStyle.Render("- " + line))
		builder.WriteByte('\n')
	}
	for i, line := range newLines {
		builder.WriteString(diffAddedStyle.Render("+ " + line))
		if i < len(newLines)-1 {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}
