package aap

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Standard capabilities from AAP-SPEC.md section 5.3
const (
	CapabilityRead     = "read"
	CapabilityWrite    = "write"
	CapabilityExecute  = "execute"
	CapabilityDeploy   = "deploy"
	CapabilityDelegate = "delegate"
	CapabilityAdmin    = "admin"
)

// MaxTrustChainDepth is the maximum depth for trust chain traversal.
const MaxTrustChainDepth = 5

// SignatureRecord holds a cryptographic signature in wire format.
type SignatureRecord struct {
	Algorithm string `json:"algorithm"` // "ed25519"
	Value     string `json:"value"`     // base64-encoded signature
}

// Conditions holds optional constraints on an attestation.
type Conditions struct {
	RequireVariants       []string `json:"require_variants,omitempty"`
	ExcludeExecutionHosts []string `json:"exclude_execution_hosts,omitempty"`
	RequireExecutionHosts []string `json:"require_execution_hosts,omitempty"`
}

// AttestationRecord is the wire format for trust attestations.
// Matches AAP-SPEC.md section 5.2.
type AttestationRecord struct {
	Version      string           `json:"version"`                  // "0.1.0"
	Type         string           `json:"type"`                     // "trust_attestation"
	ID           string           `json:"id"`                       // "att-x1y2z3w4..."
	Subject      string           `json:"subject"`                  // "@dev"
	SubjectGUID  string           `json:"subject_guid"`             // "aap-a1b2c3d4..."
	SubjectKeyID string           `json:"subject_key_id,omitempty"` // "sha256:abc123..."
	Issuer       string           `json:"issuer"`                   // "@adam"
	IssuerGUID   string           `json:"issuer_guid"`
	IssuerKeyID  string           `json:"issuer_key_id"`
	Capabilities []string         `json:"capabilities"` // ["read", "write", "deploy"]
	Scope        string           `json:"scope"`        // "github.com/adamavenir/*"
	Conditions   *Conditions      `json:"conditions,omitempty"`
	IssuedAt     string           `json:"issued_at"` // RFC 3339
	ExpiresAt    *string          `json:"expires_at,omitempty"`
	Signature    *SignatureRecord `json:"signature"`
}

// Attestation is the internal type with decoded signature.
type Attestation struct {
	Record    AttestationRecord
	Signature []byte // decoded from Record.Signature.Value
	Verified  bool   // true if signature has been verified
}

// TrustClaim is input for creating a new attestation.
type TrustClaim struct {
	Subject      string
	Capabilities []string
	Scope        string
	Conditions   *Conditions
	ExpiresIn    time.Duration // optional, converted to ExpiresAt
}

// VerifyResult is returned from trust verification.
type VerifyResult struct {
	Verified     bool
	Attestations []*Attestation // chain of attestations that granted trust
	Reason       string         // if not verified, why
}

// RevocationRecord is the wire format for revocations.
type RevocationRecord struct {
	Version       string           `json:"version"`        // "0.1.0"
	Type          string           `json:"type"`           // "revocation"
	ID            string           `json:"id"`             // "rev-x1y2z3w4..."
	AttestationID string           `json:"attestation_id"` // ID of revoked attestation
	Issuer        string           `json:"issuer"`
	IssuerGUID    string           `json:"issuer_guid"`
	IssuerKeyID   string           `json:"issuer_key_id"`
	Reason        string           `json:"reason"`
	RevokedAt     string           `json:"revoked_at"`
	Signature     *SignatureRecord `json:"signature"`
}

// Revocation is the internal type for revocations.
type Revocation struct {
	Record    RevocationRecord
	Signature []byte
}

// NewAttestationID generates a new attestation ID.
func NewAttestationID() string {
	guid := NewGUID()
	// Replace aap- prefix with att-
	return "att-" + guid[4:]
}

// NewRevocationID generates a new revocation ID.
func NewRevocationID() string {
	guid := NewGUID()
	return "rev-" + guid[4:]
}

