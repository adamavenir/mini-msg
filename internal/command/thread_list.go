package command

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewThreadsCmd creates the threads list command.
func NewThreadsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "threads",
		Short: "List threads",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			all, _ := cmd.Flags().GetBool("all")
			pinnedOnly, _ := cmd.Flags().GetBool("pinned")
			mutedOnly, _ := cmd.Flags().GetBool("muted")
			following, _ := cmd.Flags().GetBool("following")
			activity, _ := cmd.Flags().GetBool("activity")
			treeView, _ := cmd.Flags().GetBool("tree")
			asRef, _ := cmd.Flags().GetString("as")

			// Handle --pinned filter
			if pinnedOnly {
				threads, err := db.GetPinnedThreads(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				return outputThreads(cmd, ctx, threads, "Pinned threads:")
			}

			// Handle --muted filter
			if mutedOnly {
				agentID, err := resolveSubscriptionAgent(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				threads, err := db.GetMutedThreads(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				return outputThreads(cmd, ctx, threads, "Muted threads:")
			}

			var options types.ThreadQueryOptions
			var agentID string

			// --all shows everything, --following filters to subscribed
			if all {
				options.IncludeArchived = true
			} else if following {
				agentID, err = resolveSubscriptionAgent(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				options.SubscribedAgent = &agentID
			} else {
				// Default: show subscribed threads (same as --following)
				agentID, err = resolveSubscriptionAgent(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				options.SubscribedAgent = &agentID
			}

			// Apply activity sort
			if activity {
				options.SortByActivity = true
			}

			threads, err := db.GetThreads(ctx.DB, &options)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Exclude muted threads by default (unless --all or --muted)
			if !all && agentID != "" {
				mutedGUIDs, err := db.GetMutedThreadGUIDs(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if len(mutedGUIDs) > 0 {
					filtered := make([]types.Thread, 0, len(threads))
					for _, t := range threads {
						if !mutedGUIDs[t.GUID] {
							filtered = append(filtered, t)
						}
					}
					threads = filtered
				}
			}

			header := "Threads:"
			if following {
				header = "Following:"
			} else if activity {
				header = "Threads (by activity):"
			}

			if treeView {
				return outputThreadsTree(cmd, ctx, threads, agentID, header)
			}
			return outputThreads(cmd, ctx, threads, header)
		},
	}

	cmd.Flags().Bool("all", false, "list all threads (includes muted)")
	cmd.Flags().Bool("pinned", false, "list only pinned threads")
	cmd.Flags().Bool("muted", false, "list only muted threads")
	cmd.Flags().Bool("following", false, "list threads you follow")
	cmd.Flags().Bool("activity", false, "sort by recent activity")
	cmd.Flags().Bool("tree", false, "show threads as tree with indicators")
	cmd.Flags().String("as", "", "agent or user to list subscriptions for")

	return cmd
}

func outputThreads(cmd *cobra.Command, ctx *CommandContext, threads []types.Thread, header string) error {
	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(threads)
	}

	out := cmd.OutOrStdout()
	if len(threads) == 0 {
		fmt.Fprintln(out, "No threads found")
		return nil
	}
	fmt.Fprintln(out, header)
	for _, thread := range threads {
		path, err := buildThreadPath(ctx.DB, &thread)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		// Check if thread is pinned for display
		pinned, _ := db.IsThreadPinned(ctx.DB, thread.GUID)
		indicator := ""
		if pinned {
			indicator = " [pinned]"
		}
		fmt.Fprintf(out, "  %s (%s) [%s]%s\n", path, thread.GUID, thread.Status, indicator)
	}
	return nil
}

// outputThreadsTree displays threads in a tree structure with indicators.
func outputThreadsTree(cmd *cobra.Command, ctx *CommandContext, threads []types.Thread, agentID, header string) error {
	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(threads)
	}

	out := cmd.OutOrStdout()
	if len(threads) == 0 {
		fmt.Fprintln(out, "No threads found")
		return nil
	}

	// Build lookup maps
	byGUID := make(map[string]*types.Thread)
	children := make(map[string][]*types.Thread)
	var roots []*types.Thread

	for i := range threads {
		t := &threads[i]
		byGUID[t.GUID] = t
	}

	for i := range threads {
		t := &threads[i]
		if t.ParentThread == nil || *t.ParentThread == "" {
			roots = append(roots, t)
		} else {
			parent := *t.ParentThread
			children[parent] = append(children[parent], t)
		}
	}

	// Sort roots: meta first, then alphabetically
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].Name == "meta" {
			return true
		}
		if roots[j].Name == "meta" {
			return false
		}
		return roots[i].Name < roots[j].Name
	})

	// Get indicators data
	pinnedGUIDs := make(map[string]bool)
	pinnedThreads, _ := db.GetPinnedThreads(ctx.DB)
	for _, t := range pinnedThreads {
		pinnedGUIDs[t.GUID] = true
	}

	mutedGUIDs := make(map[string]bool)
	if agentID != "" {
		mutedGUIDs, _ = db.GetMutedThreadGUIDs(ctx.DB, agentID)
	}

	followedGUIDs := make(map[string]bool)
	for _, t := range threads {
		followedGUIDs[t.GUID] = true // if in our list, we follow it
	}

	fmt.Fprintln(out, header)

	// Print tree recursively
	var printTree func(t *types.Thread, prefix string, isLast bool)
	printTree = func(t *types.Thread, prefix string, isLast bool) {
		// Build indicators
		var indicators []string
		if followedGUIDs[t.GUID] && agentID != "" {
			indicators = append(indicators, "★")
		}
		if pinnedGUIDs[t.GUID] {
			indicators = append(indicators, "📌")
		}
		if mutedGUIDs[t.GUID] {
			indicators = append(indicators, "(muted)")
		}

		indicatorStr := ""
		if len(indicators) > 0 {
			indicatorStr = " " + strings.Join(indicators, " ")
		}

		// Determine tree characters
		branch := "├── "
		if isLast {
			branch = "└── "
		}
		if prefix == "" {
			branch = ""
		}

		fmt.Fprintf(out, "%s%s%s%s\n", prefix, branch, t.Name, indicatorStr)

		// Update prefix for children
		childPrefix := prefix
		if prefix != "" {
			if isLast {
				childPrefix += "    "
			} else {
				childPrefix += "│   "
			}
		} else {
			childPrefix = "  "
		}

		// Print children
		kids := children[t.GUID]
		for i, child := range kids {
			printTree(child, childPrefix, i == len(kids)-1)
		}
	}

	for i, root := range roots {
		printTree(root, "", i == len(roots)-1)
	}

	return nil
}

func resolveSubscriptionAgent(ctx *CommandContext, ref string) (string, error) {
	if ref != "" {
		return ResolveAgentRef(ref, ctx.ProjectConfig), nil
	}
	username, err := db.GetConfig(ctx.DB, "username")
	if err != nil {
		return "", err
	}
	if username == "" {
		return "", fmt.Errorf("--as is required unless --all is set")
	}
	return username, nil
}
