package aap

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAttestAndVerify(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, err := NewFileRegistry(agentsDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	// Register issuer and subject
	_, err = reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam-pass")})
	if err != nil {
		t.Fatalf("register adam: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev-pass")})
	if err != nil {
		t.Fatalf("register dev: %v", err)
	}

	// Issue attestation
	att, err := Attest(reg, "adam", []byte("adam-pass"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "github.com/adamavenir/*",
	})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}

	if att.Record.Subject != "@dev" {
		t.Errorf("expected subject '@dev', got %q", att.Record.Subject)
	}
	if att.Record.Issuer != "@adam" {
		t.Errorf("expected issuer '@adam', got %q", att.Record.Issuer)
	}
	if len(att.Record.Capabilities) != 1 || att.Record.Capabilities[0] != "write" {
		t.Errorf("expected capabilities ['write'], got %v", att.Record.Capabilities)
	}

	// Verify attestation
	err = VerifyAttestation(reg, att)
	if err != nil {
		t.Fatalf("verify attestation: %v", err)
	}
	if !att.Verified {
		t.Error("expected attestation to be verified")
	}
}

func TestAttestWithExpiration(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	att, err := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
		ExpiresIn:    24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}

	if att.Record.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}

	expiresAt, err := time.Parse(time.RFC3339, *att.Record.ExpiresAt)
	if err != nil {
		t.Fatalf("parse ExpiresAt: %v", err)
	}

	// Should expire approximately 24 hours from now
	expectedExpiry := time.Now().Add(24 * time.Hour)
	diff := expiresAt.Sub(expectedExpiry)
	if diff < -1*time.Minute || diff > 1*time.Minute {
		t.Errorf("expiration time off by %v", diff)
	}
}

func TestAttestWithConditions(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	att, err := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"deploy"},
		Scope:        "*",
		Conditions: &Conditions{
			RequireVariants: []string{"trusted"},
		},
	})
	if err != nil {
		t.Fatalf("attest: %v", err)
	}

	if att.Record.Conditions == nil {
		t.Fatal("expected conditions to be set")
	}
	if len(att.Record.Conditions.RequireVariants) != 1 || att.Record.Conditions.RequireVariants[0] != "trusted" {
		t.Errorf("expected RequireVariants ['trusted'], got %v", att.Record.Conditions.RequireVariants)
	}
}

func TestVerifyTrust(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	// Create and save attestation
	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "github.com/adamavenir/*",
	})

	store := NewMemoryAttestationStore()
	store.SaveAttestation(att)

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	// Should pass - has write on matching scope
	result, err := VerifyTrust(resolver, store, "@dev", "write", "github.com/adamavenir/fray")
	if err != nil {
		t.Fatalf("verify trust: %v", err)
	}
	if !result.Verified {
		t.Errorf("expected verified, got reason: %s", result.Reason)
	}

	// Should fail - doesn't have read capability
	result, _ = VerifyTrust(resolver, store, "@dev", "read", "github.com/adamavenir/fray")
	if result.Verified {
		t.Error("expected not verified for read capability")
	}

	// Should fail - scope doesn't match
	result, _ = VerifyTrust(resolver, store, "@dev", "write", "github.com/other/repo")
	if result.Verified {
		t.Error("expected not verified for non-matching scope")
	}
}

func TestTrustChain(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	// adam -> pm (delegate) -> dev (write)
	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("pm", RegisterOpts{GenerateKey: true, Passphrase: []byte("pm")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	store := NewMemoryAttestationStore()

	// adam grants pm delegate capability (and write to pass through)
	att1, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@pm",
		Capabilities: []string{CapabilityDelegate, "write"},
		Scope:        "*",
	})
	store.SaveAttestation(att1)

	// pm grants dev write capability
	att2, _ := Attest(reg, "pm", []byte("pm"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{CapabilityDelegate, "write"},
		Scope:        "github.com/adamavenir/fray",
	})
	store.SaveAttestation(att2)

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	result, err := VerifyTrust(resolver, store, "@dev", "write", "github.com/adamavenir/fray")
	if err != nil {
		t.Fatalf("verify trust: %v", err)
	}
	if !result.Verified {
		t.Errorf("expected verified via delegation chain, got reason: %s", result.Reason)
	}
	if len(result.Attestations) < 1 {
		t.Error("expected attestation chain in result")
	}
}

