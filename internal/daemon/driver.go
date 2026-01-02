package daemon

import (
	"context"
	"io"
	"os/exec"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// Process represents a spawned agent process.
type Process struct {
	Cmd       *exec.Cmd
	Stdin     io.WriteCloser
	Stdout    io.ReadCloser
	Stderr    io.ReadCloser
	StartedAt time.Time
	SessionID string
	TempFiles []string // Temp files to clean up after process exits
}

// Driver defines the interface for CLI-specific agent spawning.
type Driver interface {
	// Name returns the driver identifier (claude, codex, opencode).
	Name() string

	// Spawn starts a new agent session with the given prompt.
	// The prompt is delivered according to the agent's PromptDelivery config.
	Spawn(ctx context.Context, agent types.Agent, prompt string) (*Process, error)

	// Cleanup terminates the process and releases resources.
	Cleanup(proc *Process) error
}

// GetDriver returns a driver for the given name.
// Returns nil if the driver is not recognized.
func GetDriver(name string) Driver {
	switch name {
	case "claude":
		return &ClaudeDriver{}
	case "codex":
		return &CodexDriver{}
	case "opencode":
		return &OpencodeDriver{}
	default:
		return nil
	}
}

// DefaultTimeouts returns default timeout values in milliseconds.
func DefaultTimeouts() (spawnTimeout, idleAfter, maxRuntime int64) {
	return 30000, 5000, 600000
}

// GetTimeouts extracts timeout values from InvokeConfig, using defaults for zero values.
func GetTimeouts(cfg *types.InvokeConfig) (spawnTimeout, idleAfter, maxRuntime int64) {
	defSpawn, defIdle, defMax := DefaultTimeouts()

	spawnTimeout = defSpawn
	idleAfter = defIdle
	maxRuntime = defMax

	if cfg != nil {
		if cfg.SpawnTimeoutMs > 0 {
			spawnTimeout = cfg.SpawnTimeoutMs
		}
		if cfg.IdleAfterMs > 0 {
			idleAfter = cfg.IdleAfterMs
		}
		if cfg.MaxRuntimeMs > 0 {
			maxRuntime = cfg.MaxRuntimeMs
		}
	}

	return
}
