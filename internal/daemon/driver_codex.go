package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

// CodexDriver implements the Driver interface for OpenAI Codex CLI.
type CodexDriver struct{}

// Name returns "codex".
func (d *CodexDriver) Name() string {
	return "codex"
}

// Spawn starts a Codex session with the given prompt.
// Codex uses args prompt delivery by default.
func (d *CodexDriver) Spawn(ctx context.Context, agent types.Agent, prompt string) (*Process, error) {
	delivery := types.PromptDeliveryArgs
	if agent.Invoke != nil && agent.Invoke.PromptDelivery != "" {
		delivery = agent.Invoke.PromptDelivery
	}

	var cmd *exec.Cmd
	sessionID, _ := core.GenerateGUID("sess")

	switch delivery {
	case types.PromptDeliveryArgs:
		cmd = exec.CommandContext(ctx, "codex", prompt)

	case types.PromptDeliveryStdin:
		return nil, fmt.Errorf("stdin delivery not yet implemented for codex driver")

	case types.PromptDeliveryTempfile:
		return nil, fmt.Errorf("tempfile delivery not yet implemented for codex driver")

	default:
		return nil, fmt.Errorf("unknown prompt delivery: %s", delivery)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("start codex: %w", err)
	}

	return &Process{
		Cmd:       cmd,
		Stdout:    stdout,
		Stderr:    stderr,
		StartedAt: time.Now(),
		SessionID: sessionID,
	}, nil
}

// Cleanup terminates the Codex process.
func (d *CodexDriver) Cleanup(proc *Process) error {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return nil
	}

	if proc.Stdout != nil {
		proc.Stdout.Close()
	}
	if proc.Stderr != nil {
		proc.Stderr.Close()
	}

	if proc.Cmd.ProcessState == nil {
		proc.Cmd.Process.Kill()
	}

	return nil
}