// Attest creates a signed attestation from issuer to subject.
func Attest(registry Registry, issuer string, passphrase []byte, claim TrustClaim) (*Attestation, error) {
	// Get issuer identity
	issuerIdentity, err := registry.Get(issuer)
	if err != nil {
		return nil, fmt.Errorf("get issuer identity: %w", err)
	}
	if !issuerIdentity.HasKey {
		return nil, fmt.Errorf("issuer %q has no signing key", issuer)
	}

	// Parse subject address
	subjectAddr, err := Parse(claim.Subject)
	if err != nil {
		return nil, fmt.Errorf("parse subject address: %w", err)
	}

	// Get subject identity
	subjectIdentity, err := registry.Get(subjectAddr.Agent)
	if err != nil {
		return nil, fmt.Errorf("get subject identity: %w", err)
	}

	// Build attestation record
	now := time.Now().UTC()
	record := AttestationRecord{
		Version:      Version,
		Type:         "trust_attestation",
		ID:           NewAttestationID(),
		Subject:      claim.Subject,
		SubjectGUID:  subjectIdentity.Record.GUID,
		Issuer:       "@" + issuer,
		IssuerGUID:   issuerIdentity.Record.GUID,
		IssuerKeyID:  KeyFingerprint(issuerIdentity.PublicKey),
		Capabilities: claim.Capabilities,
		Scope:        claim.Scope,
		Conditions:   claim.Conditions,
		IssuedAt:     now.Format(time.RFC3339),
	}

	// Set subject key ID if available
	if subjectIdentity.HasKey {
		record.SubjectKeyID = KeyFingerprint(subjectIdentity.PublicKey)
	}

	// Set expiration if specified
	if claim.ExpiresIn != 0 {
		expiresAt := now.Add(claim.ExpiresIn).Format(time.RFC3339)
		record.ExpiresAt = &expiresAt
	}

	// Canonicalize for signing (without signature field)
	canonical, err := CanonicalizeForSigning(record)
	if err != nil {
		return nil, fmt.Errorf("canonicalize: %w", err)
	}

	// Sign
	sig, err := registry.Sign(issuer, passphrase, canonical)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	record.Signature = &SignatureRecord{
		Algorithm: "ed25519",
		Value:     base64.StdEncoding.EncodeToString(sig),
	}

	return &Attestation{
		Record:    record,
		Signature: sig,
		Verified:  true, // We just created it
	}, nil
}

// VerifyAttestation checks a single attestation's signature.
func VerifyAttestation(registry Registry, att *Attestation) error {
	if att.Record.Signature == nil {
		return fmt.Errorf("attestation has no signature")
	}

	// Parse issuer address
	issuerAddr, err := Parse(att.Record.Issuer)
	if err != nil {
		return fmt.Errorf("parse issuer: %w", err)
	}

	// Get issuer identity
	issuerIdentity, err := registry.Get(issuerAddr.Agent)
	if err != nil {
		return fmt.Errorf("get issuer identity: %w", err)
	}

	// Verify issuer GUID binding
	if issuerIdentity.Record.GUID != att.Record.IssuerGUID {
		return fmt.Errorf("issuer GUID mismatch: expected %s, got %s",
			att.Record.IssuerGUID, issuerIdentity.Record.GUID)
	}

	// Verify issuer key ID binding
	if issuerIdentity.HasKey {
		expectedKeyID := KeyFingerprint(issuerIdentity.PublicKey)
		if expectedKeyID != att.Record.IssuerKeyID {
			return fmt.Errorf("issuer key ID mismatch: expected %s, got %s",
				att.Record.IssuerKeyID, expectedKeyID)
		}
	}

	// Parse subject address
	subjectAddr, err := Parse(att.Record.Subject)
	if err != nil {
		return fmt.Errorf("parse subject: %w", err)
	}

	// Get subject identity
	subjectIdentity, err := registry.Get(subjectAddr.Agent)
	if err != nil {
		return fmt.Errorf("get subject identity: %w", err)
	}

	// Verify subject GUID binding
	if subjectIdentity.Record.GUID != att.Record.SubjectGUID {
		return fmt.Errorf("subject GUID mismatch: expected %s, got %s",
			att.Record.SubjectGUID, subjectIdentity.Record.GUID)
	}

	// Verify subject key ID binding (if both have keys)
	if subjectIdentity.HasKey && att.Record.SubjectKeyID != "" {
		expectedKeyID := KeyFingerprint(subjectIdentity.PublicKey)
		if expectedKeyID != att.Record.SubjectKeyID {
			return fmt.Errorf("subject key ID mismatch: expected %s, got %s",
				att.Record.SubjectKeyID, expectedKeyID)
		}
	}

	// Canonicalize for verification
	canonical, err := CanonicalizeForSigning(att.Record)
	if err != nil {
		return fmt.Errorf("canonicalize: %w", err)
	}

	// Decode signature
	sig, err := base64.StdEncoding.DecodeString(att.Record.Signature.Value)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	// Verify signature
	ok, err := registry.Verify(issuerAddr.Agent, canonical, sig)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if !ok {
		return fmt.Errorf("signature verification failed")
	}

	att.Signature = sig
	att.Verified = true
	return nil
}