func TestExpiredAttestation(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	// Issue attestation that's already expired
	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
		ExpiresIn:    -1 * time.Hour, // Already expired
	})

	store := NewMemoryAttestationStore()
	store.SaveAttestation(att)

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	result, _ := VerifyTrust(resolver, store, "@dev", "write", "*")
	if result.Verified {
		t.Error("expected not verified for expired attestation")
	}
}

func TestConditions(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	// Grant deploy only for .trusted variant
	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"deploy"},
		Scope:        "*",
		Conditions: &Conditions{
			RequireVariants: []string{"trusted"},
		},
	})

	store := NewMemoryAttestationStore()
	store.SaveAttestation(att)

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	// @dev without variant - should fail
	result, _ := VerifyTrust(resolver, store, "@dev", "deploy", "*")
	if result.Verified {
		t.Error("expected not verified without required variant")
	}

	// @dev.trusted - should pass
	result, _ = VerifyTrust(resolver, store, "@dev.trusted", "deploy", "*")
	if !result.Verified {
		t.Errorf("expected verified with trusted variant, got reason: %s", result.Reason)
	}
}

func TestRevocation(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
	})

	store := NewMemoryAttestationStore()
	store.SaveAttestation(att)

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	// Verify works before revocation
	result, _ := VerifyTrust(resolver, store, "@dev", "write", "*")
	if !result.Verified {
		t.Fatal("expected verified before revocation")
	}

	// Revoke
	rev, err := Revoke(reg, "adam", []byte("adam"), att.Record.ID, "Role changed")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	store.SaveRevocation(rev)

	// Verify fails after revocation
	result, _ = VerifyTrust(resolver, store, "@dev", "write", "*")
	if result.Verified {
		t.Error("expected not verified after revocation")
	}
}

func TestGUIDBindingValidation(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	// Create a valid attestation first
	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
	})

	// Tamper with the subject GUID
	att.Record.SubjectGUID = "aap-wrong-guid"

	err := VerifyAttestation(reg, att)
	if err == nil {
		t.Error("expected error for GUID mismatch")
	}
}

func TestKeyIDBindingValidation(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
	})

	// Tamper with the issuer key ID
	att.Record.IssuerKeyID = "sha256:wrong"

	err := VerifyAttestation(reg, att)
	if err == nil {
		t.Error("expected error for key ID mismatch")
	}
}

func TestUntrustedIdentityFailsVerification(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	// Register adam with key
	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	// Register dev without key (keyless/untrusted)
	reg.Register("dev", RegisterOpts{})

	// Can't attest to keyless agent? Let's see
	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
	})

	store := NewMemoryAttestationStore()
	store.SaveAttestation(att)

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	// Untrusted identities should fail verification
	result, _ := VerifyTrust(resolver, store, "@dev", "write", "*")
	if result.Verified {
		t.Error("expected not verified for untrusted identity")
	}
	if result.Reason == "" {
		t.Error("expected reason for failure")
	}
}

func TestScopeMatching(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
		want    bool
	}{
		{"*", "anything", true},
		{"*", "github.com/user/repo", true},
		{"github.com/adamavenir/*", "github.com/adamavenir/fray", true},
		{"github.com/adamavenir/*", "github.com/adamavenir", true},
		{"github.com/adamavenir/*", "github.com/other/fray", false},
		{"github.com/adamavenir/fray", "github.com/adamavenir/fray", true},
		{"github.com/adamavenir/fray", "github.com/adamavenir/other", false},
		{"*.corp.com", "api.corp.com", true},
		{"*.corp.com", "deep.api.corp.com", true},
		{"*.corp.com", "other.com", false},
		{"/Users/adam/dev/*", "/Users/adam/dev/fray", true},
		{"/Users/adam/dev/*", "/Users/other/dev/fray", false},
	}

	for _, tt := range tests {
		got := scopeMatches(tt.pattern, tt.target)
		if got != tt.want {
			t.Errorf("scopeMatches(%q, %q) = %v, want %v", tt.pattern, tt.target, got, tt.want)
		}
	}
}

