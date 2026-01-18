package daemon

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// StdoutBuffer is a thread-safe ring buffer that captures the last N bytes of stdout.
type StdoutBuffer struct {
	mu   sync.Mutex
	buf  []byte
	size int
	pos  int // write position (wraps around)
	full bool
}

// NewStdoutBuffer creates a ring buffer with the given capacity.
func NewStdoutBuffer(size int) *StdoutBuffer {
	return &StdoutBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// Write appends data to the ring buffer, overwriting oldest data if full.
func (b *StdoutBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n = len(p)
	for _, c := range p {
		b.buf[b.pos] = c
		b.pos = (b.pos + 1) % b.size
		if b.pos == 0 {
			b.full = true
		}
	}
	return n, nil
}

// Bytes returns the buffered content in chronological order.
func (b *StdoutBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.full {
		return append([]byte(nil), b.buf[:b.pos]...)
	}
	// Buffer wrapped: content is [pos:] + [:pos]
	result := make([]byte, b.size)
	copy(result, b.buf[b.pos:])
	copy(result[b.size-b.pos:], b.buf[:b.pos])
	return result
}

// String returns the buffered content as a string.
func (b *StdoutBuffer) String() string {
	return string(b.Bytes())
}

// SpawnMode indicates how an agent session was spawned.
type SpawnMode string

const (
	SpawnModeNormal SpawnMode = ""     // Regular @mention spawn
	SpawnModeFly    SpawnMode = "fly"  // /fly command - full session
	SpawnModeHop    SpawnMode = "hop"  // /hop command - auto-bye on idle
	SpawnModeLand   SpawnMode = "land" // /land command - longterm closeout
	SpawnModeHand   SpawnMode = "hand" // /hand command - hot handoff, immediate continuation
)

// Process represents a spawned agent process.
type Process struct {
	Cmd              *exec.Cmd
	Stdin            io.WriteCloser
	Stdout           io.ReadCloser
	Stderr           io.ReadCloser
	StartedAt        time.Time
	SessionID        string
	SessionIDReady   chan struct{} // Closed when SessionID is populated (for async capture)
	TempFiles        []string      // Temp files to clean up after process exits
	BaselineInput    int64         // Baseline input tokens at spawn (for resumed sessions)
	BaselineOutput   int64         // Baseline output tokens at spawn (for resumed sessions)
	LastOutputTokens int64         // Last observed output token count (for idle detection)
	LastTokenCheck   time.Time     // When we last saw token activity (for idle detection)
	StdoutBuffer     *StdoutBuffer // Ring buffer capturing last ~4KB of stdout
	StderrBuffer     *StdoutBuffer // Ring buffer capturing last ~4KB of stderr
	SpawnMode        SpawnMode     // How this session was spawned (fly, hop, land, or normal)
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

// TriggerInfo holds context about what triggered an agent spawn.
type TriggerInfo struct {
	MsgID string // The message ID that triggered this spawn
	Home  string // The home (room or thread name) of the trigger message
}

type contextKey string

const triggerInfoKey contextKey = "triggerInfo"

// ContextWithTrigger adds trigger info to a context for driver use.
func ContextWithTrigger(ctx context.Context, info TriggerInfo) context.Context {
	return context.WithValue(ctx, triggerInfoKey, info)
}

// TriggerFromContext retrieves trigger info from context.
func TriggerFromContext(ctx context.Context) (TriggerInfo, bool) {
	info, ok := ctx.Value(triggerInfoKey).(TriggerInfo)
	return info, ok
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
// spawnTimeout: max time in 'spawning' state (60s - gives time for token parsing/activity fallbacks)
// idleAfter: time since activity before 'idle' presence (5s)
// minCheckin: done-detection threshold - idle + no fray posts = kill (0 = disabled by default)
// maxRuntime: zombie safety net - forced termination (0 = unlimited)
func DefaultTimeouts() (spawnTimeout, idleAfter, minCheckin, maxRuntime int64) {
	return 60000, 5000, 0, 0
}

// GetTimeouts extracts timeout values from InvokeConfig, using defaults for zero values.
func GetTimeouts(cfg *types.InvokeConfig) (spawnTimeout, idleAfter, minCheckin, maxRuntime int64) {
	defSpawn, defIdle, defCheckin, defMax := DefaultTimeouts()

	spawnTimeout = defSpawn
	idleAfter = defIdle
	minCheckin = defCheckin
	maxRuntime = defMax

	if cfg != nil {
		if cfg.SpawnTimeoutMs > 0 {
			spawnTimeout = cfg.SpawnTimeoutMs
		}
		if cfg.IdleAfterMs > 0 {
			idleAfter = cfg.IdleAfterMs
		}
		if cfg.MinCheckinMs > 0 {
			minCheckin = cfg.MinCheckinMs
		}
		if cfg.MaxRuntimeMs > 0 {
			maxRuntime = cfg.MaxRuntimeMs
		}
	}

	return
}
