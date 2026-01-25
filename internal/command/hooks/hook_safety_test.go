package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSafetyHooksWritesScriptAndSettings(t *testing.T) {
	projectDir := t.TempDir()
	var out bytes.Buffer

	if err := installSafetyHooks(projectDir, false, &out, false); err != nil {
		t.Fatalf("installSafetyHooks error: %v", err)
	}

	guardPath := filepath.Join(projectDir, ".claude", "hooks", "fray_safety_guard.py")
	if _, err := os.Stat(guardPath); err != nil {
		t.Fatalf("expected guard script at %s: %v", guardPath, err)
	}

	script, err := os.ReadFile(guardPath)
	if err != nil {
		t.Fatalf("read guard script: %v", err)
	}
	if !bytes.Contains(script, []byte("Fray Safety Guard")) {
		t.Fatalf("guard script missing expected content")
	}

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	settings := readHookSettings(t, settingsPath)
	preToolUse, ok := settings.Hooks["PreToolUse"].([]any)
	if !ok || len(preToolUse) == 0 {
		t.Fatalf("expected PreToolUse hooks in settings")
	}
	if !hooksContainCommand(preToolUse, "fray_safety_guard.py") {
		t.Fatalf("expected guard command in PreToolUse hooks")
	}
}

func TestUninstallSafetyHooksRemovesGuard(t *testing.T) {
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	hooksDir := filepath.Join(claudeDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}

	guardPath := filepath.Join(hooksDir, "fray_safety_guard.py")
	if err := os.WriteFile(guardPath, []byte("# guard\n"), 0o755); err != nil {
		t.Fatalf("write guard: %v", err)
	}

	guardCmd := "python3 $CLAUDE_PROJECT_DIR/.claude/hooks/fray_safety_guard.py"
	otherCmd := "echo ok"
	settings := hookSettings{
		Hooks: map[string]any{
			"PreToolUse": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": guardCmd}}},
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": otherCmd}}},
			},
		},
	}
	writeHookSettings(t, filepath.Join(claudeDir, "settings.local.json"), settings)

	var out bytes.Buffer
	if err := uninstallSafetyHooks(projectDir, &out, false); err != nil {
		t.Fatalf("uninstallSafetyHooks error: %v", err)
	}

	if _, err := os.Stat(guardPath); !os.IsNotExist(err) {
		t.Fatalf("expected guard script removed, got %v", err)
	}

	updated := readHookSettings(t, filepath.Join(claudeDir, "settings.local.json"))
	preToolUse, ok := updated.Hooks["PreToolUse"].([]any)
	if !ok || len(preToolUse) == 0 {
		t.Fatalf("expected remaining PreToolUse hooks")
	}
	if hooksContainCommand(preToolUse, "fray_safety_guard.py") {
		t.Fatalf("guard command should have been removed")
	}
	if !hooksContainCommand(preToolUse, otherCmd) {
		t.Fatalf("expected non-guard hook to remain")
	}
}

func TestUninstallIntegrationHooksRemovesFrayHooks(t *testing.T) {
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude: %v", err)
	}

	settings := hookSettings{
		Hooks: map[string]any{
			"SessionStart":     []any{map[string]any{"hooks": []any{map[string]any{"command": "fray hook-session startup"}}}},
			"UserPromptSubmit": []any{map[string]any{"hooks": []any{map[string]any{"command": "fray hook-prompt"}}}},
			"Custom":           []any{map[string]any{"hooks": []any{map[string]any{"command": "echo ok"}}}},
		},
	}
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	writeHookSettings(t, settingsPath, settings)

	var out bytes.Buffer
	if err := uninstallIntegrationHooks(projectDir, &out, false); err != nil {
		t.Fatalf("uninstallIntegrationHooks error: %v", err)
	}

	updated := readHookSettings(t, settingsPath)
	if _, ok := updated.Hooks["SessionStart"]; ok {
		t.Fatalf("expected SessionStart hook removed")
	}
	if _, ok := updated.Hooks["UserPromptSubmit"]; ok {
		t.Fatalf("expected UserPromptSubmit hook removed")
	}
	if _, ok := updated.Hooks["Custom"]; !ok {
		t.Fatalf("expected custom hook to remain")
	}
}

func readHookSettings(t *testing.T, path string) hookSettings {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings hookSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if settings.Hooks == nil {
		settings.Hooks = map[string]any{}
	}
	return settings
}

func writeHookSettings(t *testing.T, path string, settings hookSettings) {
	t.Helper()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

func hooksContainCommand(hooks []any, needle string) bool {
	for _, hook := range hooks {
		hookMap, ok := hook.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, ok := hm["command"].(string)
			if ok && strings.Contains(cmd, needle) {
				return true
			}
		}
	}
	return false
}
