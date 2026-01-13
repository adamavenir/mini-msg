package aap

import (
	"crypto/ed25519"
	"encoding/base64"
	"time"
)

// Version is the current AAP specification version.
const Version = "0.1.0"

// IdentityRecord is the wire format for identity.json.
// Matches AAP-SPEC.md section 2.2.
type IdentityRecord struct {
	Version   string            `json:"version"`              // "0.1.0"
	Type      string            `json:"type"`                 // always "identity"
	GUID      string            `json:"guid"`                 // "aap-a1b2c3d4e5f6g7h8"
	Address   string            `json:"address"`              // "@dev"
	Agent     string            `json:"agent"`                // "dev"
	PublicKey *PublicKeyRecord  `json:"public_key,omitempty"` // nil for keyless mode
	CreatedAt string            `json:"created_at"`           // RFC 3339 timestamp
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// PublicKeyRecord holds a public key in wire format.
type PublicKeyRecord struct {
	Algorithm string `json:"algorithm"` // "ed25519"
	Key       string `json:"key"`       // base64-encoded public key
}

// Identity is the internal type with decoded key and convenience methods.
type Identity struct {
	Record    IdentityRecord
	PublicKey ed25519.PublicKey // decoded from Record.PublicKey.Key
	HasKey    bool              // false for legacy/keyless identities
}

// DecodePublicKey decodes the base64 public key from the record.
func (r *PublicKeyRecord) DecodePublicKey() (ed25519.PublicKey, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(r.Key)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(keyBytes), nil
}

// EncodePublicKey creates a PublicKeyRecord from an Ed25519 public key.
func EncodePublicKey(key ed25519.PublicKey) *PublicKeyRecord {
	return &PublicKeyRecord{
		Algorithm: "ed25519",
		Key:       base64.StdEncoding.EncodeToString(key),
	}
}

// NewIdentityRecord creates a new identity record for an agent.
func NewIdentityRecord(agent string, publicKey ed25519.PublicKey, metadata map[string]string) IdentityRecord {
	record := IdentityRecord{
		Version:   Version,
		Type:      "identity",
		GUID:      NewGUID(),
		Address:   "@" + agent,
		Agent:     agent,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Metadata:  metadata,
	}

	if publicKey != nil {
		record.PublicKey = EncodePublicKey(publicKey)
	}

	return record
}

// ToIdentity converts a record to an Identity with decoded public key.
func (r IdentityRecord) ToIdentity() (*Identity, error) {
	identity := &Identity{
		Record: r,
	}

	if r.PublicKey != nil {
		pubKey, err := r.PublicKey.DecodePublicKey()
		if err != nil {
			return nil, err
		}
		identity.PublicKey = pubKey
		identity.HasKey = true
	}

	return identity, nil
}

// RegisterOpts controls identity registration.
type RegisterOpts struct {
	GenerateKey bool              // true to generate Ed25519 keypair
	Passphrase  []byte            // required if GenerateKey is true
	Metadata    map[string]string // optional display_name, description, etc.
}

// Registry provides identity management operations.
type Registry interface {
	// Register creates a new identity for the given agent name.
	Register(agent string, opts RegisterOpts) (*Identity, error)

	// Get retrieves an identity by agent name.
	Get(agent string) (*Identity, error)

	// List returns all registered identities.
	List() ([]*Identity, error)

	// Sign signs data using the agent's private key.
	// Returns error if agent has no key or passphrase is wrong.
	Sign(agent string, passphrase []byte, data []byte) ([]byte, error)

	// Verify checks a signature against an agent's public key.
	Verify(agent string, data, signature []byte) (bool, error)
}
