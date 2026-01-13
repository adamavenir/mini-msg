package aap

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileRegistry implements Registry using filesystem storage.
type FileRegistry struct {
	basePath string // e.g., ~/.config/aap/agents/
}

// NewFileRegistry creates a registry backed by filesystem storage.
// basePath should be the agents directory (e.g., ~/.config/aap/agents/).
func NewFileRegistry(basePath string) (*FileRegistry, error) {
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create base path: %w", err)
	}
	return &FileRegistry{basePath: basePath}, nil
}

// agentDir returns the directory path for an agent.
func (r *FileRegistry) agentDir(agent string) string {
	return filepath.Join(r.basePath, agent)
}

// identityPath returns the path to an agent's identity.json.
func (r *FileRegistry) identityPath(agent string) string {
	return filepath.Join(r.agentDir(agent), "identity.json")
}

// privateKeyPath returns the path to an agent's private.key.
func (r *FileRegistry) privateKeyPath(agent string) string {
	return filepath.Join(r.agentDir(agent), "private.key")
}

// Register creates a new identity for the given agent name.
func (r *FileRegistry) Register(agent string, opts RegisterOpts) (*Identity, error) {
	// Validate agent name
	if err := validateName(agent, "agent"); err != nil {
		return nil, err
	}

	// Check if agent already exists
	if _, err := r.Get(agent); err == nil {
		return nil, fmt.Errorf("agent %q already registered", agent)
	}

	// Create agent directory
	agentDir := r.agentDir(agent)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return nil, fmt.Errorf("create agent directory: %w", err)
	}

	var publicKey ed25519.PublicKey
	var privateKey ed25519.PrivateKey

	if opts.GenerateKey {
		if len(opts.Passphrase) == 0 {
			return nil, fmt.Errorf("passphrase required when generating key")
		}

		var err error
		publicKey, privateKey, err = GenerateKeyPair()
		if err != nil {
			return nil, fmt.Errorf("generate keypair: %w", err)
		}

		// Encrypt and save private key
		ekf, err := EncryptPrivateKey(privateKey, opts.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("encrypt private key: %w", err)
		}

		keyData, err := json.MarshalIndent(ekf, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal encrypted key: %w", err)
		}

		if err := os.WriteFile(r.privateKeyPath(agent), keyData, 0600); err != nil {
			return nil, fmt.Errorf("write private key: %w", err)
		}
	}

	// Create identity record
	record := NewIdentityRecord(agent, publicKey, opts.Metadata)

	// Save identity
	identityData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal identity: %w", err)
	}

	if err := os.WriteFile(r.identityPath(agent), identityData, 0644); err != nil {
		return nil, fmt.Errorf("write identity: %w", err)
	}

	return record.ToIdentity()
}

// Get retrieves an identity by agent name.
func (r *FileRegistry) Get(agent string) (*Identity, error) {
	identityPath := r.identityPath(agent)

	data, err := os.ReadFile(identityPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent %q not found", agent)
		}
		return nil, fmt.Errorf("read identity: %w", err)
	}

	var record IdentityRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}

	return record.ToIdentity()
}

// List returns all registered identities.
func (r *FileRegistry) List() ([]*Identity, error) {
	entries, err := os.ReadDir(r.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agents directory: %w", err)
	}

	var identities []*Identity
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		identity, err := r.Get(entry.Name())
		if err != nil {
			// Skip entries that aren't valid identities
			continue
		}
		identities = append(identities, identity)
	}

	return identities, nil
}

// Sign signs data using the agent's private key.
func (r *FileRegistry) Sign(agent string, passphrase []byte, data []byte) ([]byte, error) {
	// Get identity to verify agent exists and has a key
	identity, err := r.Get(agent)
	if err != nil {
		return nil, err
	}

	if !identity.HasKey {
		return nil, fmt.Errorf("agent %q has no private key", agent)
	}

	// Load encrypted private key
	keyData, err := os.ReadFile(r.privateKeyPath(agent))
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	var ekf EncryptedKeyFile
	if err := json.Unmarshal(keyData, &ekf); err != nil {
		return nil, fmt.Errorf("parse private key file: %w", err)
	}

	// Decrypt private key
	privateKey, err := DecryptPrivateKey(&ekf, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt private key: %w", err)
	}

	// Sign the data
	return Sign(privateKey, data), nil
}

// Verify checks a signature against an agent's public key.
func (r *FileRegistry) Verify(agent string, data, signature []byte) (bool, error) {
	identity, err := r.Get(agent)
	if err != nil {
		return false, err
	}

	if !identity.HasKey {
		return false, fmt.Errorf("agent %q has no public key", agent)
	}

	return Verify(identity.PublicKey, data, signature), nil
}

// Delete removes an agent's identity and keys.
func (r *FileRegistry) Delete(agent string) error {
	agentDir := r.agentDir(agent)

	// Check if agent exists
	if _, err := os.Stat(agentDir); os.IsNotExist(err) {
		return fmt.Errorf("agent %q not found", agent)
	}

	return os.RemoveAll(agentDir)
}

// Exists checks if an agent is registered.
func (r *FileRegistry) Exists(agent string) bool {
	_, err := r.Get(agent)
	return err == nil
}
