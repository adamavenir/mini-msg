package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/router"
	"github.com/adamavenir/fray/internal/usage"
	"github.com/fsnotify/fsnotify"
	mlld "github.com/mlld-lang/mlld/sdk/go"
)

// Daemon watches for @mentions and spawns managed agents.
type Daemon struct {
	mu             sync.RWMutex
	project        core.Project
	database       *sql.DB
	debouncer      *MentionDebouncer
	detector       ActivityDetector
	router         *router.Router       // mlld-based routing for mention interpretation
	mlldClient     *mlld.Client         // mlld client for prompt template execution
	usageWatcher   *usage.Watcher       // monitors transcript files for token usage
	processes      map[string]*Process  // agent_id -> process
	spawning       map[string]bool      // agent_id -> true if spawn in progress (prevents races)
	handled        map[string]bool      // agent_id -> true if exit already handled
	cooldownUntil  map[string]time.Time // agent_id -> when cooldown expires (after clean exit)
	drivers        map[string]Driver    // driver name -> driver
	lastSpawnTime  time.Time            // rate-limit spawns to prevent resource exhaustion
	startedAt      time.Time            // daemon start time (for staleness gate)
	lastDebugMsg   string               // last debug message for collapsing repeats
	lastDebugCount int                  // count of consecutive repeated messages
	stopCh         chan struct{}
	cancelFunc     context.CancelFunc // cancels spawned process contexts
	wg             sync.WaitGroup
	lockPath       string
	pollInterval   time.Duration
	debug          bool
	force          bool
	watchSync      bool
	syncWatcher    *fsnotify.Watcher
	syncDebounce   *time.Timer
	syncRebuildMu  sync.Mutex
	syncRebuildFn  func() error
}

// LockInfo represents the daemon lock file contents.
type LockInfo struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"started_at"`
}

// Config holds daemon configuration options.
type Config struct {
	PollInterval time.Duration
	Debug        bool
	Force        bool // Kill existing daemon if running
	WatchSync    bool
}

// DefaultConfig returns default daemon configuration.
func DefaultConfig() Config {
	return Config{
		PollInterval: 1 * time.Second,
	}
}

// New creates a new daemon for the given project.
func New(project core.Project, database *sql.DB, cfg Config) *Daemon {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultConfig().PollInterval
	}

	frayDir := filepath.Dir(project.DBPath)

	// Create mlld client for prompt template execution
	mlldClient := mlld.New()
	mlldClient.Timeout = 30 * time.Second
	mlldClient.WorkingDir = project.Root

	d := &Daemon{
		project:       project,
		database:      database,
		debouncer:     NewMentionDebouncer(database, project.DBPath),
		detector:      NewActivityDetector(),
		router:        router.New(frayDir),
		mlldClient:    mlldClient,
		processes:     make(map[string]*Process),
		spawning:      make(map[string]bool),
		handled:       make(map[string]bool),
		cooldownUntil: make(map[string]time.Time),
		drivers:       make(map[string]Driver),
		stopCh:        make(chan struct{}),
		lockPath:      filepath.Join(frayDir, "daemon.lock"),
		pollInterval:  cfg.PollInterval,
		debug:         cfg.Debug,
		force:         cfg.Force,
		watchSync:     cfg.WatchSync,
	}

	d.syncRebuildFn = func() error {
		return db.RebuildDatabaseFromJSONL(d.database, d.project.DBPath)
	}

	// Register drivers
	d.drivers["claude"] = &ClaudeDriver{}
	d.drivers["codex"] = &CodexDriver{}
	d.drivers["opencode"] = &OpencodeDriver{}

	return d
}

// Start begins the daemon watch loop.
func (d *Daemon) Start(ctx context.Context) error {
	// Record start time for staleness gate
	d.startedAt = time.Now()

	// Acquire lock
	if err := d.acquireLock(); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}

	// Clean up stale presence states from previous daemon runs
	d.cleanupStalePresence()

	// Create usage watcher for monitoring transcript files
	usageWatcher, err := usage.NewWatcher()
	if err != nil {
		d.debugf("failed to create usage watcher: %v", err)
		// Non-fatal - daemon can run without usage watching
	} else {
		d.usageWatcher = usageWatcher
		d.wg.Add(1)
		go d.usageWatchLoop()
	}

	// Create cancellable context for spawned processes
	procCtx, cancel := context.WithCancel(ctx)
	d.cancelFunc = cancel

	d.wg.Add(1)
	go d.watchLoop(procCtx)

	if d.watchSync {
		if err := d.startSyncWatcher(procCtx); err != nil {
			d.debugf("failed to start sync watcher: %v", err)
		}
	}

	return nil
}

// Stop gracefully shuts down the daemon.
func (d *Daemon) Stop() error {
	// Signal watch loop to stop
	close(d.stopCh)

	// Cancel process contexts - this kills spawned processes via CommandContext,
	// allowing monitorProcess goroutines to exit
	if d.cancelFunc != nil {
		d.cancelFunc()
	}

	// Close usage watcher
	if d.usageWatcher != nil {
		d.usageWatcher.Close()
	}

	if d.syncWatcher != nil {
		_ = d.syncWatcher.Close()
	}
	d.stopSyncDebounce()

	// Wait for all goroutines (watchLoop, usageWatchLoop, and monitorProcess) to finish
	d.wg.Wait()

	// Cleanup any remaining resources
	d.mu.Lock()
	for agentID, proc := range d.processes {
		driver := d.getDriver(agentID)
		if driver != nil {
			driver.Cleanup(proc)
		}
		if proc.Cmd.Process != nil {
			d.detector.Cleanup(proc.Cmd.Process.Pid)
		}
	}
	d.processes = make(map[string]*Process)
	d.spawning = make(map[string]bool)
	d.handled = make(map[string]bool)
	d.cooldownUntil = make(map[string]time.Time)
	d.mu.Unlock()

	// Release lock
	return d.releaseLock()
}
