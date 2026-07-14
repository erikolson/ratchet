package doctor

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

func gitCmd(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitCapture(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = gitEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out.Bytes()
}

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
	)
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitCmd(t, root, "init", "-b", "main")
	return root
}

// makePatch generates a git patch that changes rel to `mutated`, then restores rel.
func makePatch(t *testing.T, root, rel, mutated string) []byte {
	t.Helper()
	orig, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, rel, mutated)
	patch := gitCapture(t, root, "diff", "--", rel)
	writeFile(t, root, rel, string(orig)) // restore
	if len(patch) == 0 {
		t.Fatalf("makePatch produced an empty diff for %s", rel)
	}
	return patch
}

func run(t *testing.T, root string) (Report, *bytes.Buffer, error) {
	t.Helper()
	var out bytes.Buffer
	rep, err := Run(Options{
		RepoRoot: root,
		Now:      func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) },
		Stdout:   &out,
		Stderr:   &out,
	})
	return rep, &out, err
}

const calcSrc = "def refund_ok(amount):\n    return amount > 100\n"

const calcTest = `import unittest
from calc import refund_ok

class T(unittest.TestCase):
    def test_high(self):
        self.assertTrue(refund_ok(150))
    def test_low(self):
        self.assertFalse(refund_ok(50))
`

func requirePython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
}

// flipFixture: a real unittest suite with a real negate-return probe.
func flipFixture(t *testing.T) string {
	root := initRepo(t)
	writeFile(t, root, "calc.py", calcSrc)
	writeFile(t, root, "test_calc.py", calcTest)
	writeFile(t, root, ".ratchet/.gitignore", "verdicts.jsonl\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "src")
	// A behaviorally-wrong-but-valid mutation: > becomes <.
	patch := makePatch(t, root, "calc.py", "def refund_ok(amount):\n    return amount < 100\n")
	writeFile(t, root, ".ratchet/probes/negate.patch", string(patch))
	writeFile(t, root, "ratchet.yaml", `version: 0
capabilities:
  - name: test
    run: "python3 -m unittest"
    verdict: exit
probes:
  - name: negate
    patch: .ratchet/probes/negate.patch
    flips: [test]
`)
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "manifest")
	return root
}

func TestDoctor_FlipCalibrates(t *testing.T) {
	requirePython(t)
	root := flipFixture(t)
	rep, out, err := run(t, root)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if rep.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0\n%s", rep.ExitCode, out)
	}
	cal := findCal(t, rep, "test")
	if cal.Status != verdict.StatusPass {
		t.Fatalf("test calibration = %q, want pass\n%s", cal.Status, out)
	}
	// A calibration verdict was written with kind=calibration and nested probes.
	raw, err := os.ReadFile(filepath.Join(root, ".ratchet", "verdicts.jsonl"))
	if err != nil {
		t.Fatalf("calibration verdict not logged: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &v); err != nil {
		t.Fatalf("calibration line not JSON: %v", err)
	}
	if v["kind"] != "calibration" {
		t.Fatalf("kind = %v, want calibration", v["kind"])
	}
	probes, ok := v["probes"].([]any)
	if !ok || len(probes) < 2 {
		t.Fatalf("expected nested probes (baseline + negate), got %v", v["probes"])
	}
}

// brokenFixture: the test never exercises the mutated code, so the mutation does
// not flip it — a vacuous oracle doctor must catch.
func TestDoctor_BrokenOracleCaught(t *testing.T) {
	requirePython(t)
	root := initRepo(t)
	writeFile(t, root, "calc.py", calcSrc)
	// This "suite" asserts a constant; it never calls refund_ok.
	writeFile(t, root, "test_calc.py", "import unittest\nclass T(unittest.TestCase):\n    def test(self):\n        self.assertTrue(True)\n")
	writeFile(t, root, ".ratchet/.gitignore", "verdicts.jsonl\n")
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "src")
	patch := makePatch(t, root, "calc.py", "def refund_ok(amount):\n    return amount < 100\n")
	writeFile(t, root, ".ratchet/probes/negate.patch", string(patch))
	writeFile(t, root, "ratchet.yaml", `version: 0
capabilities:
  - name: test
    run: "python3 -m unittest"
    verdict: exit
probes:
  - name: negate
    patch: .ratchet/probes/negate.patch
    flips: [test]
`)
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "manifest")

	rep, out, err := run(t, root)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if rep.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (broken oracle)\n%s", rep.ExitCode, out)
	}
	if cal := findCal(t, rep, "test"); cal.Status != verdict.StatusFail {
		t.Fatalf("test calibration = %q, want fail (oracle did not flip)", cal.Status)
	}
}

