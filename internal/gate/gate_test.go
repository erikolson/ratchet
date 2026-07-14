package gate

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func run(t *testing.T, root string) (Result, *bytes.Buffer, error) {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(Options{
		RepoRoot: root, Action: "git commit",
		Now:    func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) },
		Stdout: &out, Stderr: &out,
	})
	return res, &out, err
}

func readLog(t *testing.T, root string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".ratchet", "verdicts.jsonl"))
	if err != nil {
		t.Fatalf("no verdict log: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("bad log line: %v", err)
		}
		out = append(out, v)
	}
	return out
}

const green = `
version: 0
capabilities:
  - name: alpha
    run: "sh -c 'exit 0'"
    verdict: exit
  - name: beta
    run: "sh -c 'exit 0'"
    verdict: exit
`

func TestGate_AllowOnGreen(t *testing.T) {
	root := repoWith(t, green)
	res, _, err := run(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked || res.ExitCode != 0 {
		t.Fatalf("green run blocked=%v exit=%d, want allow/0", res.Blocked, res.ExitCode)
	}
	log := readLog(t, root)
	gate := log[len(log)-1]
	if gate["kind"] != "gate" || gate["decision"] != "allow" {
		t.Fatalf("last log entry = %v, want gate/allow", gate)
	}
}

func TestGate_BlockOnRed(t *testing.T) {
	root := repoWith(t, `
version: 0
capabilities:
  - name: alpha
    run: "sh -c 'exit 0'"
    verdict: exit
  - name: beta
    run: "sh -c 'exit 1'"
    verdict: exit
`)
	res, out, err := run(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked || res.ExitCode == 0 {
		t.Fatalf("red run blocked=%v exit=%d, want block/nonzero", res.Blocked, res.ExitCode)
	}
	if !strings.Contains(out.String(), "BLOCKED") {
		t.Fatalf("blocked output should say BLOCKED:\n%s", out)
	}
	gate := readLog(t, root)
	last := gate[len(gate)-1]
	if last["decision"] != "block" {
		t.Fatalf("gate decision = %v, want block", last["decision"])
	}
}

func TestGate_ReferencesCheckVerdictsByIdentity(t *testing.T) {
	root := repoWith(t, green)
	res, _, err := run(t, root)
	if err != nil {
		t.Fatal(err)
	}
	log := readLog(t, root)
	// The check verdicts come first, then the gate verdict last.
	gate := log[len(log)-1]
	refs, ok := gate["refs"].([]any)
	if !ok || len(refs) != 2 {
		t.Fatalf("gate refs = %v, want 2 identities", gate["refs"])
	}
	// Each ref is an identity string of a check verdict, not an embedded object.
	for _, r := range refs {
		s, ok := r.(string)
		if !ok || !strings.Contains(s, "@check@tree:") {
			t.Fatalf("ref %v is not a check identity string", r)
		}
	}
	// The gate verdict must not re-embed check verdicts (no nested status/oracle).
	if _, embedded := gate["status"]; embedded {
		t.Fatal("gate verdict must not carry a per-capability status (it is not a check)")
	}
	if len(res.Refs) != 2 {
		t.Fatalf("Result.Refs = %v, want 2", res.Refs)
	}
}

func TestGate_FailsClosedOnBrokenManifest(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "ratchet.yaml"), []byte("version: 0\ncapabilities: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "init")
	res, _, err := run(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked || res.ExitCode != 3 {
		t.Fatalf("broken manifest blocked=%v exit=%d, want block/3 (fail closed)", res.Blocked, res.ExitCode)
	}
}
