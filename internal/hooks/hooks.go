// Package hooks installs the local gate at both loci SEED §10.4 names: a Claude
// Code PreToolUse hook (fast in-agent feedback) and a git pre-commit hook (the
// human backstop). Both invoke one `ratchet gate` primitive so the two surfaces
// emit an identical kind=gate verdict (ADR-0008).
//
// This is the harness-specific generator: ratchet's contract is portable, but the
// hook wiring for a particular harness (Claude Code) is generated here.
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const marker = "ratchet-managed"

// Options configures installation.
type Options struct {
	RepoRoot string
	Binary   string // absolute path to the ratchet binary the hooks invoke
}

// Result reports what was written.
type Result struct {
	GitHook        string
	ClaudeSettings string
	Wrote          []string
}

// Install writes both hook surfaces. It is idempotent and refuses to overwrite a
// pre-commit hook it did not write.
func Install(opts Options) (Result, error) {
	var res Result

	if err := installGitHook(opts, &res); err != nil {
		return res, err
	}
	if err := installClaudeHook(opts, &res); err != nil {
		return res, err
	}
	return res, nil
}

func gitHookBody(binary string) string {
	return fmt.Sprintf("#!/bin/sh\n"+
		"# %s: block a commit when a declared capability is red (ADR-0008).\n"+
		"# Local gate — fast feedback and a receipt, not a guarantee against a\n"+
		"# determined agent. Prevention lives off-machine (CI / pre-receive).\n"+
		"exec %q gate --action \"git commit\"\n", marker, binary)
}

func installGitHook(opts Options, res *Result) error {
	hooksDir := filepath.Join(opts.RepoRoot, ".git", "hooks")
	preCommit := filepath.Join(hooksDir, "pre-commit")

	if existing, err := os.ReadFile(preCommit); err == nil {
		if !strings.Contains(string(existing), marker) {
			return fmt.Errorf(".git/hooks/pre-commit exists and is not %s; refusing to overwrite it", marker)
		}
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(preCommit, []byte(gitHookBody(opts.Binary)), 0o755); err != nil {
		return err
	}
	res.GitHook = preCommit
	res.Wrote = append(res.Wrote, preCommit)
	return nil
}

// installClaudeHook merges a PreToolUse hook into .claude/settings.json without
// disturbing existing settings, and without duplicating on re-run.
func installClaudeHook(opts Options, res *Result) error {
	dir := filepath.Join(opts.RepoRoot, ".claude")
	path := filepath.Join(dir, "settings.json")

	settings := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}

	command := fmt.Sprintf("%s __gate-hook", opts.Binary)

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)

	// Idempotency: skip if our command is already registered.
	for _, entry := range pre {
		if entryHasCommand(entry, command) {
			res.ClaudeSettings = path
			return nil
		}
	}

	pre = append(pre, map[string]any{
		"matcher": "Bash",
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	})
	hooks["PreToolUse"] = pre
	settings["hooks"] = hooks

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	res.ClaudeSettings = path
	res.Wrote = append(res.Wrote, path)
	return nil
}

func entryHasCommand(entry any, command string) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	inner, ok := m["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range inner {
		hm, ok := h.(map[string]any)
		if ok && hm["command"] == command {
			return true
		}
	}
	return false
}
