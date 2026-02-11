package db

// ProjectKnownAgent stores per-project known-agent data.
type ProjectKnownAgent struct {
	Name        *string  `json:"name,omitempty"`
	GlobalName  *string  `json:"global_name,omitempty"`
	HomeChannel *string  `json:"home_channel,omitempty"`
	CreatedAt   *string  `json:"created_at,omitempty"`
	FirstSeen   *string  `json:"first_seen,omitempty"`
	Status      *string  `json:"status,omitempty"`
	Nicks       []string `json:"nicks,omitempty"`
}

// ProjectSyncConfig stores sync backend configuration for shared storage.
type ProjectSyncConfig struct {
	Backend   string `json:"backend,omitempty"`
	Path      string `json:"path,omitempty"`
	HostedURL string `json:"hosted_url,omitempty"`
}

// ProjectConfig represents the per-project config file.
type ProjectConfig struct {
	Version        int                          `json:"version"`
	StorageVersion int                          `json:"storage_version,omitempty"`
	ChannelID      string                       `json:"channel_id,omitempty"`
	ChannelName    string                       `json:"channel_name,omitempty"`
	CreatedAt      string                       `json:"created_at,omitempty"`
	KnownAgents    map[string]ProjectKnownAgent `json:"known_agents,omitempty"`
	MachineAliases map[string]string            `json:"machine_aliases,omitempty"`
	Sync           *ProjectSyncConfig           `json:"sync,omitempty"`
}
type machineIDFile struct {
	ID        string `json:"id"`
	Seq       int64  `json:"seq"`
	CreatedAt int64  `json:"created_at"`
}

// GetStorageVersion returns the storage_version from fray-config.json.
// Defaults to 1 when config is missing or storage_version is unset.
