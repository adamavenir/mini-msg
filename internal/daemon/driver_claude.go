package daemon

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

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

	var cmd *exec.Cmd
	sessionID, _ := core.GenerateGUID("sess")

	switch delivery {
	case types.PromptDeliveryArgs:
		cmd = exec.CommandContext(ctx, "claude", "-p", prompt)

	case types.PromptDeliveryStdin:
		cmd = exec.CommandContext(ctx, "claude", "-p", "-")

	case types.PromptDeliveryTempfile:
		// Write prompt to temp file and pass path
		return nil, fmt.Errorf("tempfile delivery not yet implemented for claude driver")

	default:
		return nil, fmt.Errorf("unknown prompt delivery: %s", delivery)
	}

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
