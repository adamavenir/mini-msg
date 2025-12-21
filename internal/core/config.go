package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GlobalConfig stores registered channels.
type GlobalConfig struct {
	Version  int                         `json:"version"`
	Channels map[string]GlobalChannelRef `json:"channels"`
}

// GlobalChannelRef stores channel metadata in the global config.
type GlobalChannelRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "mm")
	return filepath.Join(configDir, "mm-config.json"), nil
}

func ensureConfigDir() (string, error) {
	path, err := globalConfigPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// ReadGlobalConfig reads the global config file if present.
func ReadGlobalConfig() (*GlobalConfig, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var config GlobalConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if config.Channels == nil {
		config.Channels = map[string]GlobalChannelRef{}
	}
	return &config, nil
}

// WriteGlobalConfig writes the global config to disk.
func WriteGlobalConfig(config GlobalConfig) error {
	path, err := ensureConfigDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// RegisterChannel adds or updates a channel in the global config.
func RegisterChannel(channelID, channelName, projectRoot string) (*GlobalConfig, error) {
	config, err := ReadGlobalConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		config = &GlobalConfig{Version: 1, Channels: map[string]GlobalChannelRef{}}
	}
	if config.Version == 0 {
		config.Version = 1
	}
	if config.Channels == nil {
		config.Channels = map[string]GlobalChannelRef{}
	}

	config.Channels[channelID] = GlobalChannelRef{
		Name: channelName,
		Path: projectRoot,
	}

	if err := WriteGlobalConfig(*config); err != nil {
		return nil, err
	}
	return config, nil
}

// FindChannelByRef resolves by ID or name.
func FindChannelByRef(ref string, config *GlobalConfig) (string, GlobalChannelRef, bool) {
	if config == nil {
		return "", GlobalChannelRef{}, false
	}
	if channel, ok := config.Channels[ref]; ok {
		return ref, channel, true
	}
	for id, channel := range config.Channels {
		if channel.Name == ref {
			return id, channel, true
		}
	}
	return "", GlobalChannelRef{}, false
}
