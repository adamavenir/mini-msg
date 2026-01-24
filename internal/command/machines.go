package command

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

type machineInfo struct {
	ID                   string         `json:"id"`
	Status               string         `json:"status"`
	LastWrite            int64          `json:"last_write,omitempty"`
	LastWriteRelative    string         `json:"last_write_relative,omitempty"`
	LastActivity         int64          `json:"last_activity,omitempty"`
	LastActivityRelative string         `json:"last_activity_relative,omitempty"`
	Agents               []string       `json:"agents,omitempty"`
	FileCounts           map[string]int `json:"file_counts,omitempty"`
}

// NewMachinesCmd creates the machines command.
func NewMachinesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machines",
		Short: "List synced machines",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			if !db.IsMultiMachineMode(ctx.Project.DBPath) {
				return writeCommandError(cmd, fmt.Errorf("machines command is only supported in multi-machine projects"))
			}

			verbose, _ := cmd.Flags().GetBool("verbose")
			jsonMode, _ := cmd.Flags().GetBool("json")

			machineDirs := db.GetSharedMachinesDirs(ctx.Project.DBPath)
			if len(machineDirs) == 0 {
				if jsonMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"machines": []machineInfo{}})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "No machines found")
				return nil
			}

			localID := db.GetLocalMachineID(ctx.Project.DBPath)
			infos := make([]machineInfo, 0, len(machineDirs))
			for _, dir := range machineDirs {
				machineID := filepath.Base(dir)
				status := "remote"
				if localID != "" && machineID == localID {
					status = "local"
				}
				lastWrite := lastMachineWrite(dir)
				lastWriteRel := "never"
				if lastWrite > 0 {
					lastWriteRel = formatRelative(lastWrite)
				}
				agents, err := agentsForMachine(ctx.DB, machineID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				lastActivity, err := lastActivityForMachine(ctx.DB, machineID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				lastActivityRel := "never"
				if lastActivity > 0 {
					lastActivityRel = formatRelative(lastActivity)
				}

				info := machineInfo{
					ID:                   machineID,
					Status:               status,
					LastWrite:            lastWrite,
					LastWriteRelative:    lastWriteRel,
					LastActivity:         lastActivity,
					LastActivityRelative: lastActivityRel,
					Agents:               agents,
				}
				if verbose {
					counts, err := countMachineFiles(dir)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					info.FileCounts = counts
				}
				infos = append(infos, info)
			}

			sort.Slice(infos, func(i, j int) bool {
				return infos[i].ID < infos[j].ID
			})

			if jsonMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"machines": infos})
			}

			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			if verbose {
				fmt.Fprintln(w, "MACHINE\tSTATUS\tLAST WRITE\tLAST ACTIVITY\tFILES\tAGENTS")
			} else {
				fmt.Fprintln(w, "MACHINE\tSTATUS\tLAST WRITE\tAGENTS")
			}
			for _, info := range infos {
				agents := "-"
				if len(info.Agents) > 0 {
					agents = strings.Join(info.Agents, ", ")
				}
				if verbose {
					files := formatFileCounts(info.FileCounts)
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
						info.ID, info.Status, info.LastWriteRelative, info.LastActivityRelative, files, agents)
				} else {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
						info.ID, info.Status, info.LastWriteRelative, agents)
				}
			}
			return w.Flush()
		},
	}

	cmd.Flags().Bool("verbose", false, "show file counts and last activity")
	return cmd
}

func lastMachineWrite(machineDir string) int64 {
	files := []string{"messages.jsonl", "threads.jsonl", "questions.jsonl", "agent-state.jsonl"}
	latest := int64(0)
	for _, name := range files {
		info, err := os.Stat(filepath.Join(machineDir, name))
		if err != nil {
			continue
		}
		ts := info.ModTime().Unix()
		if ts > latest {
			latest = ts
		}
	}
	return latest
}

func agentsForMachine(dbConn *sql.DB, machineID string) ([]string, error) {
	rows, err := dbConn.Query(`
		SELECT DISTINCT from_agent
		FROM fray_messages
		WHERE origin = ?
		AND from_agent != ''
		ORDER BY from_agent ASC
	`, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []string
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, err
		}
		if agent == "" {
			continue
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func lastActivityForMachine(dbConn *sql.DB, machineID string) (int64, error) {
	row := dbConn.QueryRow(`
		SELECT MAX(ts) FROM fray_messages WHERE origin = ?
	`, machineID)
	var maxTS sql.NullInt64
	if err := row.Scan(&maxTS); err != nil {
		return 0, err
	}
	if !maxTS.Valid {
		return 0, nil
	}
	return maxTS.Int64, nil
}

func countMachineFiles(machineDir string) (map[string]int, error) {
	files := map[string]string{
		"messages":   "messages.jsonl",
		"threads":    "threads.jsonl",
		"questions":  "questions.jsonl",
		"agentState": "agent-state.jsonl",
	}
	counts := make(map[string]int, len(files))
	for key, name := range files {
		count, err := countJSONLLines(filepath.Join(machineDir, name))
		if err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return counts, nil
}

func countJSONLLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func formatFileCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "-"
	}
	return fmt.Sprintf("m:%d t:%d q:%d s:%d",
		counts["messages"], counts["threads"], counts["questions"], counts["agentState"])
}
