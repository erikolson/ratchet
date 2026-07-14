package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return root
}

func TestInstall_WritesBothLoci(t *testing.T) {
	root := initRepo(t)
	res, err := Install(Options{RepoRoot: root, Binary: "/usr/local/bin/ratchet"})
	if err != nil {
		t.Fatal(err)
	}

	// git pre-commit hook, executable, invokes ratchet gate.
	pc := filepath.Join(root, ".git", "hooks", "pre-commit")
	info, err := os.Stat(pc)
	if err != nil {
		t.Fatalf("pre-commit not written: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatal("pre-commit hook is not executable")
	}
	body, _ := os.ReadFile(pc)
	if !strings.Contains(string(body), "ratchet") || !strings.Contains(string(body), "gate") {
		t.Fatalf("pre-commit hook does not invoke ratchet gate:\n%s", body)
	}

	// .claude/settings.json has a PreToolUse hook invoking ratchet.
	sp := filepath.Join(root, ".claude", "settings.json")
	raw, err := os.ReadFile(sp)
	if err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	if !strings.Contains(string(raw), "PreToolUse") || !strings.Contains(string(raw), "ratchet") {
		t.Fatalf("settings.json missing PreToolUse ratchet hook:\n%s", raw)
	}
	if res.GitHook == "" || res.ClaudeSettings == "" {
		t.Fatalf("result missing paths: %+v", res)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	root := initRepo(t)
	if _, err := Install(Options{RepoRoot: root, Binary: "/bin/ratchet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(Options{RepoRoot: root, Binary: "/bin/ratchet"}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	// The PreToolUse hook must not be duplicated.
	raw, _ := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if n := strings.Count(string(raw), "__gate-hook"); n != 1 {
		t.Fatalf("PreToolUse hook appears %d times, want 1 (idempotent)", n)
	}
}

func TestInstall_PreservesExistingSettings(t *testing.T) {
	root := initRepo(t)
	dir := filepath.Join(root, ".claude")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model":"opus","hooks":{"PreToolUse":[]}}`), 0o644)

	if _, err := Install(Options{RepoRoot: root, Binary: "/bin/ratchet"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s["model"] != "opus" {
		t.Fatalf("install clobbered existing settings: %s", raw)
	}
}

func TestInstall_RefusesToClobberForeignPreCommit(t *testing.T) {
	root := initRepo(t)
	hooksDir := filepath.Join(root, ".git", "hooks")
	os.MkdirAll(hooksDir, 0o755)
	os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte("#!/bin/sh\necho someone elses hook\n"), 0o755)

	if _, err := Install(Options{RepoRoot: root, Binary: "/bin/ratchet"}); err == nil {
		t.Fatal("expected refusal to overwrite a non-ratchet pre-commit hook")
	}
}
