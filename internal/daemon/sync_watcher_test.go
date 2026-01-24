package daemon

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

func setupSyncWatchDaemon(t *testing.T) (*Daemon, string, string) {
	t.Helper()

	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir fray: %v", err)
	}

	if _, err := db.UpdateProjectConfig(projectDir, db.ProjectConfig{
		StorageVersion: 2,
		ChannelID:      "ch-sync",
		ChannelName:    "sync",
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machineDir := filepath.Join(frayDir, "shared", "machines", "alpha")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir machine: %v", err)
	}

	project := core.Project{Root: projectDir, DBPath: filepath.Join(frayDir, "fray.db")}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		t.Fatalf("init schema: %v", err)
	}

	cfg := Config{
		PollInterval: 1 * time.Hour,
		Force:        true,
		WatchSync:    true,
	}
	d := New(project, dbConn, cfg)

	t.Cleanup(func() {
		_ = d.Stop()
		_ = dbConn.Close()
	})

	return d, projectDir, machineDir
}

func waitForRebuild(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for rebuild")
	}
}

func TestSyncWatcherTriggersRebuild(t *testing.T) {
	d, _, machineDir := setupSyncWatchDaemon(t)

	rebuildCh := make(chan struct{}, 1)
	d.syncRebuildFn = func() error {
		rebuildCh <- struct{}{}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	path := filepath.Join(machineDir, "messages.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"message\",\"id\":\"msg-1\"}\n"), 0o644); err != nil {
		t.Fatalf("write message: %v", err)
	}

	waitForRebuild(t, rebuildCh)
}

func TestSyncWatcherDebounces(t *testing.T) {
	d, _, machineDir := setupSyncWatchDaemon(t)

	var count int32
	d.syncRebuildFn = func() error {
		atomic.AddInt32(&count, 1)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	path := filepath.Join(machineDir, "messages.jsonl")
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(path, []byte("{\"type\":\"message\",\"id\":\"msg\"}\n"), 0o644); err != nil {
			t.Fatalf("write message: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(1 * time.Second)
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("expected 1 rebuild, got %d", got)
	}
}

func TestSyncWatcherDetectsNewMachine(t *testing.T) {
	d, projectDir, _ := setupSyncWatchDaemon(t)

	rebuildCh := make(chan struct{}, 2)
	d.syncRebuildFn = func() error {
		rebuildCh <- struct{}{}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	betaDir := filepath.Join(projectDir, ".fray", "shared", "machines", "beta")
	if err := os.MkdirAll(betaDir, 0o755); err != nil {
		t.Fatalf("mkdir beta: %v", err)
	}

	time.Sleep(800 * time.Millisecond)
	for {
		select {
		case <-rebuildCh:
		default:
			goto drained
		}
	}
drained:

	path := filepath.Join(betaDir, "messages.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"message\",\"id\":\"msg-beta\"}\n"), 0o644); err != nil {
		t.Fatalf("write message: %v", err)
	}

	waitForRebuild(t, rebuildCh)
}
