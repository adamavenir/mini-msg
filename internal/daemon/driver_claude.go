package daemon

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
	"github.com/google/uuid"
)

// FindClaudeSessionID finds the most recent Claude Code session ID for the project.
// Claude Code stores sessions in ~/.claude/projects/{hash}/{uuid}.jsonl
// Returns empty string if no session found.
func FindClaudeSessionID(projectPath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Claude Code hashes the project path for the directory name
	// Format: ~/.claude/projects/-Users-adam-dev-fray/
	// We need to find sessions that were modified recently (within last few seconds)
	projectsDir := filepath.Join(home, ".claude", "projects")

	// Try to find the directory for this project by matching path patterns
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	// Convert project path to Claude's hash format (slashes to dashes)
	projectHash := strings.ReplaceAll(projectPath, "/", "-")
	if strings.HasPrefix(projectHash, "-") {
		projectHash = projectHash[1:] // Remove leading dash
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check if this directory matches our project
		if !strings.Contains(entry.Name(), projectHash) && !strings.HasSuffix(projectPath, strings.ReplaceAll(entry.Name(), "-", "/")) {
			continue
		}

		// Found our project dir - find most recent session
		sessionsDir := filepath.Join(projectsDir, entry.Name())
		sessionFiles, err := os.ReadDir(sessionsDir)
		if err != nil {
			continue
		}

		var newestTime time.Time
		var newestID string

		for _, sf := range sessionFiles {
			if sf.IsDir() || !strings.HasSuffix(sf.Name(), ".jsonl") {
				continue
			}
			info, err := sf.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
				// Extract UUID from filename (e.g., "abc123-def456.jsonl" -> "abc123-def456")
				newestID = strings.TrimSuffix(sf.Name(), ".jsonl")
			}
		}

		if newestID != "" {
			return newestID
		}
	}

	return ""
}

// resolveClaudePath finds the claude executable, checking common install locations.
func resolveClaudePath() (string, error) {
	// First try PATH
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	// Check common install locations
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	commonPaths := []string{
		filepath.Join(home, ".claude", "local", "claude"),
		filepath.Join(home, ".claude", "claude"),
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, "bin", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("claude executable not found in PATH or common locations")
}

// ClaudeDriver implements the Driver interface for Claude Code CLI.
type ClaudeDriver struct{}

// Name returns "claude".
func (d *ClaudeDriver) Name() string {
	return "claude"
}

// Spawn starts a Claude Code session with the given prompt.
// Prompt is delivered via stdin (PromptDeliveryStdin) by default.
func (d *ClaudeDriver) Spawn(ctx context.Context, agent types.Agent, prompt string) (*Process, error) {
	delivery := types.PromptDeliveryStdin
	if agent.Invoke != nil && agent.Invoke.PromptDelivery != "" {
		delivery = agent.Invoke.PromptDelivery
	}

	// Resolve claude executable path
	claudePath, err := resolveClaudePath()
	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd

	// Use existing session ID or generate new one for this agent
	// This ensures each agent has their own session, preventing cross-contamination
	var sessionID string
	if agent.LastSessionID != nil && *agent.LastSessionID != "" {
		sessionID = *agent.LastSessionID
	} else {
		sessionID = uuid.New().String()
	}

	// Build base args - always pass --session-id to control which session is used
	// If resuming, also add --resume to continue the conversation
	var args []string
	args = append(args, "--session-id", sessionID)
	if agent.LastSessionID != nil && *agent.LastSessionID != "" {
		args = append(args, "--resume")
	}

	switch delivery {
	case types.PromptDeliveryArgs:
		args = append(args, "-p", prompt)
		cmd = exec.CommandContext(ctx, claudePath, args...)

	case types.PromptDeliveryStdin:
		args = append(args, "-p", "-")
		cmd = exec.CommandContext(ctx, claudePath, args...)

	case types.PromptDeliveryTempfile:
		// Write prompt to temp file and pass path
		return nil, fmt.Errorf("tempfile delivery not yet implemented for claude driver")

	default:
		return nil, fmt.Errorf("unknown prompt delivery: %s", delivery)
	}

	// Set FRAY_AGENT_ID so the agent can use fray commands without --as flag
	cmd.Env = append(os.Environ(), "FRAY_AGENT_ID="+agent.AgentID)

	// Get pipes for stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("start claude: %w", err)
	}

	proc := &Process{
		Cmd:       cmd,
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		StartedAt: time.Now(),
		SessionID: sessionID,
	}

	// Write prompt to stdin if using stdin delivery
	if delivery == types.PromptDeliveryStdin {
		go func() {
			io.WriteString(stdin, prompt)
			stdin.Close()
		}()
	}

	return proc, nil
}

// Cleanup terminates the Claude Code process.
func (d *ClaudeDriver) Cleanup(proc *Process) error {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return nil
	}

	// Close pipes
	if proc.Stdin != nil {
		proc.Stdin.Close()
	}
	if proc.Stdout != nil {
		proc.Stdout.Close()
	}
	if proc.Stderr != nil {
		proc.Stderr.Close()
	}

	// Kill process if still running
	if proc.Cmd.ProcessState == nil {
		proc.Cmd.Process.Kill()
	}

	return nil
}
