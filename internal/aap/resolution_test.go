package aap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLocal(t *testing.T) {
	globalDir := t.TempDir()
	agentsDir := filepath.Join(globalDir, "agents")
	reg, err := NewFileRegistry(agentsDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("test")})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		GlobalRegistry: globalDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@dev")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Identity.Record.Agent != "dev" {
		t.Errorf("expected agent 'dev', got %q", res.Identity.Record.Agent)
	}
	if res.TrustLevel != TrustLevelFull {
		t.Errorf("expected TrustLevelFull, got %q", res.TrustLevel)
	}
	if res.Source != "aap" {
		t.Errorf("expected source 'aap', got %q", res.Source)
	}
}

func TestResolveKeyless(t *testing.T) {
	globalDir := t.TempDir()
	agentsDir := filepath.Join(globalDir, "agents")
	reg, err := NewFileRegistry(agentsDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	// Register without generating key
	_, err = reg.Register("keyless", RegisterOpts{
		Metadata: map[string]string{"description": "no keys"},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		GlobalRegistry: globalDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@keyless")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.TrustLevel != TrustLevelUntrusted {
		t.Errorf("expected TrustLevelUntrusted for keyless agent, got %q", res.TrustLevel)
	}
}

func TestProjectOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	globalAgentsDir := filepath.Join(globalDir, "agents")
	globalReg, err := NewFileRegistry(globalAgentsDir)
	if err != nil {
		t.Fatalf("create global registry: %v", err)
	}

	projectAgentsDir := filepath.Join(projectDir, "agents")
	projectReg, err := NewFileRegistry(projectAgentsDir)
	if err != nil {
		t.Fatalf("create project registry: %v", err)
	}

	// Register same agent in both
	_, err = globalReg.Register("dev", RegisterOpts{
		Metadata: map[string]string{"source": "global"},
	})
	if err != nil {
		t.Fatalf("register global: %v", err)
	}

	_, err = projectReg.Register("dev", RegisterOpts{
		Metadata: map[string]string{"source": "project"},
	})
	if err != nil {
		t.Fatalf("register project: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		GlobalRegistry:  globalDir,
		ProjectRegistry: projectDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@dev")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Per AAP-SPEC, global is checked first
	if res.Identity.Record.Metadata["source"] != "global" {
		t.Errorf("expected global to take precedence, got source=%q", res.Identity.Record.Metadata["source"])
	}
}

func TestProjectFallback(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Only create agents dir in global, not register anyone
	os.MkdirAll(filepath.Join(globalDir, "agents"), 0755)

	projectAgentsDir := filepath.Join(projectDir, "agents")
	projectReg, err := NewFileRegistry(projectAgentsDir)
	if err != nil {
		t.Fatalf("create project registry: %v", err)
	}

	_, err = projectReg.Register("project-only", RegisterOpts{
		Metadata: map[string]string{"source": "project"},
	})
	if err != nil {
		t.Fatalf("register project: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		GlobalRegistry:  globalDir,
		ProjectRegistry: projectDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@project-only")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Identity.Record.Metadata["source"] != "project" {
		t.Errorf("expected project fallback, got source=%q", res.Identity.Record.Metadata["source"])
	}
}

func TestFrayCompat(t *testing.T) {
	frayDir := t.TempDir()
	jsonl := `{"type":"agent","guid":"usr-abc123","agent_id":"legacy","registered_at":1234567890,"status":"available"}`
	if err := os.WriteFile(filepath.Join(frayDir, "agents.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write agents.jsonl: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		FrayCompat: true,
		FrayPath:   frayDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@legacy")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Identity.Record.Agent != "legacy" {
		t.Errorf("expected agent 'legacy', got %q", res.Identity.Record.Agent)
	}
	if res.TrustLevel != TrustLevelUntrusted {
		t.Errorf("expected TrustLevelUntrusted for legacy agent, got %q", res.TrustLevel)
	}
	if res.Source != "fray" {
		t.Errorf("expected source 'fray', got %q", res.Source)
	}
}

func TestFrayCompatWithInvoke(t *testing.T) {
	frayDir := t.TempDir()
	jsonl := `{"type":"agent","guid":"usr-abc123","agent_id":"managed","registered_at":1234567890,"managed":true,"invoke":{"driver":"claude","model":"sonnet","trust":["wake"]}}`
	if err := os.WriteFile(filepath.Join(frayDir, "agents.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write agents.jsonl: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		FrayCompat: true,
		FrayPath:   frayDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@managed")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Invoke == nil {
		t.Fatal("expected invoke config")
	}
	if res.Invoke.Driver != "claude" {
		t.Errorf("expected driver 'claude', got %q", res.Invoke.Driver)
	}
	if res.Invoke.Model != "sonnet" {
		t.Errorf("expected model 'sonnet', got %q", res.Invoke.Model)
	}
	if len(res.Invoke.Trust) != 1 || res.Invoke.Trust[0] != "wake" {
		t.Errorf("expected trust ['wake'], got %v", res.Invoke.Trust)
	}
}

func TestFrayCompatAgentUpdate(t *testing.T) {
	frayDir := t.TempDir()
	jsonl := `{"type":"agent","guid":"usr-abc123","agent_id":"updated","registered_at":1234567890,"status":"available"}
{"type":"agent_update","agent_id":"updated","status":"away","last_seen":1234567999}`
	if err := os.WriteFile(filepath.Join(frayDir, "agents.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write agents.jsonl: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{
		FrayCompat: true,
		FrayPath:   frayDir,
	})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@updated")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Status should be updated
	if res.Identity.Record.Metadata["status"] != "away" {
		t.Errorf("expected status 'away', got %q", res.Identity.Record.Metadata["status"])
	}
}

func TestResolveVariant(t *testing.T) {
	globalDir := t.TempDir()
	agentsDir := filepath.Join(globalDir, "agents")
	reg, err := NewFileRegistry(agentsDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	_, err = reg.Register("devrel", RegisterOpts{
		GenerateKey: true,
		Passphrase:  []byte("test"),
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Create variant context
	variantDir := filepath.Join(globalDir, "variants", "devrel", "mlld")
	if err := os.MkdirAll(variantDir, 0755); err != nil {
		t.Fatalf("create variant dir: %v", err)
	}

	variantContext := VariantContext{
		Variant:    "mlld",
		ContextURI: "git://github.com/mlld-lang/mlld/.aap/variants/devrel/",
	}
	data, _ := json.Marshal(variantContext)
	if err := os.WriteFile(filepath.Join(variantDir, "context.json"), data, 0644); err != nil {
		t.Fatalf("write context.json: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{GlobalRegistry: globalDir})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@devrel.mlld")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Variants == nil || res.Variants["mlld"] == nil {
		t.Fatal("expected variant 'mlld' to be loaded")
	}
	if res.Variants["mlld"].ContextURI != "git://github.com/mlld-lang/mlld/.aap/variants/devrel/" {
		t.Errorf("unexpected context URI: %q", res.Variants["mlld"].ContextURI)
	}
}

func TestResolveWithInvokeConfig(t *testing.T) {
	globalDir := t.TempDir()
	agentsDir := filepath.Join(globalDir, "agents")
	reg, err := NewFileRegistry(agentsDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	_, err = reg.Register("configured", RegisterOpts{
		GenerateKey: true,
		Passphrase:  []byte("test"),
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Write invoke.json
	invokeConfig := InvokeConfig{
		Driver: "claude",
		Model:  "opus",
		Trust:  []string{"wake", "read"},
	}
	data, _ := json.Marshal(invokeConfig)
	if err := os.WriteFile(filepath.Join(agentsDir, "configured", "invoke.json"), data, 0644); err != nil {
		t.Fatalf("write invoke.json: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{GlobalRegistry: globalDir})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	res, err := resolver.Resolve("@configured")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if res.Invoke == nil {
		t.Fatal("expected invoke config")
	}
	if res.Invoke.Driver != "claude" {
		t.Errorf("expected driver 'claude', got %q", res.Invoke.Driver)
	}
	if res.Invoke.Model != "opus" {
		t.Errorf("expected model 'opus', got %q", res.Invoke.Model)
	}
}

func TestResolveWithHost(t *testing.T) {
	globalDir := t.TempDir()
	agentsDir := filepath.Join(globalDir, "agents")
	reg, err := NewFileRegistry(agentsDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	_, err = reg.Register("dev", RegisterOpts{GenerateKey: true, Passphrase: []byte("test")})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Write hosts.json
	hosts := []string{"@workstation", "@server"}
	data, _ := json.Marshal(hosts)
	if err := os.WriteFile(filepath.Join(agentsDir, "dev", "hosts.json"), data, 0644); err != nil {
		t.Fatalf("write hosts.json: %v", err)
	}

	resolver, err := NewResolver(ResolverOpts{GlobalRegistry: globalDir})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	// Should work for listed host
	res, err := resolver.Resolve("@dev@workstation")
	if err != nil {
		t.Fatalf("resolve with host: %v", err)
	}
	if res.Identity.Record.Agent != "dev" {
		t.Errorf("expected agent 'dev', got %q", res.Identity.Record.Agent)
	}

	// Should fail for unlisted host (remote resolution not implemented)
	_, err = resolver.Resolve("@dev@unknown-host")
	if err == nil {
		t.Error("expected error for unknown host")
	}
}

func TestResolveNotFound(t *testing.T) {
	globalDir := t.TempDir()
	os.MkdirAll(filepath.Join(globalDir, "agents"), 0755)

	resolver, err := NewResolver(ResolverOpts{GlobalRegistry: globalDir})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	_, err = resolver.Resolve("@nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestResolveInvalidAddress(t *testing.T) {
	resolver, err := NewResolver(ResolverOpts{})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	cases := []string{
		"",
		"dev",     // missing @
		"@",       // empty agent
		"@Dev",    // uppercase
		"@dev[",   // unclosed bracket
		"@dev@",   // empty host
		"@dev#",   // empty session
		"@123dev", // starts with number
	}

	for _, addr := range cases {
		_, err := resolver.Resolve(addr)
		if err == nil {
			t.Errorf("expected error for invalid address %q", addr)
		}
	}
}

func TestLoadFrayAgents(t *testing.T) {
	frayDir := t.TempDir()
	jsonl := `{"type":"agent","guid":"usr-abc","agent_id":"alice","registered_at":1000}
{"type":"agent","guid":"usr-def","agent_id":"bob","registered_at":2000,"managed":true}
{"type":"agent_update","agent_id":"alice","status":"away"}
`
	if err := os.WriteFile(filepath.Join(frayDir, "agents.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write agents.jsonl: %v", err)
	}

	agents, err := LoadFrayAgents(filepath.Join(frayDir, "agents.jsonl"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}

	alice := agents["alice"]
	if alice == nil {
		t.Fatal("alice not found")
	}
	if alice.Status == nil || *alice.Status != "away" {
		t.Errorf("expected alice status 'away', got %v", alice.Status)
	}

	bob := agents["bob"]
	if bob == nil {
		t.Fatal("bob not found")
	}
	if !bob.Managed {
		t.Error("expected bob to be managed")
	}
}

func TestFrayAgentToIdentity(t *testing.T) {
	status := "available"
	purpose := "test agent"
	agent := &FrayAgent{
		GUID:         "usr-abc123",
		AgentID:      "test",
		Status:       &status,
		Purpose:      &purpose,
		RegisteredAt: 1234567890,
	}

	identity := FrayAgentToIdentity(agent)

	if identity.Record.Agent != "test" {
		t.Errorf("expected agent 'test', got %q", identity.Record.Agent)
	}
	if identity.Record.GUID != "usr-abc123" {
		t.Errorf("expected GUID 'usr-abc123', got %q", identity.Record.GUID)
	}
	if identity.Record.Address != "@test" {
		t.Errorf("expected address '@test', got %q", identity.Record.Address)
	}
	if identity.HasKey {
		t.Error("expected HasKey to be false")
	}
	if identity.Record.Metadata["status"] != "available" {
		t.Errorf("expected status 'available', got %q", identity.Record.Metadata["status"])
	}
	if identity.Record.Metadata["purpose"] != "test agent" {
		t.Errorf("expected purpose 'test agent', got %q", identity.Record.Metadata["purpose"])
	}
}
