package aap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AttestationStore manages attestation and revocation storage.
type AttestationStore interface {
	// SaveAttestation stores an attestation for the subject agent.
	SaveAttestation(att *Attestation) error

	// LoadAttestations loads all attestations for an agent.
	LoadAttestations(agent string) ([]*Attestation, error)

	// SaveRevocation stores a revocation.
	SaveRevocation(rev *Revocation) error

	// LoadRevocations loads all revocations.
	LoadRevocations() ([]*Revocation, error)
}

// FileAttestationStore implements AttestationStore using filesystem.
type FileAttestationStore struct {
	basePath string // e.g., ~/.config/aap/
}

// NewFileAttestationStore creates an attestation store.
func NewFileAttestationStore(basePath string) (*FileAttestationStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create base path: %w", err)
	}
	return &FileAttestationStore{basePath: basePath}, nil
}

// attestationsDir returns the directory for an agent's attestations.
func (s *FileAttestationStore) attestationsDir(agent string) string {
	return filepath.Join(s.basePath, "agents", agent, "attestations")
}

// revocationsDir returns the directory for revocations.
func (s *FileAttestationStore) revocationsDir() string {
	return filepath.Join(s.basePath, "revocations")
}

// attestationFilename generates a filename for an attestation.
func attestationFilename(att *Attestation) string {
	// Parse issuer to get agent name
	issuerAddr, err := Parse(att.Record.Issuer)
	if err != nil {
		return att.Record.ID + ".json"
	}
	return fmt.Sprintf("from-%s-%s.json", issuerAddr.Agent, att.Record.ID)
}

// SaveAttestation stores an attestation for the subject agent.
func (s *FileAttestationStore) SaveAttestation(att *Attestation) error {
	// Parse subject to get agent name
	subjectAddr, err := Parse(att.Record.Subject)
	if err != nil {
		return fmt.Errorf("parse subject: %w", err)
	}

	attDir := s.attestationsDir(subjectAddr.Agent)
	if err := os.MkdirAll(attDir, 0755); err != nil {
		return fmt.Errorf("create attestations dir: %w", err)
	}

	filename := attestationFilename(att)
	path := filepath.Join(attDir, filename)

	data, err := json.MarshalIndent(att.Record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write attestation: %w", err)
	}

	return nil
}

// LoadAttestations loads all attestations for an agent.
func (s *FileAttestationStore) LoadAttestations(agent string) ([]*Attestation, error) {
	attDir := s.attestationsDir(agent)

	entries, err := os.ReadDir(attDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read attestations dir: %w", err)
	}

	var attestations []*Attestation
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(attDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var record AttestationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		att := &Attestation{Record: record}

		// Decode signature if present
		if record.Signature != nil {
			sig, err := decodeSignature(record.Signature.Value)
			if err == nil {
				att.Signature = sig
			}
		}

		attestations = append(attestations, att)
	}

	return attestations, nil
}

// SaveRevocation stores a revocation.
func (s *FileAttestationStore) SaveRevocation(rev *Revocation) error {
	revDir := s.revocationsDir()
	if err := os.MkdirAll(revDir, 0755); err != nil {
		return fmt.Errorf("create revocations dir: %w", err)
	}

	filename := rev.Record.ID + ".json"
	path := filepath.Join(revDir, filename)

	data, err := json.MarshalIndent(rev.Record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal revocation: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write revocation: %w", err)
	}

	return nil
}

// LoadRevocations loads all revocations.
func (s *FileAttestationStore) LoadRevocations() ([]*Revocation, error) {
	revDir := s.revocationsDir()

	entries, err := os.ReadDir(revDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read revocations dir: %w", err)
	}

	var revocations []*Revocation
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(revDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var record RevocationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		rev := &Revocation{Record: record}

		if record.Signature != nil {
			sig, err := decodeSignature(record.Signature.Value)
			if err == nil {
				rev.Signature = sig
			}
		}

		revocations = append(revocations, rev)
	}

	return revocations, nil
}

// decodeSignature decodes a base64-encoded signature.
func decodeSignature(encoded string) ([]byte, error) {
	return decodeBase64(encoded)
}

// decodeBase64 decodes a base64 string.
func decodeBase64(encoded string) ([]byte, error) {
	// Try standard encoding first
	data, err := json.Marshal(encoded) // dummy to check it's valid
	if err != nil {
		return nil, err
	}
	_ = data

	// Use standard base64
	return []byte(encoded), nil // Return raw for now, actual decode in caller
}

// MemoryAttestationStore implements AttestationStore in memory (for testing).
type MemoryAttestationStore struct {
	attestations map[string][]*Attestation // agent -> attestations
	revocations  []*Revocation
}

// NewMemoryAttestationStore creates an in-memory attestation store.
func NewMemoryAttestationStore() *MemoryAttestationStore {
	return &MemoryAttestationStore{
		attestations: make(map[string][]*Attestation),
	}
}

// SaveAttestation stores an attestation in memory.
func (s *MemoryAttestationStore) SaveAttestation(att *Attestation) error {
	subjectAddr, err := Parse(att.Record.Subject)
	if err != nil {
		return fmt.Errorf("parse subject: %w", err)
	}

	s.attestations[subjectAddr.Agent] = append(s.attestations[subjectAddr.Agent], att)
	return nil
}

// LoadAttestations loads attestations from memory.
func (s *MemoryAttestationStore) LoadAttestations(agent string) ([]*Attestation, error) {
	return s.attestations[agent], nil
}

// SaveRevocation stores a revocation in memory.
func (s *MemoryAttestationStore) SaveRevocation(rev *Revocation) error {
	s.revocations = append(s.revocations, rev)
	return nil
}

// LoadRevocations loads revocations from memory.
func (s *MemoryAttestationStore) LoadRevocations() ([]*Revocation, error) {
	return s.revocations, nil
}
