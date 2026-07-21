package ratify

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/erikolson/ratchet/internal/oracles"
	"github.com/erikolson/ratchet/internal/ossification"
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

// repo commits base, then overwrites the working-tree manifest with head.
func repo(t *testing.T, base, head string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	write(t, root, base)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "base")
	write(t, root, head)
	return root
}

func write(t *testing.T, root, manifest string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "ratchet.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

const base = `version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
`
const weakened = `version: 0
capabilities:
  - { name: test, run: "pytest -q --ignore=tests", verdict: exit }
`

func run(t *testing.T, opts Options) (Result, error) {
	t.Helper()
	if opts.RepoRoot == "" {
		t.Fatal("RepoRoot required")
	}
	if opts.BaseRef == "" {
		opts.BaseRef = "HEAD"
	}
	if opts.Now == "" {
		opts.Now = "2026-07-21T00:00:00Z"
	}
	var out bytes.Buffer
	opts.Stdout = &out
	return Run(opts)
}

func TestRatify_WeakeningThenDiffClears(t *testing.T) {
	root := repo(t, base, weakened)
	if _, err := run(t, Options{RepoRoot: root, Capability: "test", Requester: "agent", Ratifier: "alice"}); err != nil {
		t.Fatalf("ratify: %v", err)
	}
	// End to end: the same weakening now clears diff-oracles.
	var buf bytes.Buffer
	rep, err := oracles.Diff(root, "HEAD", &buf)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if rep.ExitCode != 0 {
		t.Fatalf("after ratification the weakening must clear (exit 0); got %d\n%s", rep.ExitCode, buf.String())
	}
}

func TestRatify_RequiresRequester(t *testing.T) {
	root := repo(t, base, weakened)
	if _, err := run(t, Options{RepoRoot: root, Capability: "test", Ratifier: "alice"}); err == nil {
		t.Fatal("ratify without --requester must error")
	}
}

func TestRatify_RefusesSelfRatification(t *testing.T) {
	root := repo(t, base, weakened)
	if _, err := run(t, Options{RepoRoot: root, Capability: "test", Requester: "agent", Ratifier: "agent"}); err == nil {
		t.Fatal("ratifier == requester must be refused for an approval (SEED §5)")
	}
}

func TestRatify_RejectionByProposerIsAllowed(t *testing.T) {
	root := repo(t, base, weakened)
	res, err := run(t, Options{RepoRoot: root, Capability: "test", Requester: "agent", Ratifier: "agent", Reject: true})
	if err != nil {
		t.Fatalf("a rejection by the proposer is legitimate: %v", err)
	}
	if res.Entry.Decision != ossification.Rejected {
		t.Fatalf("decision=%q, want rejected", res.Entry.Decision)
	}
	// A rejection does not clear the weakening.
	var buf bytes.Buffer
	rep, _ := oracles.Diff(root, "HEAD", &buf)
	if rep.ExitCode == 0 {
		t.Fatal("a rejected change must still alarm in diff-oracles")
	}
}

func TestRatify_NothingToRatifyOnUnchanged(t *testing.T) {
	root := repo(t, base, base) // identical working tree
	if _, err := run(t, Options{RepoRoot: root, Capability: "test", Requester: "agent", Ratifier: "alice"}); err == nil {
		t.Fatal("ratifying an unchanged oracle must error (nothing to ratify)")
	}
}

func TestRatify_NothingToRatifyOnAddition(t *testing.T) {
	head := `version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
  - { name: lint, run: "ruff check", verdict: exit }
`
	root := repo(t, base, head)
	if _, err := run(t, Options{RepoRoot: root, Capability: "lint", Requester: "agent", Ratifier: "alice"}); err == nil {
		t.Fatal("ratifying an addition (tightening) must error — it needs no ratification")
	}
}

func TestRatify_RemovalRecordsEmptyNewOracle(t *testing.T) {
	twoCap := `version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
  - { name: lint, run: "ruff check", verdict: exit }
`
	root := repo(t, twoCap, base) // lint removed
	res, err := run(t, Options{RepoRoot: root, Capability: "lint", Requester: "agent", Ratifier: "alice"})
	if err != nil {
		t.Fatalf("ratifying a removal: %v", err)
	}
	if res.Entry.NewOracle != "" {
		t.Fatalf("a removal must record an empty new oracle, got %q", res.Entry.NewOracle)
	}
	var buf bytes.Buffer
	rep, _ := oracles.Diff(root, "HEAD", &buf)
	if rep.ExitCode != 0 {
		t.Fatalf("a ratified removal must clear; got %d\n%s", rep.ExitCode, buf.String())
	}
}
