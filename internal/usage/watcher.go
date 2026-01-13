package usage

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// UsageEvent represents a change in token usage for a session.
type UsageEvent struct {
	SessionID    string
	Usage        *SessionUsage
	PrevUsage    *SessionUsage
	TokensDelta  int64 // Change in input tokens
	Timestamp    time.Time
}

// Watcher monitors transcript files for active sessions and emits events on changes.
type Watcher struct {
	fsWatcher    *fsnotify.Watcher
	sessions     map[string]*watchedSession // sessionID -> session info
	sessionsMu   sync.RWMutex
	events       chan UsageEvent
	errors       chan error
	done         chan struct{}
	debounceMs   int64 // Debounce file changes (ms)
}

type watchedSession struct {
	sessionID     string
	transcriptPath string
	driver        string
	lastUsage     *SessionUsage
	lastModified  time.Time
	pendingUpdate bool
}

// NewWatcher creates a new usage watcher.
func NewWatcher() (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fsWatcher:  fsWatcher,
		sessions:   make(map[string]*watchedSession),
		events:     make(chan UsageEvent, 100),
		errors:     make(chan error, 10),
		done:       make(chan struct{}),
		debounceMs: 500, // 500ms debounce
	}

	go w.run()

	return w, nil
}

// Events returns the channel for usage change events.
func (w *Watcher) Events() <-chan UsageEvent {
	return w.events
}

// Errors returns the channel for watcher errors.
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// WatchSession starts watching a session's transcript file.
// driver should be "claude" or "codex" to help locate the transcript.
func (w *Watcher) WatchSession(sessionID, driver string) error {
	w.sessionsMu.Lock()
	defer w.sessionsMu.Unlock()

	// Already watching?
	if _, ok := w.sessions[sessionID]; ok {
		return nil
	}

	// Find the transcript file
	var transcriptPath string
	switch driver {
	case "claude":
		transcriptPath = findClaudeTranscript(sessionID)
	case "codex":
		transcriptPath = findCodexTranscript(sessionID)
	default:
		// Try both
		transcriptPath = findClaudeTranscript(sessionID)
		if transcriptPath == "" {
			transcriptPath = findCodexTranscript(sessionID)
		}
	}

	if transcriptPath == "" {
		// File doesn't exist yet - we'll need to watch the parent directories
		// and wait for it to appear. For now, store session without path.
		w.sessions[sessionID] = &watchedSession{
			sessionID: sessionID,
			driver:    driver,
		}
		// Watch potential parent directories
		w.watchParentDirs(driver)
		return nil
	}

	// Watch the transcript file
	if err := w.fsWatcher.Add(transcriptPath); err != nil {
		return err
	}

	// Get initial usage
	var usage *SessionUsage
	var err error
	switch driver {
	case "claude":
		usage, err = parseClaudeTranscript(sessionID, transcriptPath)
	case "codex":
		usage, err = parseCodexTranscript(sessionID, transcriptPath)
	default:
		usage, err = GetSessionUsage(sessionID)
	}
	if err != nil {
		usage = nil
	}

	w.sessions[sessionID] = &watchedSession{
		sessionID:      sessionID,
		transcriptPath: transcriptPath,
		driver:         driver,
		lastUsage:      usage,
		lastModified:   time.Now(),
	}

	return nil
}

// UnwatchSession stops watching a session's transcript file.
func (w *Watcher) UnwatchSession(sessionID string) {
	w.sessionsMu.Lock()
	defer w.sessionsMu.Unlock()

	session, ok := w.sessions[sessionID]
	if !ok {
		return
	}

	if session.transcriptPath != "" {
		w.fsWatcher.Remove(session.transcriptPath)
	}

	delete(w.sessions, sessionID)
}

// Close stops the watcher and releases resources.
func (w *Watcher) Close() error {
	close(w.done)
	return w.fsWatcher.Close()
}