func TestDoctor_Uncalibrated(t *testing.T) {
	requirePython(t)
	root := initRepo(t)
	writeFile(t, root, "calc.py", calcSrc)
	writeFile(t, root, "test_calc.py", calcTest)
	writeFile(t, root, ".ratchet/.gitignore", "verdicts.jsonl\n")
	writeFile(t, root, "ratchet.yaml", `version: 0
capabilities:
  - name: test
    run: "python3 -m unittest"
    verdict: exit
`)
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "init")

	rep, out, err := run(t, root)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if rep.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (uncalibrated warns but passes)\n%s", rep.ExitCode, out)
	}
	if !strings.Contains(out.String(), "UNCALIBRATED") {
		t.Fatalf("expected loud UNCALIBRATED warning:\n%s", out)
	}
	// Uncalibrated emits NO calibration verdict.
	if p := filepath.Join(root, ".ratchet", "verdicts.jsonl"); fileNonEmpty(p) {
		t.Fatal("uncalibrated capability must emit no calibration verdict")
	}
}

func TestDoctor_StalePatchIsError(t *testing.T) {
	requirePython(t)
	root := initRepo(t)
	writeFile(t, root, "calc.py", calcSrc)
	writeFile(t, root, "test_calc.py", calcTest)
	writeFile(t, root, ".ratchet/.gitignore", "verdicts.jsonl\n")
	// A patch that references context not present in calc.py -> will not apply.
	stale := "--- a/calc.py\n+++ b/calc.py\n@@ -1,1 +1,1 @@\n-this line does not exist\n+neither does this\n"
	writeFile(t, root, ".ratchet/probes/stale.patch", stale)
	writeFile(t, root, "ratchet.yaml", `version: 0
capabilities:
  - name: test
    run: "python3 -m unittest"
    verdict: exit
probes:
  - name: stale
    patch: .ratchet/probes/stale.patch
    flips: [test]
`)
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "init")

	rep, out, err := run(t, root)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if rep.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (stale patch)\n%s", rep.ExitCode, out)
	}
	if cal := findCal(t, rep, "test"); cal.Status != verdict.StatusError {
		t.Fatalf("stale patch calibration = %q, want error", cal.Status)
	}
	if !strings.Contains(strings.ToLower(out.String()), "stale") {
		t.Fatalf("expected 'stale' in output:\n%s", out)
	}
}

// venvFixture: the capability runs a gitignored interpreter, absent from a fresh
// worktree unless prepare recreates it.
func venvFixture(t *testing.T, prepare string) string {
	root := initRepo(t)
	writeFile(t, root, "calc.py", calcSrc)
	writeFile(t, root, "test_calc.py", calcTest)
	writeFile(t, root, ".gitignore", ".venv/\n")
	writeFile(t, root, ".ratchet/.gitignore", "verdicts.jsonl\n")
	manifest := "version: 0\n"
	if prepare != "" {
		manifest += "prepare: \"" + prepare + "\"\n"
	}
	manifest += `capabilities:
  - name: test
    run: ".venv/bin/python -m unittest"
    verdict: exit
`
	writeFile(t, root, "ratchet.yaml", manifest)
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "init")
	return root
}

func TestDoctor_MissingDepBaselineErrorIsLegible(t *testing.T) {
	requirePython(t)
	root := venvFixture(t, "") // no prepare
	rep, out, err := run(t, root)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if rep.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1 (baseline error)\n%s", rep.ExitCode, out)
	}
	low := strings.ToLower(out.String())
	if !strings.Contains(low, "prepare") || !strings.Contains(low, "depend") {
		t.Fatalf("baseline error should name likely cause (dependencies) and suggest prepare:\n%s", out)
	}
}

func TestDoctor_PrepareFixesBaseline(t *testing.T) {
	requirePython(t)
	root := venvFixture(t, "python3 -m venv --without-pip .venv")
	rep, out, err := run(t, root)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if rep.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0 (prepare makes baseline runnable; capability uncalibrated)\n%s", rep.ExitCode, out)
	}
}

func findCal(t *testing.T, rep Report, cap string) Calibration {
	t.Helper()
	for _, c := range rep.Calibrations {
		if c.Capability == cap {
			return c
		}
	}
	t.Fatalf("no calibration for capability %q", cap)
	return Calibration{}
}

func fileNonEmpty(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Size() > 0
}
