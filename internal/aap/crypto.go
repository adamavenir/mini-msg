package aap

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// GenerateKeyPair creates a new Ed25519 keypair.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// Sign signs data with an Ed25519 private key.
func Sign(key ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(key, data)
}

// Verify checks an Ed25519 signature.
func Verify(key ed25519.PublicKey, data, sig []byte) bool {
	return ed25519.Verify(key, data, sig)
}

// KeyFingerprint generates sha256:{hex} fingerprint for a public key.
func KeyFingerprint(key ed25519.PublicKey) string {
	hash := sha256.Sum256(key)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// CanonicalizeForSigning removes the signature field and applies RFC 8785 JCS.
// Per AAP-SPEC.md section 6.5, signature must be stripped before canonicalization.
func CanonicalizeForSigning(v any) ([]byte, error) {
	// Convert to map to remove signature field
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal to map: %w", err)
	}

	// Remove signature field
	delete(m, "signature")

	// Apply JCS canonicalization
	return Canonicalize(m)
}

// Canonicalize applies RFC 8785 JSON Canonicalization Scheme.
// For now, we use Go's standard json.Marshal which produces deterministic output
// for maps (sorted keys). For full JCS compliance, a dedicated library should be used.
func Canonicalize(v any) ([]byte, error) {
	// Go's json.Marshal sorts map keys alphabetically, which matches JCS for simple cases.
	// For production, use github.com/cyberphone/json-canonicalization
	return json.Marshal(v)
}

// EncryptedKeyFile is the format for storing encrypted private keys.
type EncryptedKeyFile struct {
	Version    int       `json:"version"`
	Algorithm  string    `json:"algorithm"`
	KDF        string    `json:"kdf"`
	KDFParams  KDFParams `json:"kdf_params"`
	Nonce      string    `json:"nonce"`
	Ciphertext string    `json:"ciphertext"`
}

// KDFParams holds Argon2id parameters.
type KDFParams struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	Salt    string `json:"salt"` // base64-encoded
}

// DefaultKDFParams returns secure default Argon2id parameters.
func DefaultKDFParams() KDFParams {
	return KDFParams{
		Time:    3,
		Memory:  65536, // 64MB
		Threads: 4,
	}
}

// EncryptPrivateKey encrypts a private key using XChaCha20-Poly1305.
func EncryptPrivateKey(privateKey ed25519.PrivateKey, passphrase []byte) (*EncryptedKeyFile, error) {
	// Generate random salt
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	params := DefaultKDFParams()
	params.Salt = base64.StdEncoding.EncodeToString(salt)

	// Derive key using Argon2id
	derivedKey := argon2.IDKey(passphrase, salt, params.Time, params.Memory, params.Threads, 32)

	// Create cipher
	aead, err := chacha20poly1305.NewX(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	// Generate random nonce (24 bytes for XChaCha20)
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Encrypt the private key
	ciphertext := aead.Seal(nil, nonce, []byte(privateKey), nil)

	return &EncryptedKeyFile{
		Version:    1,
		Algorithm:  "xchacha20-poly1305",
		KDF:        "argon2id",
		KDFParams:  params,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

// DecryptPrivateKey decrypts a private key using the passphrase.
func DecryptPrivateKey(ekf *EncryptedKeyFile, passphrase []byte) (ed25519.PrivateKey, error) {
	if ekf.Version != 1 {
		return nil, fmt.Errorf("unsupported key file version: %d", ekf.Version)
	}
	if ekf.Algorithm != "xchacha20-poly1305" {
		return nil, fmt.Errorf("unsupported algorithm: %s", ekf.Algorithm)
	}
	if ekf.KDF != "argon2id" {
		return nil, fmt.Errorf("unsupported KDF: %s", ekf.KDF)
	}

	// Decode salt
	salt, err := base64.StdEncoding.DecodeString(ekf.KDFParams.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	// Derive key using Argon2id
	derivedKey := argon2.IDKey(
		passphrase,
		salt,
		ekf.KDFParams.Time,
		ekf.KDFParams.Memory,
		ekf.KDFParams.Threads,
		32,
	)

	// Create cipher
	aead, err := chacha20poly1305.NewX(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	// Decode nonce
	nonce, err := base64.StdEncoding.DecodeString(ekf.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}

	// Decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(ekf.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	// Decrypt
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w (wrong passphrase?)", err)
	}

	return ed25519.PrivateKey(plaintext), nil
}