// watchParentDirs watches parent directories where transcripts might appear.
func (w *Watcher) watchParentDirs(driver string) {
	switch driver {
	case "claude":
		for _, path := range getClaudePaths() {
			w.fsWatcher.Add(path)
		}
	case "codex":
		for _, path := range getCodexPaths() {
			w.fsWatcher.Add(path)
		}
	default:
		for _, path := range getClaudePaths() {
			w.fsWatcher.Add(path)
		}
		for _, path := range getCodexPaths() {
			w.fsWatcher.Add(path)
		}
	}
}

// run is the main event loop for the watcher.
func (w *Watcher) run() {
	debounceTimer := time.NewTicker(time.Duration(w.debounceMs) * time.Millisecond)
	defer debounceTimer.Stop()

	for {
		select {
		case <-w.done:
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleFSEvent(event)

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			default:
			}

		case <-debounceTimer.C:
			w.processPendingUpdates()
		}
	}
}

// handleFSEvent processes a filesystem event.
func (w *Watcher) handleFSEvent(event fsnotify.Event) {
	// Only care about writes
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
		return
	}

	w.sessionsMu.Lock()
	defer w.sessionsMu.Unlock()

	// Check if this is a file we're watching
	for _, session := range w.sessions {
		if session.transcriptPath == event.Name {
			session.pendingUpdate = true
			session.lastModified = time.Now()
			return
		}
	}

	// Check if this is a new transcript file for a session we're waiting on
	fileName := filepath.Base(event.Name)
	for sessionID, session := range w.sessions {
		if session.transcriptPath == "" {
			// Check if filename matches session ID
			expectedName := sessionID + ".jsonl"
			if fileName == expectedName {
				// Found the transcript! Start watching it.
				session.transcriptPath = event.Name
				session.pendingUpdate = true
				session.lastModified = time.Now()
				w.fsWatcher.Add(event.Name)
				return
			}
		}
	}
}

// processPendingUpdates checks sessions with pending updates and emits events.
func (w *Watcher) processPendingUpdates() {
	w.sessionsMu.Lock()
	defer w.sessionsMu.Unlock()

	for _, session := range w.sessions {
		if !session.pendingUpdate || session.transcriptPath == "" {
			continue
		}

		session.pendingUpdate = false

		// Parse the updated transcript
		var newUsage *SessionUsage
		var err error
		switch session.driver {
		case "claude":
			newUsage, err = parseClaudeTranscript(session.sessionID, session.transcriptPath)
		case "codex":
			newUsage, err = parseCodexTranscript(session.sessionID, session.transcriptPath)
		default:
			newUsage, err = GetSessionUsage(session.sessionID)
		}

		if err != nil || newUsage == nil {
			continue
		}

		// Calculate delta
		var tokensDelta int64
		if session.lastUsage != nil {
			tokensDelta = newUsage.InputTokens - session.lastUsage.InputTokens
		} else {
			tokensDelta = newUsage.InputTokens
		}

		// Emit event if there was a meaningful change
		if tokensDelta > 0 {
			event := UsageEvent{
				SessionID:   session.sessionID,
				Usage:       newUsage,
				PrevUsage:   session.lastUsage,
				TokensDelta: tokensDelta,
				Timestamp:   time.Now(),
			}

			select {
			case w.events <- event:
			default:
				// Channel full, drop event
			}
		}

		session.lastUsage = newUsage
	}
}

// GetWatchedSessions returns the list of currently watched session IDs.
func (w *Watcher) GetWatchedSessions() []string {
	w.sessionsMu.RLock()
	defer w.sessionsMu.RUnlock()

	sessions := make([]string, 0, len(w.sessions))
	for id := range w.sessions {
		sessions = append(sessions, id)
	}
	return sessions
}

// GetSessionUsageSnapshot returns the current usage for a watched session.
func (w *Watcher) GetSessionUsageSnapshot(sessionID string) *SessionUsage {
	w.sessionsMu.RLock()
	defer w.sessionsMu.RUnlock()

	if session, ok := w.sessions[sessionID]; ok {
		return session.lastUsage
	}
	return nil
}
