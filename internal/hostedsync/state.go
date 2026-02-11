package hostedsync

import "path/filepath"

const stateFileName = "sync-state.json"

// StreamCursor tracks the last synced position for a stream.
type StreamCursor struct {
	Line    int64  `json:"line"`
	SHA256  string `json:"sha256,omitempty"`
	LastSeq int64  `json:"last_seq,omitempty"`
}

// State stores per-stream cursors for hosted sync.
type State struct {
	ChannelID string                  `json:"channel_id,omitempty"`
	Streams   map[string]StreamCursor `json:"streams"`
}

func statePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".fray", "local", stateFileName)
}

// StreamKey returns the canonical map key for a stream.
func StreamKey(machineID, file string) string {
	return machineID + "/" + file
}

// LoadState reads or initializes sync state.
func LoadState(projectRoot string) (*State, error) {
	path := statePath(projectRoot)
	var state State
	ok, err := readJSON(path, &state)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &State{Streams: map[string]StreamCursor{}}, nil
	}
	if state.Streams == nil {
		state.Streams = map[string]StreamCursor{}
	}
	return &state, nil
}

// SaveState writes sync state to disk.
func SaveState(projectRoot string, state *State) error {
	if state == nil {
		return nil
	}
	if state.Streams == nil {
		state.Streams = map[string]StreamCursor{}
	}
	return writeJSONAtomic(statePath(projectRoot), state)
}
