package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

// OpencodeDriver implements the Driver interface for opencode CLI.
type OpencodeDriver struct{}

// Name returns "opencode".
func (d *OpencodeDriver) Name() string {
	return "opencode"
}

// Spawn starts an opencode session with the given prompt.
// Opencode uses tempfile prompt delivery by default.
func (d *OpencodeDriver) Spawn(ctx context.Context, agent types.Agent, prompt string) (*Process, error) {
	delivery := types.PromptDeliveryTempfile
	if agent.Invoke != nil && agent.Invoke.PromptDelivery != "" {
		delivery = agent.Invoke.PromptDelivery
	}

	var cmd *exec.Cmd
	sessionID, _ := core.GenerateGUID("sess")

	switch delivery {
	case types.PromptDeliveryArgs:
		cmd = exec.CommandContext(ctx, "opencode", "-p", prompt)

	case types.PromptDeliveryStdin:
		return nil, fmt.Errorf("stdin delivery not yet implemented for opencode driver")

	case types.PromptDeliveryTempfile:
		// Write prompt to temp file with secure permissions
		tmpFile, err := os.CreateTemp("", "fray-prompt-*.txt")
		if err != nil {
			return nil, fmt.Errorf("create temp file: %w", err)
		}

		// Set restrictive permissions (0600)
		if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
			os.Remove(tmpFile.Name())
			return nil, fmt.Errorf("chmod temp file: %w", err)
		}

		if _, err := tmpFile.WriteString(prompt); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return nil, fmt.Errorf("write temp file: %w", err)
		}
		tmpFile.Close()

		promptPath, _ := filepath.Abs(tmpFile.Name())
		cmd = exec.CommandContext(ctx, "opencode", "-f", promptPath)

		// Track temp file for cleanup in Cleanup()
		proc := &Process{
			Cmd:       cmd,
			StartedAt: time.Now(),
			SessionID: sessionID,
			TempFiles: []string{promptPath},
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			os.Remove(promptPath)
			return nil, fmt.Errorf("stdout pipe: %w", err)
		}
		proc.Stdout = stdout

		stderr, err := cmd.StderrPipe()
		if err != nil {
			stdout.Close()
			os.Remove(promptPath)
			return nil, fmt.Errorf("stderr pipe: %w", err)
		}
		proc.Stderr = stderr

		if err := cmd.Start(); err != nil {
			stdout.Close()
			stderr.Close()
			os.Remove(promptPath)
			return nil, fmt.Errorf("start opencode: %w", err)
		}

		return proc, nil

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
		return nil, fmt.Errorf("start opencode: %w", err)
	}

	return &Process{
		Cmd:       cmd,
		Stdout:    stdout,
		Stderr:    stderr,
		StartedAt: time.Now(),
		SessionID: sessionID,
	}, nil
}

// Cleanup terminates the opencode process and removes temp files.
func (d *OpencodeDriver) Cleanup(proc *Process) error {
	if proc == nil {
		return nil
	}

	// Clean up temp files
	for _, path := range proc.TempFiles {
		os.Remove(path)
	}

	if proc.Cmd == nil || proc.Cmd.Process == nil {
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
