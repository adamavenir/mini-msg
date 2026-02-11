package hostedsync

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
)

const defaultBatchSize = 200

var sharedFiles = []string{
	"messages.jsonl",
	"threads.jsonl",
	"questions.jsonl",
	"agent-state.jsonl",
}

// Runner coordinates hosted sync operations.
type Runner struct {
	ProjectRoot string
	ChannelID   string
	MachineID   string
	Client      *Client
	Logger      *log.Logger
	BatchSize   int
}

// SyncResult summarizes a single sync pass.
type SyncResult struct {
	PushedLines    int `json:"pushed_lines"`
	PulledLines    int `json:"pulled_lines"`
	PushedStreams  int `json:"pushed_streams"`
	PulledStreams  int `json:"pulled_streams"`
	BaseMismatches int `json:"base_mismatches"`
}

// SyncOnce runs a single push/pull cycle.
func (r *Runner) SyncOnce(ctx context.Context) (SyncResult, error) {
	result := SyncResult{}
	if r.Client == nil {
		return result, fmt.Errorf("hosted sync client not configured")
	}
	if r.ProjectRoot == "" || r.ChannelID == "" || r.MachineID == "" {
		return result, fmt.Errorf("hosted sync missing project/channel/machine configuration")
	}
	if r.BatchSize <= 0 {
		r.BatchSize = defaultBatchSize
	}

	state, err := LoadState(r.ProjectRoot)
	if err != nil {
		return result, err
	}
	if state.Streams == nil {
		state.Streams = map[string]StreamCursor{}
	}
	if state.ChannelID == "" {
		state.ChannelID = r.ChannelID
	} else if state.ChannelID != r.ChannelID {
		return result, fmt.Errorf("sync state channel mismatch: %s != %s", state.ChannelID, r.ChannelID)
	}

	manifest, err := r.Client.Manifest(ctx, r.ChannelID)
	if err != nil {
		return result, err
	}
	manifestByKey := map[string]ManifestEntry{}
	for _, entry := range manifest.Streams {
		manifestByKey[StreamKey(entry.MachineID, entry.File)] = entry
	}

	if err := r.adoptManifestForLocal(state, manifestByKey); err != nil {
		return result, err
	}

	pushResult, baseMismatch, err := r.pushLocal(ctx, state)
	if err != nil && !errors.Is(err, ErrBaseMismatch) {
		return result, err
	}
	result.add(pushResult)

	pullResult, err := r.pull(ctx, state, manifest.Streams, manifestByKey)
	if err != nil {
		return result, err
	}
	result.add(pullResult)

	if baseMismatch {
		retryResult, _, err := r.pushLocal(ctx, state)
		if err != nil && !errors.Is(err, ErrBaseMismatch) {
			return result, err
		}
		result.add(retryResult)
	}

	if err := SaveState(r.ProjectRoot, state); err != nil {
		return result, err
	}

	return result, nil
}

// Run executes sync in a loop until the context is canceled.
func (r *Runner) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if _, err := r.SyncOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) adoptManifestForLocal(state *State, manifest map[string]ManifestEntry) error {
	localDir := r.localMachineDir()
	if localDir == "" {
		return nil
	}
	for _, file := range sharedFiles {
		key := StreamKey(r.MachineID, file)
		cursor, hasCursor := state.Streams[key]
		entry, ok := manifest[key]
		if !ok || entry.LineCount == 0 || entry.SHA256 == "" {
			continue
		}
		if hasCursor && cursor.Line >= entry.LineCount {
			continue
		}
		path := filepath.Join(localDir, file)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		sum, lines, _, err := db.ComputeChecksum(path)
		if err != nil {
			r.logf("checksum failed for %s: %v", path, err)
			continue
		}
		if lines == entry.LineCount && strings.EqualFold(sum, entry.SHA256) {
			state.Streams[key] = StreamCursor{
				Line:    entry.LineCount,
				SHA256:  entry.SHA256,
				LastSeq: entry.LastSeq,
			}
		}
	}
	return nil
}

func (r *Runner) pushLocal(ctx context.Context, state *State) (SyncResult, bool, error) {
	result := SyncResult{}
	baseMismatch := false

	localDir := r.localMachineDir()
	if localDir == "" {
		return result, false, fmt.Errorf("local machine directory not found")
	}

	for _, file := range sharedFiles {
		path := filepath.Join(localDir, file)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return result, baseMismatch, err
		}
		pushed, err := r.pushFile(ctx, state, file, path)
		if err != nil {
			if errors.Is(err, ErrBaseMismatch) {
				baseMismatch = true
				result.BaseMismatches++
				continue
			}
			return result, baseMismatch, err
		}
		if pushed > 0 {
			result.PushedLines += pushed
			result.PushedStreams++
		}
	}

	return result, baseMismatch, nil
}