// VerifyTrust checks if an agent has a capability in a scope.
func VerifyTrust(resolver Resolver, store AttestationStore, addr string, capability string, scope string) (*VerifyResult, error) {
	return verifyTrustWithDepth(resolver, store, addr, capability, scope, 0, nil)
}

func verifyTrustWithDepth(resolver Resolver, store AttestationStore, addr string, capability string, scope string, depth int, seen map[string]bool) (*VerifyResult, error) {
	if depth >= MaxTrustChainDepth {
		return &VerifyResult{
			Verified: false,
			Reason:   "max trust chain depth exceeded",
		}, nil
	}

	if seen == nil {
		seen = make(map[string]bool)
	}

	// Parse address
	parsedAddr, err := Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	// Check for cycles
	if seen[parsedAddr.Agent] {
		return &VerifyResult{
			Verified: false,
			Reason:   "cycle detected in trust chain",
		}, nil
	}
	seen[parsedAddr.Agent] = true

	// Resolve identity
	resolution, err := resolver.Resolve(addr)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}

	// Untrusted identities cannot be verified
	if resolution.TrustLevel == TrustLevelUntrusted {
		return &VerifyResult{
			Verified: false,
			Reason:   "identity has no trusted key (TrustLevel=untrusted)",
		}, nil
	}

	// Load attestations for this agent
	attestations, err := store.LoadAttestations(parsedAddr.Agent)
	if err != nil {
		return nil, fmt.Errorf("load attestations: %w", err)
	}

	// Load revocations
	revocations, err := store.LoadRevocations()
	if err != nil {
		return nil, fmt.Errorf("load revocations: %w", err)
	}
	revokedIDs := make(map[string]bool)
	for _, rev := range revocations {
		revokedIDs[rev.Record.AttestationID] = true
	}

	// Find attestations that grant the requested capability
	for _, att := range attestations {
		// Check if revoked
		if revokedIDs[att.Record.ID] {
			continue
		}

		// Check if expired
		if att.Record.ExpiresAt != nil {
			expiresAt, err := time.Parse(time.RFC3339, *att.Record.ExpiresAt)
			if err == nil && time.Now().After(expiresAt) {
				continue // expired
			}
		}

		// Check capability
		if !hasCapability(att.Record.Capabilities, capability) {
			continue
		}

		// Check scope
		if !scopeMatches(att.Record.Scope, scope) {
			continue
		}

		// Check conditions
		if !conditionsMatch(att.Record.Conditions, parsedAddr, resolution) {
			continue
		}

		// This attestation grants the capability
		return &VerifyResult{
			Verified:     true,
			Attestations: []*Attestation{att},
		}, nil
	}

	// No direct attestation found - check for delegated trust
	for _, att := range attestations {
		// Check if revoked
		if revokedIDs[att.Record.ID] {
			continue
		}

		// Check if expired
		if att.Record.ExpiresAt != nil {
			expiresAt, err := time.Parse(time.RFC3339, *att.Record.ExpiresAt)
			if err == nil && time.Now().After(expiresAt) {
				continue
			}
		}

		// Check if this is a delegation attestation
		if !hasCapability(att.Record.Capabilities, CapabilityDelegate) {
			continue
		}

		// Check scope for delegation
		if !scopeMatches(att.Record.Scope, scope) {
			continue
		}

		// Check conditions
		if !conditionsMatch(att.Record.Conditions, parsedAddr, resolution) {
			continue
		}

		// Follow the chain - check if issuer has the capability
		issuerResult, err := verifyTrustWithDepth(resolver, store, att.Record.Issuer, capability, scope, depth+1, seen)
		if err != nil {
			continue
		}
		if issuerResult.Verified {
			// Found trust through delegation
			chain := append([]*Attestation{att}, issuerResult.Attestations...)
			return &VerifyResult{
				Verified:     true,
				Attestations: chain,
			}, nil
		}
	}

	return &VerifyResult{
		Verified: false,
		Reason:   fmt.Sprintf("no attestation grants %q for scope %q", capability, scope),
	}, nil
}

