package check

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erikolson/ratchet/internal/verdict"
)

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repoWith writes ratchet.yaml + a source file and commits, returning the root.
func repoWith(t *testing.T, manifestYAML string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte("code"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ratchet.yaml"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "init")
	return root
}

func fixedClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) }
}

func run(t *testing.T, root string, only []string, jsonMode bool) (Result, *bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	res, err := Run(Options{
		RepoRoot: root, Only: only, JSON: jsonMode,
		Now: fixedClock(), Stdout: &out, Stderr: &errBuf,
	})
	return res, &out, &errBuf, err
}

const twoPass = `
version: 0
capabilities:
  - name: alpha
    run: "sh -c 'exit 0'"
    verdict: exit
  - name: beta
    run: "sh -c 'exit 0'"
    verdict: exit
`

func TestCheck_AllPass(t *testing.T) {
	root := repoWith(t, twoPass)
	res, out, _, err := run(t, root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.ExitCode)
	}
	if len(res.Verdicts) != 2 {
		t.Fatalf("got %d verdicts, want 2", len(res.Verdicts))
	}
	if !strings.Contains(out.String(), "alpha") || !strings.Contains(out.String(), "beta") {
		t.Fatalf("green summary should list both capabilities: %q", out.String())
	}
	// Log written.
	raw, err := os.ReadFile(filepath.Join(root, ".ratchet", "verdicts.jsonl"))
	if err != nil {
		t.Fatalf("verdict log not written: %v", err)
	}
	if n := strings.Count(strings.TrimRight(string(raw), "\n"), "\n") + 1; n != 2 {
		t.Fatalf("log has %d lines, want 2", n)
	}
}

func TestCheck_FailExitOneAndShowsOutput(t *testing.T) {
	root := repoWith(t, `
version: 0
capabilities:
  - name: alpha
    run: "sh -c 'exit 0'"
    verdict: exit
  - name: beta
    run: "sh -c 'echo boom; exit 1'"
    verdict: exit
`)
	res, out, _, err := run(t, root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(out.String(), "FAIL") || !strings.Contains(out.String(), "boom") {
		t.Fatalf("red output should show FAIL and the tool output: %q", out.String())
	}
}

func TestCheck_ErrorDominatesFail(t *testing.T) {
	root := repoWith(t, `
version: 0
capabilities:
  - name: afail
    run: "sh -c 'exit 1'"
    verdict: exit
  - name: berror
    run: "sh -c 'exit 5'"
    verdict: exit
`)
	res, _, _, err := run(t, root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 2 {
		t.Fatalf("exit = %d, want 2 (error dominates fail)", res.ExitCode)
	}
}

func TestCheck_JSONMode(t *testing.T) {
	root := repoWith(t, twoPass)
	_, out, _, err := run(t, root, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("json stdout has %d lines, want 2: %q", len(lines), out.String())
	}
	for _, l := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Fatalf("json line not parseable: %v (%q)", err, l)
		}
		if v["kind"] != "check" {
			t.Fatalf("verdict kind = %v, want check", v["kind"])
		}
	}
}

func TestCheck_Subset(t *testing.T) {
	root := repoWith(t, twoPass)
	res, _, _, err := run(t, root, []string{"alpha"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Verdicts) != 1 || res.Verdicts[0].Capability != "alpha" {
		t.Fatalf("subset run produced %#v, want just alpha", res.Verdicts)
	}
}

func TestCheck_UnknownCapabilityIsError(t *testing.T) {
	root := repoWith(t, twoPass)
	if _, _, _, err := run(t, root, []string{"nope"}, false); err == nil {
		t.Fatal("unknown capability in subset should error (exit 3)")
	}
}

func TestCheck_VerdictIdentityWellFormed(t *testing.T) {
	root := repoWith(t, twoPass)
	res, _, _, err := run(t, root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	v := res.Verdicts[0]
	if v.Subject == "" {
		t.Fatal("subject empty")
	}
	if len(v.Oracle) != 64 {
		t.Fatalf("oracle = %q, want 64-char hash", v.Oracle)
	}
	if v.Kind != verdict.KindCheck {
		t.Fatalf("kind = %q, want check", v.Kind)
	}
	if v.Head == "" {
		t.Fatal("head provenance empty after a commit")
	}
	if v.Timestamp != "2026-07-14T12:00:00Z" {
		t.Fatalf("timestamp = %q, want RFC3339 UTC from injected clock", v.Timestamp)
	}
}

func TestCheck_NotAManifestIsError(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	// no ratchet.yaml
	if _, _, _, err := run(t, root, nil, false); err == nil {
		t.Fatal("missing manifest should error (exit 3)")
	}
}