func TestHasCapability(t *testing.T) {
	caps := []string{"read", "write", "deploy"}

	if !hasCapability(caps, "read") {
		t.Error("expected to have read")
	}
	if !hasCapability(caps, "write") {
		t.Error("expected to have write")
	}
	if !hasCapability(caps, "deploy") {
		t.Error("expected to have deploy")
	}
	if hasCapability(caps, "admin") {
		t.Error("expected not to have admin")
	}
	if hasCapability(nil, "read") {
		t.Error("expected nil to not have any capability")
	}
}

func TestAttestationStore(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})
	reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("dev")})

	att, _ := Attest(reg, "adam", []byte("adam"), TrustClaim{
		Subject:      "@dev",
		Capabilities: []string{"write"},
		Scope:        "*",
	})

	// Test file-based store
	store, err := NewFileAttestationStore(dir)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	if err := store.SaveAttestation(att); err != nil {
		t.Fatalf("save attestation: %v", err)
	}

	loaded, err := store.LoadAttestations("dev")
	if err != nil {
		t.Fatalf("load attestations: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("expected 1 attestation, got %d", len(loaded))
	}

	if loaded[0].Record.ID != att.Record.ID {
		t.Errorf("expected ID %s, got %s", att.Record.ID, loaded[0].Record.ID)
	}
}

func TestRevocationStore(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	reg.Register("adam", RegisterOpts{GenerateKey: true, Passphrase: []byte("adam")})

	rev, _ := Revoke(reg, "adam", []byte("adam"), "att-test123", "Testing")

	store, _ := NewFileAttestationStore(dir)

	if err := store.SaveRevocation(rev); err != nil {
		t.Fatalf("save revocation: %v", err)
	}

	loaded, err := store.LoadRevocations()
	if err != nil {
		t.Fatalf("load revocations: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("expected 1 revocation, got %d", len(loaded))
	}

	if loaded[0].Record.AttestationID != "att-test123" {
		t.Errorf("expected attestation ID 'att-test123', got %s", loaded[0].Record.AttestationID)
	}
}

func TestMaxTrustChainDepth(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	reg, _ := NewFileRegistry(agentsDir)

	store := NewMemoryAttestationStore()

	// Create a chain deeper than MaxTrustChainDepth (5)
	// We need 7 agents to create a 6-hop chain that exceeds depth 5
	agents := []string{"root", "a1", "a2", "a3", "a4", "a5", "target"}
	for _, agent := range agents {
		reg.Register(agent, RegisterOpts{GenerateKey: true, Passphrase: []byte(agent)})
	}

	// root has write capability directly
	rootAtt, _ := Attest(reg, "root", []byte("root"), TrustClaim{
		Subject:      "@root", // self-attestation to establish root trust
		Capabilities: []string{"write"},
		Scope:        "*",
	})
	store.SaveAttestation(rootAtt)

	// Create delegation chain: each agent delegates to the next
	// target needs to go through 6 levels of delegation to reach root
	for i := 0; i < len(agents)-1; i++ {
		att, _ := Attest(reg, agents[i], []byte(agents[i]), TrustClaim{
			Subject:      "@" + agents[i+1],
			Capabilities: []string{CapabilityDelegate}, // only delegate, not write
			Scope:        "*",
		})
		store.SaveAttestation(att)
	}

	resolver, _ := NewResolver(ResolverOpts{GlobalRegistry: dir})

	// target needs to traverse: target->a5->a4->a3->a2->a1->root (6 hops)
	// This exceeds MaxTrustChainDepth of 5
	result, _ := VerifyTrust(resolver, store, "@target", "write", "*")
	if result.Verified {
		t.Error("expected not verified due to max depth")
	}
	if result.Reason != "max trust chain depth exceeded" {
		t.Logf("got reason: %s", result.Reason)
	}
}

func TestNewAttestationID(t *testing.T) {
	id := NewAttestationID()
	if !hasPrefix(id, "att-") {
		t.Errorf("expected att- prefix, got %s", id)
	}
}

func TestNewRevocationID(t *testing.T) {
	id := NewRevocationID()
	if !hasPrefix(id, "rev-") {
		t.Errorf("expected rev- prefix, got %s", id)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