// Revoke creates a revocation record for an attestation.
func Revoke(registry Registry, issuer string, passphrase []byte, attestationID string, reason string) (*Revocation, error) {
	// Get issuer identity
	issuerIdentity, err := registry.Get(issuer)
	if err != nil {
		return nil, fmt.Errorf("get issuer identity: %w", err)
	}
	if !issuerIdentity.HasKey {
		return nil, fmt.Errorf("issuer %q has no signing key", issuer)
	}

	now := time.Now().UTC()
	record := RevocationRecord{
		Version:       Version,
		Type:          "revocation",
		ID:            NewRevocationID(),
		AttestationID: attestationID,
		Issuer:        "@" + issuer,
		IssuerGUID:    issuerIdentity.Record.GUID,
		IssuerKeyID:   KeyFingerprint(issuerIdentity.PublicKey),
		Reason:        reason,
		RevokedAt:     now.Format(time.RFC3339),
	}

	// Canonicalize for signing
	canonical, err := CanonicalizeForSigning(record)
	if err != nil {
		return nil, fmt.Errorf("canonicalize: %w", err)
	}

	// Sign
	sig, err := registry.Sign(issuer, passphrase, canonical)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	record.Signature = &SignatureRecord{
		Algorithm: "ed25519",
		Value:     base64.StdEncoding.EncodeToString(sig),
	}

	return &Revocation{
		Record:    record,
		Signature: sig,
	}, nil
}

// hasCapability checks if a capability list contains a specific capability.
func hasCapability(caps []string, cap string) bool {
	for _, c := range caps {
		if c == cap {
			return true
		}
	}
	return false
}

// scopeMatches checks if a scope pattern matches a target scope.
// Supports glob-like patterns with * wildcard.
func scopeMatches(pattern, target string) bool {
	// Universal scope matches everything
	if pattern == "*" {
		return true
	}

	// Exact match
	if pattern == target {
		return true
	}

	// Handle trailing wildcard (e.g., "github.com/adamavenir/*")
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return target == prefix || strings.HasPrefix(target, prefix+"/")
	}

	// Handle leading wildcard (e.g., "*.corp.com")
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(target, suffix)
	}

	// Use filepath.Match for other patterns
	match, _ := filepath.Match(pattern, target)
	return match
}

// conditionsMatch checks if attestation conditions are satisfied.
func conditionsMatch(cond *Conditions, addr Address, res *Resolution) bool {
	if cond == nil {
		return true
	}

	// Check required variants
	if len(cond.RequireVariants) > 0 {
		for _, rv := range cond.RequireVariants {
			if !addr.HasVariant(rv) {
				return false
			}
		}
	}

	// Check execution host conditions (only if invoke config exists)
	if res.Invoke != nil {
		execHost := res.Invoke.Config["execution_host"]
		execHostStr, _ := execHost.(string)

		// Check excluded hosts
		for _, eh := range cond.ExcludeExecutionHosts {
			if execHostStr == eh {
				return false
			}
		}

		// Check required hosts
		if len(cond.RequireExecutionHosts) > 0 {
			found := false
			for _, rh := range cond.RequireExecutionHosts {
				if execHostStr == rh {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}
