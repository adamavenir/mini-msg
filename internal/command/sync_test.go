package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/hostedsync"
)

func setupSyncProject(t *testing.T, channelName string) string {
	t.Helper()

	projectDir := t.TempDir()
	project, err := core.InitProject(projectDir, false)
	if err != nil {
		t.Fatalf("init project: %v", err)
	}

	sharedDir := filepath.Join(projectDir, ".fray", "shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("mkdir shared: %v", err)
	}

	if _, err := db.UpdateProjectConfig(projectDir, db.ProjectConfig{
		StorageVersion: 2,
		ChannelID:      "ch-sync",
		ChannelName:    channelName,
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		t.Fatalf("init schema: %v", err)
	}
	_ = dbConn.Close()

	return projectDir
}

func setupHostedSyncProject(t *testing.T, channelName string) string {
	t.Helper()

	projectDir := setupSyncProject(t, channelName)

	localDir := filepath.Join(projectDir, ".fray", "local")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "machine-id"), []byte(`{"id":"laptop","seq":0,"created_at":1}`), 0o644); err != nil {
		t.Fatalf("write machine-id: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".fray", "shared", "machines", "laptop"), 0o755); err != nil {
		t.Fatalf("mkdir machine dir: %v", err)
	}

	return projectDir
}

func TestSyncSetupPathCreatesSymlink(t *testing.T) {
	projectDir := setupSyncProject(t, "sync-path")
	frayDir := filepath.Join(projectDir, ".fray")

	markerPath := filepath.Join(frayDir, "shared", "marker.txt")
	if err := os.WriteFile(markerPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	baseDir := filepath.Join(t.TempDir(), "sync-root")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "sync", "setup", "--path", baseDir); err != nil {
		t.Fatalf("sync setup: %v", err)
	}

	expectedTarget := filepath.Join(baseDir, "sync-path", "shared")
	info, err := os.Lstat(filepath.Join(frayDir, "shared"))
	if err != nil {
		t.Fatalf("stat shared: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected shared to be symlink")
	}
	link, err := os.Readlink(filepath.Join(frayDir, "shared"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if filepath.Clean(link) != filepath.Clean(expectedTarget) {
		t.Fatalf("expected link target %s, got %s", expectedTarget, link)
	}
	if _, err := os.Stat(filepath.Join(expectedTarget, "marker.txt")); err != nil {
		t.Fatalf("expected marker moved to target: %v", err)
	}
}

func TestSyncSetupICloudCreatesSymlink(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := setupSyncProject(t, "sync-icloud")
	frayDir := filepath.Join(projectDir, ".fray")

	if err := os.WriteFile(filepath.Join(frayDir, "shared", "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "sync", "setup", "--icloud"); err != nil {
		t.Fatalf("sync setup icloud: %v", err)
	}

	expectedTarget := filepath.Join(homeDir, "Library", "Mobile Documents", "com~apple~CloudDocs", "fray-sync", "sync-icloud", "shared")
	link, err := os.Readlink(filepath.Join(frayDir, "shared"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if filepath.Clean(link) != filepath.Clean(expectedTarget) {
		t.Fatalf("expected link target %s, got %s", expectedTarget, link)
	}
	if _, err := os.Stat(filepath.Join(expectedTarget, "marker.txt")); err != nil {
		t.Fatalf("expected marker moved to target: %v", err)
	}
}

func TestSyncStatusShowsConfiguration(t *testing.T) {
	projectDir := setupSyncProject(t, "sync-status")

	config := &db.ProjectSyncConfig{Backend: "path", Path: "/tmp/sync/shared"}
	if _, err := db.UpdateProjectConfig(projectDir, db.ProjectConfig{Sync: config}); err != nil {
		t.Fatalf("update sync config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	output, err := executeCommand(cmd, "--json", "sync", "status")
	if err != nil {
		t.Fatalf("sync status: %v", err)
	}
	var result syncStatusResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !result.Configured || result.Backend != "path" || result.Path != "/tmp/sync/shared" {
		t.Fatalf("unexpected status: %#v", result)
	}
}

func TestSyncSetupHostedRegistersMachine(t *testing.T) {
	projectDir := setupHostedSyncProject(t, "sync-hosted")

	var gotReq hostedsync.RegisterMachineRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sync/register-machine" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode register: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"machine_id":"laptop","token":"tok-123"}`))
	}))
	defer server.Close()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "sync", "setup", "--hosted", server.URL); err != nil {
		t.Fatalf("sync setup hosted: %v", err)
	}

	if gotReq.ChannelID != "ch-sync" || gotReq.MachineID != "laptop" {
		t.Fatalf("unexpected register payload: %#v", gotReq)
	}

	config, err := db.ReadProjectConfig(projectDir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if config == nil || config.Sync == nil || config.Sync.Backend != "hosted" {
		t.Fatalf("expected hosted sync config, got %#v", config)
	}
	if config.Sync.HostedURL != server.URL {
		t.Fatalf("expected hosted url %s, got %s", server.URL, config.Sync.HostedURL)
	}

	auth, err := hostedsync.LoadAuth(projectDir)
	if err != nil {
		t.Fatalf("load auth: %v", err)
	}
	if auth == nil || auth.Token != "tok-123" {
		t.Fatalf("expected auth token saved, got %#v", auth)
	}
	if auth.MachineID != "laptop" {
		t.Fatalf("expected machine id laptop, got %q", auth.MachineID)
	}
}
