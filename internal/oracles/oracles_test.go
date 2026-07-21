package oracles

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/erikolson/ratchet/internal/manifest"
	"github.com/erikolson/ratchet/internal/ossification"
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

// baseRepo commits baseManifest, then overwrites ratchet.yaml with headManifest
// (uncommitted working-tree change), returning the root.
func baseRepo(t *testing.T, baseManifest, headManifest string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "ratchet.yaml"), []byte(baseManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(root, "ratchet.yaml"), []byte(headManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func diff(t *testing.T, root string) (Report, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	rep, err := Diff(root, "HEAD", &out)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	return rep, &out
}

const oneCap = `version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
`

func has(rep Report, cap string, kind ChangeKind) bool {
	for _, c := range rep.Changes {
		if c.Capability == cap && c.Kind == kind {
			return true
		}
	}
	return false
}

func TestDiff_NoChange(t *testing.T) {
	root := baseRepo(t, oneCap, oneCap)
	rep, _ := diff(t, root)
	if rep.ExitCode != 0 || len(rep.Changes) != 0 {
		t.Fatalf("identical manifest: exit=%d changes=%v, want 0/none", rep.ExitCode, rep.Changes)
	}
}

func TestDiff_TightenIsSilent(t *testing.T) {
	// add a capability — tightening.
	head := `version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
  - { name: lint, run: "ruff check", verdict: exit }
`
	root := baseRepo(t, oneCap, head)
	rep, _ := diff(t, root)
	if rep.ExitCode != 0 {
		t.Fatalf("tightening (added capability) exit=%d, want 0 (no alarm)", rep.ExitCode)
	}
	if !has(rep, "lint", Added) {
		t.Fatalf("expected lint Added, got %v", rep.Changes)
	}
	if has(rep, "test", Changed) || has(rep, "test", Removed) {
		t.Fatal("test should be unchanged")
	}
}

func TestDiff_WeakenAlarms(t *testing.T) {
	head := `version: 0
capabilities:
  - { name: test, run: "pytest -q --ignore=tests", verdict: exit }
`
	root := baseRepo(t, oneCap, head)
	rep, out := diff(t, root)
	if rep.ExitCode == 0 {
		t.Fatalf("weakening a command must alarm (nonzero exit); got 0")
	}
	if !has(rep, "test", Changed) {
		t.Fatalf("expected test Changed, got %v", rep.Changes)
	}
	if !bytes.Contains(out.Bytes(), []byte("CHANGED")) {
		t.Fatalf("output should flag the change:\n%s", out)
	}
}

// oracleHash returns the oracle hash of capability cap in a manifest string,
// exactly as diff computes it — so a ratification entry can be content-addressed
// to match.
func oracleHash(t *testing.T, manifestYAML, cap string) string {
	t.Helper()
	m, err := manifest.Parse([]byte(manifestYAML), "")
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	for _, c := range m.Capabilities {
		if c.Name == cap {
			return verdict.OracleSpec{Adapter: "exit", Version: m.Version, Argv: c.Argv, Pass: c.Pass, Fail: c.Fail, Timeout: c.Timeout}.Hash()
		}
	}
	t.Fatalf("capability %q not in manifest", cap)
	return ""
}

func writeRatification(t *testing.T, root string, e ossification.Entry) {
	t.Helper()
	if err := ossification.Append(ossification.Path(root), e); err != nil {
		t.Fatalf("write ratification: %v", err)
	}
}

func TestDiff_WeakenClearedByRatification(t *testing.T) {
	head := `version: 0
capabilities:
  - { name: test, run: "pytest -q --ignore=tests", verdict: exit }
`
	root := baseRepo(t, oneCap, head)
	writeRatification(t, root, ossification.Entry{
		Capability: "test",
		BaseOracle: oracleHash(t, oneCap, "test"),
		NewOracle:  oracleHash(t, head, "test"),
		Requester:  "agent", Ratifier: "alice", Decision: ossification.Ratified,
		Timestamp: "2026-07-21T00:00:00Z",
	})
	rep, out := diff(t, root)
	if rep.ExitCode != 0 {
		t.Fatalf("a ratified weakening must clear (exit 0); got %d\n%s", rep.ExitCode, out)
	}
	if !bytes.Contains(out.Bytes(), []byte("RATIFIED")) {
		t.Fatalf("output should record the ratification:\n%s", out)
	}
}

func TestDiff_SelfRatificationStillAlarms(t *testing.T) {
	head := `version: 0
capabilities:
  - { name: test, run: "pytest -q --ignore=tests", verdict: exit }
`
	root := baseRepo(t, oneCap, head)
	writeRatification(t, root, ossification.Entry{
		Capability: "test",
		BaseOracle: oracleHash(t, oneCap, "test"),
		NewOracle:  oracleHash(t, head, "test"),
		Requester:  "agent", Ratifier: "agent", Decision: ossification.Ratified,
		Timestamp: "2026-07-21T00:00:00Z",
	})
	rep, _ := diff(t, root)
	if rep.ExitCode == 0 {
		t.Fatal("a self-ratification (requester == ratifier) must NOT clear the weakening")
	}
}

func TestDiff_RemoveAlarmsLoudest(t *testing.T) {
	base := `version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
  - { name: lint, run: "ruff check", verdict: exit }
`
	head := oneCap // lint removed
	root := baseRepo(t, base, head)
	rep, out := diff(t, root)
	if rep.ExitCode == 0 {
		t.Fatalf("removing a capability must alarm; got 0")
	}
	if !has(rep, "lint", Removed) {
		t.Fatalf("expected lint Removed, got %v", rep.Changes)
	}
	if !bytes.Contains(out.Bytes(), []byte("REMOVED")) {
		t.Fatalf("output should flag the removal:\n%s", out)
	}
}
