package aap

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if len(pub) != 32 {
		t.Errorf("Public key length = %d, want 32", len(pub))
	}
	if len(priv) != 64 {
		t.Errorf("Private key length = %d, want 64", len(priv))
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	data := []byte("hello world")
	sig := Sign(priv, data)

	if len(sig) != 64 {
		t.Errorf("Signature length = %d, want 64", len(sig))
	}

	if !Verify(pub, data, sig) {
		t.Error("Verify returned false, want true")
	}

	// Tampered data
	if Verify(pub, []byte("tampered"), sig) {
		t.Error("Verify tampered returned true, want false")
	}

	// Tampered signature
	badSig := make([]byte, 64)
	copy(badSig, sig)
	badSig[0] ^= 0xff
	if Verify(pub, data, badSig) {
		t.Error("Verify bad signature returned true, want false")
	}
}

func TestKeyFingerprint(t *testing.T) {
	pub, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	fp := KeyFingerprint(pub)

	if !strings.HasPrefix(fp, "sha256:") {
		t.Errorf("Fingerprint = %q, want sha256: prefix", fp)
	}

	// Should be deterministic
	fp2 := KeyFingerprint(pub)
	if fp != fp2 {
		t.Errorf("Fingerprint not deterministic: %q != %q", fp, fp2)
	}
}

func TestEncryptDecryptPrivateKey(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	passphrase := []byte("test-passphrase")

	ekf, err := EncryptPrivateKey(priv, passphrase)
	if err != nil {
		t.Fatalf("EncryptPrivateKey failed: %v", err)
	}

	// Verify format
	if ekf.Version != 1 {
		t.Errorf("Version = %d, want 1", ekf.Version)
	}
	if ekf.Algorithm != "xchacha20-poly1305" {
		t.Errorf("Algorithm = %q, want xchacha20-poly1305", ekf.Algorithm)
	}
	if ekf.KDF != "argon2id" {
		t.Errorf("KDF = %q, want argon2id", ekf.KDF)
	}

	// Decrypt
	decrypted, err := DecryptPrivateKey(ekf, passphrase)
	if err != nil {
		t.Fatalf("DecryptPrivateKey failed: %v", err)
	}

	if !bytes.Equal(priv, decrypted) {
		t.Error("Decrypted key doesn't match original")
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	_, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	ekf, err := EncryptPrivateKey(priv, []byte("correct"))
	if err != nil {
		t.Fatalf("EncryptPrivateKey failed: %v", err)
	}

	_, err = DecryptPrivateKey(ekf, []byte("wrong"))
	if err == nil {
		t.Error("DecryptPrivateKey with wrong passphrase should fail")
	}
}

func TestCanonicalizeForSigning(t *testing.T) {
	// Object with signature field should have it stripped
	input := map[string]any{
		"type":    "attestation",
		"subject": "@dev",
		"signature": map[string]any{
			"algorithm": "ed25519",
			"value":     "xxx",
		},
	}

	canonical, err := CanonicalizeForSigning(input)
	if err != nil {
		t.Fatalf("CanonicalizeForSigning failed: %v", err)
	}

	// Should not contain signature
	if strings.Contains(string(canonical), "signature") {
		t.Error("Canonical output contains 'signature', should be stripped")
	}

	// Should be deterministic
	canonical2, err := CanonicalizeForSigning(input)
	if err != nil {
		t.Fatalf("Second CanonicalizeForSigning failed: %v", err)
	}
	if !bytes.Equal(canonical, canonical2) {
		t.Error("Canonicalization not deterministic")
	}
}

func TestCanonicalize(t *testing.T) {
	// Test that keys are sorted
	input := map[string]any{
		"z": 1,
		"a": 2,
		"m": 3,
	}

	canonical, err := Canonicalize(input)
	if err != nil {
		t.Fatalf("Canonicalize failed: %v", err)
	}

	expected := `{"a":2,"m":3,"z":1}`
	if string(canonical) != expected {
		t.Errorf("Canonical = %q, want %q", string(canonical), expected)
	}
}

func TestNewGUID(t *testing.T) {
	guid := NewGUID()

	if !strings.HasPrefix(guid, "aap-") {
		t.Errorf("GUID = %q, want aap- prefix", guid)
	}

	if len(guid) != 20 { // "aap-" (4) + 16 chars
		t.Errorf("GUID length = %d, want 20", len(guid))
	}

	// Check uniqueness
	guid2 := NewGUID()
	if guid == guid2 {
		t.Error("Two GUIDs are equal, should be unique")
	}
}

func TestIsValidGUID(t *testing.T) {
	tests := []struct {
		guid  string
		valid bool
	}{
		{"aap-1234567890abcdef", true},
		{"aap-abcdefghijklmnop", true},
		{"aap-0000000000000000", true},
		{"aap-", false},                     // Too short
		{"aap-123", false},                  // Too short
		{"aap-1234567890abcdefg", false},    // Too long
		{"usr-1234567890abcdef", false},     // Wrong prefix
		{"AAP-1234567890abcdef", false},     // Uppercase prefix
		{"aap-1234567890ABCDEF", false},     // Uppercase chars
		{"aap-123456789!abcdef", false},     // Invalid char
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.guid, func(t *testing.T) {
			got := IsValidGUID(tt.guid)
			if got != tt.valid {
				t.Errorf("IsValidGUID(%q) = %v, want %v", tt.guid, got, tt.valid)
			}
		})
	}
}
