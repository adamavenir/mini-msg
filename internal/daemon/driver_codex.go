package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// CodexDriver implements the Driver interface for OpenAI Codex CLI.
type CodexDriver struct{}

// Name returns "codex".
func (d *CodexDriver) Name() string {
	return "codex"
}

// codexEvent represents a JSONL event from codex exec --json output.
type codexEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id,omitempty"`
}

// Spawn starts a Codex session with the given prompt.
// Uses codex exec --json to enable JSONL streaming and capture session ID.
// Resume syntax: codex exec resume <session-id> <prompt>
func (d *CodexDriver) Spawn(ctx context.Context, agent types.Agent, prompt string) (*Process, error) {
	var args []string
	isResume := agent.LastSessionID != nil && *agent.LastSessionID != ""

	// Build command args:
	// Fresh: codex exec --json <prompt>
	// Resume: codex exec resume <session-id> --json <prompt>
	if isResume {
		args = []string{"exec", "resume", *agent.LastSessionID, "--json", prompt}
	} else {
		args = []string{"exec", "--json", prompt}
	}

	cmd := exec.CommandContext(ctx, "codex", args...)

	// Set FRAY_AGENT_ID so the agent can use fray commands without --as flag
	cmd.Env = append(os.Environ(), "FRAY_AGENT_ID="+agent.AgentID)

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

	// Create a pipe to intercept stdout and capture thread_id from first event
	pipeReader, pipeWriter := io.Pipe()
	sessionIDReady := make(chan struct{})

	proc := &Process{
		Cmd:            cmd,
		Stdout:         pipeReader, // Consumers read from this
		Stderr:         stderr,
		StartedAt:      time.Now(),
		SessionID:      "", // Will be populated from thread.started event
		SessionIDReady: sessionIDReady,
	}

	// Start goroutine to parse JSONL and capture session ID, then forward all output
	go func() {
		defer pipeWriter.Close()
		reader := bufio.NewReader(stdout)
		sessionIDCaptured := false

		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					// Write error marker to help with debugging
					pipeWriter.Write([]byte(fmt.Sprintf(`{"type":"error","message":"%s"}`+"\n", err.Error())))
				}
				// Close channel if we never got a session ID (process died early)
				if !sessionIDCaptured {
					close(sessionIDReady)
				}
				break
			}

			// Parse events to capture thread_id from thread.started
			if !sessionIDCaptured {
				var event codexEvent
				if json.Unmarshal(line, &event) == nil {
					if event.Type == "thread.started" && event.ThreadID != "" {
						proc.SessionID = event.ThreadID
						sessionIDCaptured = true
						close(sessionIDReady) // Signal that session ID is available
					}
				}
			}

			// Forward all lines to consumers
			pipeWriter.Write(line)
		}
	}()

	return proc, nil
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
