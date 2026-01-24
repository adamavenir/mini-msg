package daemon

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/fsnotify/fsnotify"
)

var syncWatchFiles = map[string]bool{
	"messages.jsonl":    true,
	"threads.jsonl":     true,
	"questions.jsonl":   true,
	"agent-state.jsonl": true,
}

func (d *Daemon) startSyncWatcher(ctx context.Context) error {
	if d.syncWatcher != nil {
		return nil
	}
	if !db.IsMultiMachineMode(d.project.DBPath) {
		return nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	d.syncWatcher = watcher

	if err := d.addSyncWatchPaths(); err != nil {
		_ = watcher.Close()
		d.syncWatcher = nil
		return err
	}

	d.wg.Add(1)
	go d.syncWatchLoop(ctx)
	return nil
}

func (d *Daemon) addSyncWatchPaths() error {
	frayDir := filepath.Dir(d.project.DBPath)
	machinesRoot := filepath.Join(frayDir, "shared", "machines")
	if err := os.MkdirAll(machinesRoot, 0o755); err != nil {
		return err
	}
	if err := d.syncWatcher.Add(machinesRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(machinesRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(machinesRoot, entry.Name())
		if err := d.syncWatcher.Add(path); err != nil {
			d.debugf("sync watcher: failed to watch %s: %v", path, err)
		}
	}
	return nil
}

func (d *Daemon) syncWatchLoop(ctx context.Context) {
	defer d.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case event, ok := <-d.syncWatcher.Events:
			if !ok {
				return
			}
			d.handleSyncEvent(event)
		case err, ok := <-d.syncWatcher.Errors:
			if !ok {
				return
			}
			d.debugf("sync watcher error: %v", err)
		}
	}
}

func (d *Daemon) handleSyncEvent(event fsnotify.Event) {
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if err := d.syncWatcher.Add(event.Name); err != nil {
				d.debugf("sync watcher: failed to watch new dir %s: %v", event.Name, err)
			}
			d.scheduleSyncRebuild()
			return
		}
	}

	if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
		if isSyncJSONL(event.Name) {
			d.scheduleSyncRebuild()
		}
	}
}

func (d *Daemon) scheduleSyncRebuild() {
	d.syncRebuildMu.Lock()
	defer d.syncRebuildMu.Unlock()

	if d.syncDebounce != nil {
		d.syncDebounce.Stop()
	}
	d.syncDebounce = time.AfterFunc(500*time.Millisecond, func() {
		select {
		case <-d.stopCh:
			return
		default:
		}
		d.syncRebuildMu.Lock()
		defer d.syncRebuildMu.Unlock()
		if d.syncRebuildFn == nil {
			return
		}
		if err := d.syncRebuildFn(); err != nil {
			d.debugf("sync rebuild failed: %v", err)
		}
	})
}

func (d *Daemon) stopSyncDebounce() {
	d.syncRebuildMu.Lock()
	defer d.syncRebuildMu.Unlock()
	if d.syncDebounce != nil {
		d.syncDebounce.Stop()
		d.syncDebounce = nil
	}
}

func isSyncJSONL(path string) bool {
	return syncWatchFiles[filepath.Base(path)]
}