func (r *Runner) pushFile(ctx context.Context, state *State, fileName, path string) (int, error) {
	key := StreamKey(r.MachineID, fileName)
	cursor := state.Streams[key]
	start := cursor.Line
	pushed := 0

	for {
		lines, hasMore, err := readJSONLBatch(path, start, r.BatchSize)
		if err != nil {
			return pushed, err
		}
		if len(lines) == 0 {
			break
		}
		req := PushRequest{
			ChannelID: r.ChannelID,
			MachineID: r.MachineID,
			File:      fileName,
			Base: PushBase{
				LineCount: cursor.Line,
				SHA256:    cursor.SHA256,
				LastSeq:   cursor.LastSeq,
			},
			Lines:          lines,
			IdempotencyKey: newIdempotencyKey(),
		}
		resp, err := r.Client.Push(ctx, req)
		if err != nil {
			return pushed, err
		}
		cursor.Line = resp.NewLineCount
		cursor.SHA256 = resp.NewSHA256
		cursor.LastSeq = resp.NewLastSeq
		state.Streams[key] = cursor
		pushed += len(lines)
		start = cursor.Line
		if !hasMore {
			break
		}
	}

	return pushed, nil
}

func (r *Runner) pull(ctx context.Context, state *State, manifest []ManifestEntry, manifestByKey map[string]ManifestEntry) (SyncResult, error) {
	result := SyncResult{}
	if len(manifest) == 0 {
		return result, nil
	}
	cursors := make([]PullCursor, 0, len(manifest))
	for _, entry := range manifest {
		key := StreamKey(entry.MachineID, entry.File)
		cursor := state.Streams[key]
		cursors = append(cursors, PullCursor{
			MachineID:  entry.MachineID,
			File:       entry.File,
			LineOffset: cursor.Line,
		})
	}
	resp, err := r.Client.Pull(ctx, PullRequest{
		ChannelID: r.ChannelID,
		Cursors:   cursors,
	})
	if err != nil {
		return result, err
	}

	for _, update := range resp.Updates {
		key := StreamKey(update.MachineID, update.File)
		if len(update.Lines) > 0 {
			path := r.sharedStreamPath(update.MachineID, update.File)
			if err := db.AppendSharedJSONLLines(r.ProjectRoot, path, update.Lines); err != nil {
				return result, err
			}
			result.PulledLines += len(update.Lines)
			result.PulledStreams++
		}
		cursor := state.Streams[key]
		cursor.Line = update.NewOffset
		if entry, ok := manifestByKey[key]; ok && entry.LineCount == update.NewOffset {
			cursor.SHA256 = entry.SHA256
			cursor.LastSeq = entry.LastSeq
		}
		state.Streams[key] = cursor
	}

	return result, nil
}

func (r *Runner) sharedStreamPath(machineID, file string) string {
	return filepath.Join(r.ProjectRoot, ".fray", "shared", "machines", machineID, file)
}

func (r *Runner) localMachineDir() string {
	if r.ProjectRoot == "" || r.MachineID == "" {
		return ""
	}
	return filepath.Join(r.ProjectRoot, ".fray", "shared", "machines", r.MachineID)
}

func (r *Runner) logf(format string, args ...any) {
	if r.Logger == nil {
		return
	}
	r.Logger.Printf(format, args...)
}

func (r *SyncResult) add(other SyncResult) {
	r.PushedLines += other.PushedLines
	r.PulledLines += other.PulledLines
	r.PushedStreams += other.PushedStreams
	r.PulledStreams += other.PulledStreams
	r.BaseMismatches += other.BaseMismatches
}

func readJSONLBatch(path string, startLine int64, maxLines int) ([]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	if maxLines <= 0 {
		return nil, false, nil
	}

	endsWithNewline, _ := fileEndsWithNewline(file)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lines := make([]string, 0, maxLines)
	var idx int64
	for scanner.Scan() {
		if idx < startLine {
			idx++
			continue
		}
		if len(lines) >= maxLines {
			break
		}
		line := scanner.Text()
		if !json.Valid([]byte(line)) {
			if !endsWithNewline {
				return lines, false, nil
			}
			return nil, false, fmt.Errorf("invalid jsonl line at %s:%d", path, idx+1)
		}
		lines = append(lines, line)
		idx++
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(lines) == maxLines
	return lines, hasMore, nil
}

func newIdempotencyKey() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("sync-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func fileEndsWithNewline(file *os.File) (bool, error) {
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return true, nil
	}
	buf := make([]byte, 1)
	if _, err := file.ReadAt(buf, info.Size()-1); err != nil {
		return false, err
	}
	return buf[0] == '\n', nil
}
