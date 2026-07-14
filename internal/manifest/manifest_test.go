package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeProbe creates repoRoot/.ratchet/probes/<name> so patch-existence checks pass.
func writeProbe(t *testing.T, repoRoot, rel string) {
	t.Helper()
	p := filepath.Join(repoRoot, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("--- a\n+++ b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParse_ValidAppliesDefaultsAndTokenizes(t *testing.T) {
	src := `
version: 0
capabilities:
  - { name: lint, run: "ruff check", verdict: exit }
  - { name: test, run: "pytest -q", verdict: exit, pass: [0], fail: [1], timeout: 30s }
`
	m, err := Parse([]byte(src), "")
	if err != nil {
		t.Fatalf("Parse valid manifest: %v", err)
	}
	if m.Version != 0 {
		t.Fatalf("version = %d, want 0", m.Version)
	}
	if len(m.Capabilities) != 2 {
		t.Fatalf("got %d capabilities, want 2", len(m.Capabilities))
	}

	lint := m.Capabilities[0]
	if lint.Name != "lint" {
		t.Fatalf("cap[0].Name = %q, want lint (declaration order preserved)", lint.Name)
	}
	if got, want := lint.Argv, []string{"ruff", "check"}; !equalStrings(got, want) {
		t.Fatalf("lint.Argv = %#v, want %#v", got, want)
	}
	if !equalInts(lint.Pass, []int{0}) {
		t.Fatalf("lint.Pass = %v, want default [0]", lint.Pass)
	}
	if !equalInts(lint.Fail, []int{1}) {
		t.Fatalf("lint.Fail = %v, want default [1]", lint.Fail)
	}
	if lint.Timeout != DefaultTimeout {
		t.Fatalf("lint.Timeout = %v, want default %v", lint.Timeout, DefaultTimeout)
	}

	test := m.Capabilities[1]
	if test.Timeout != 30*time.Second {
		t.Fatalf("test.Timeout = %v, want 30s", test.Timeout)
	}
}

func TestParse_ValidProbe(t *testing.T) {
	root := t.TempDir()
	writeProbe(t, root, ".ratchet/probes/negate.patch")
	src := `
version: 0
capabilities:
  - { name: test, run: "pytest -q", verdict: exit }
probes:
  - { name: negate, patch: .ratchet/probes/negate.patch, flips: [test] }
`
	m, err := Parse([]byte(src), root)
	if err != nil {
		t.Fatalf("Parse valid probe manifest: %v", err)
	}
	if len(m.Probes) != 1 || m.Probes[0].Name != "negate" {
		t.Fatalf("probes = %#v, want one named negate", m.Probes)
	}
	if !equalStrings(m.Probes[0].Flips, []string{"test"}) {
		t.Fatalf("flips = %v", m.Probes[0].Flips)
	}
}

func TestParse_Rejects(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantSub string // substring the error message must contain
	}{
		{"version missing", `
capabilities:
  - { name: test, run: "pytest", verdict: exit }
`, "version"},
		{"version too new", `
version: 1
capabilities:
  - { name: test, run: "pytest", verdict: exit }
`, "newer ratchet"},
		{"unknown top-level key", `
version: 0
capabilties:
  - { name: test, run: "pytest", verdict: exit }
`, "capabilties"},
		{"unknown capability key", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit, verdct: exit }
`, "verdct"},
		{"verdict missing", `
version: 0
capabilities:
  - { name: test, run: "pytest" }
`, "verdict"},
		{"verdict unknown adapter", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: json/pytest }
`, "adapter"},
		{"empty capabilities", `
version: 0
capabilities: []
`, "no capabilities"},
		{"bad name charset", `
version: 0
capabilities:
  - { name: "Test Suite", run: "pytest", verdict: exit }
`, "name"},
		{"duplicate capability name", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit }
  - { name: test, run: "pytest -q", verdict: exit }
`, "duplicate"},
		{"run metacharacter", `
version: 0
capabilities:
  - { name: test, run: "pytest && mypy", verdict: exit }
`, "metacharacter"},
		{"empty run", `
version: 0
capabilities:
  - { name: test, run: "", verdict: exit }
`, "run"},
		{"pass empty", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit, pass: [] }
`, "pass"},
		{"fail empty", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit, fail: [] }
`, "fail"},
		{"pass and fail overlap", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit, pass: [0], fail: [0] }
`, "overlap"},
		{"timeout not positive", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit, timeout: 0s }
`, "timeout"},
		{"flips unknown capability", `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit }
probes:
  - { name: p, patch: x.patch, flips: [nope] }
`, "nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.src), "")
			if err == nil {
				t.Fatalf("Parse(%s) = nil error, want rejection", tc.name)
			}
			var me *Error
			if !errors.As(err, &me) {
				t.Fatalf("Parse(%s) error type = %T, want *manifest.Error", tc.name, err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Parse(%s) error = %q, want substring %q", tc.name, err.Error(), tc.wantSub)
			}
		})
	}
}

func TestParse_VacuousProbeRejected(t *testing.T) {
	// non-empty patch with empty flips: asserts a mutation that flips nothing.
	root := t.TempDir()
	writeProbe(t, root, ".ratchet/probes/v.patch")
	src := `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit }
probes:
  - { name: v, patch: .ratchet/probes/v.patch, flips: [] }
`
	if _, err := Parse([]byte(src), root); err == nil {
		t.Fatal("vacuous probe (patch, no flips) accepted, want rejection")
	}
}

func TestParse_MissingPatchFileRejected(t *testing.T) {
	root := t.TempDir() // no patch written
	src := `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit }
probes:
  - { name: gone, patch: .ratchet/probes/gone.patch, flips: [test] }
`
	_, err := Parse([]byte(src), root)
	if err == nil {
		t.Fatal("missing patch file accepted, want rejection (un-calibration is loud everywhere)")
	}
	if !strings.Contains(err.Error(), "gone.patch") {
		t.Fatalf("error = %q, want it to name the missing patch", err.Error())
	}
}

func TestParse_DuplicateProbeNameRejected(t *testing.T) {
	root := t.TempDir()
	writeProbe(t, root, ".ratchet/probes/a.patch")
	src := `
version: 0
capabilities:
  - { name: test, run: "pytest", verdict: exit }
probes:
  - { name: dup, patch: .ratchet/probes/a.patch, flips: [test] }
  - { name: dup, patch: .ratchet/probes/a.patch, flips: [test] }
`
	if _, err := Parse([]byte(src), root); err == nil {
		t.Fatal("duplicate probe name accepted, want rejection")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
