package aap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterAndGet(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	// Register with key
	id, err := reg.Register("dev", RegisterOpts{
		GenerateKey: true,
		Passphrase:  []byte("test-passphrase"),
		Metadata:    map[string]string{"display_name": "Dev Agent"},
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify identity fields
	if !id.HasKey {
		t.Error("HasKey = false, want true")
	}
	if id.Record.Agent != "dev" {
		t.Errorf("Agent = %q, want %q", id.Record.Agent, "dev")
	}
	if id.Record.Address != "@dev" {
		t.Errorf("Address = %q, want %q", id.Record.Address, "@dev")
	}
	if !strings.HasPrefix(id.Record.GUID, "aap-") {
		t.Errorf("GUID = %q, want prefix %q", id.Record.GUID, "aap-")
	}
	if id.Record.PublicKey == nil {
		t.Error("PublicKey = nil, want non-nil")
	} else if id.Record.PublicKey.Algorithm != "ed25519" {
		t.Errorf("Algorithm = %q, want %q", id.Record.PublicKey.Algorithm, "ed25519")
	}
	if id.Record.Metadata["display_name"] != "Dev Agent" {
		t.Errorf("Metadata[display_name] = %q, want %q", id.Record.Metadata["display_name"], "Dev Agent")
	}

	// Get back
	id2, err := reg.Get("dev")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if id.Record.GUID != id2.Record.GUID {
		t.Errorf("GUID mismatch: %q != %q", id.Record.GUID, id2.Record.GUID)
	}
}

func TestRegisterKeyless(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	id, err := reg.Register("legacy", RegisterOpts{GenerateKey: false})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if id.HasKey {
		t.Error("HasKey = true, want false")
	}
	if id.Record.PublicKey != nil {
		t.Error("PublicKey = non-nil, want nil")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{GenerateKey: false})
	if err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{GenerateKey: false})
	if err == nil {
		t.Error("Second Register should fail for duplicate agent")
	}
}

func TestRegisterRequiresPassphrase(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{
		GenerateKey: true,
		// No passphrase
	})
	if err == nil {
		t.Error("Register without passphrase should fail when GenerateKey=true")
	}
}

func TestSignAndVerify(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	passphrase := []byte("test-passphrase")
	_, err = reg.Register("signer", RegisterOpts{GenerateKey: true, Passphrase: passphrase})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	data := []byte("hello world")
	sig, err := reg.Sign("signer", passphrase, data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	valid, err := reg.Verify("signer", data, sig)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !valid {
		t.Error("Verify returned false, want true")
	}

	// Tampered data fails
	valid, err = reg.Verify("signer", []byte("tampered"), sig)
	if err != nil {
		t.Fatalf("Verify tampered failed: %v", err)
	}
	if valid {
		t.Error("Verify tampered returned true, want false")
	}
}

func TestSignWrongPassphrase(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Register("agent", RegisterOpts{GenerateKey: true, Passphrase: []byte("correct")})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	_, err = reg.Sign("agent", []byte("wrong"), []byte("data"))
	if err == nil {
		t.Error("Sign with wrong passphrase should fail")
	}
}

func TestSignKeylessAgent(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Register("legacy", RegisterOpts{GenerateKey: false})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	_, err = reg.Sign("legacy", []byte("any"), []byte("data"))
	if err == nil {
		t.Error("Sign on keyless agent should fail")
	}
}

func TestList(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	// Empty initially
	list, err := reg.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List = %d entries, want 0", len(list))
	}

	// Register some agents
	reg.Register("alice", RegisterOpts{GenerateKey: false})
	reg.Register("bob", RegisterOpts{GenerateKey: false})
	reg.Register("charlie", RegisterOpts{GenerateKey: true, Passphrase: []byte("pass")})

	list, err = reg.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List = %d entries, want 3", len(list))
	}
}

func TestFileStorage(t *testing.T) {
	tempDir := t.TempDir()
	reg, err := NewFileRegistry(tempDir)
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{
		GenerateKey: true,
		Passphrase:  []byte("pass"),
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Check identity.json exists
	identityPath := filepath.Join(tempDir, "dev", "identity.json")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		t.Error("identity.json not created")
	}

	// Check private.key exists
	keyPath := filepath.Join(tempDir, "dev", "private.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("private.key not created")
	}

	// Check private.key has restrictive permissions
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("Stat private.key failed: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("private.key mode = %o, want 0600", mode)
	}
}

func TestDelete(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{GenerateKey: false})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify exists
	if !reg.Exists("dev") {
		t.Error("Exists = false after register")
	}

	// Delete
	if err := reg.Delete("dev"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	if reg.Exists("dev") {
		t.Error("Exists = true after delete")
	}
}

func TestGetNotFound(t *testing.T) {
	reg, err := NewFileRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRegistry failed: %v", err)
	}

	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("Get nonexistent agent should fail")
	}
}
