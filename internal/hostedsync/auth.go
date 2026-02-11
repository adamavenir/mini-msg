package hostedsync

import "path/filepath"

const authFileName = "hosted-sync.json"

// Auth stores local-only hosted sync credentials.
type Auth struct {
	ChannelID    string `json:"channel_id"`
	MachineID    string `json:"machine_id"`
	HostedURL    string `json:"hosted_url"`
	Token        string `json:"token"`
	RegisteredAt int64  `json:"registered_at,omitempty"`
}

func authPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".fray", "local", authFileName)
}

// LoadAuth reads hosted sync credentials if present.
func LoadAuth(projectRoot string) (*Auth, error) {
	path := authPath(projectRoot)
	var auth Auth
	ok, err := readJSON(path, &auth)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &auth, nil
}

// SaveAuth writes hosted sync credentials.
func SaveAuth(projectRoot string, auth Auth) error {
	return writeJSONAtomic(authPath(projectRoot), auth)
}
