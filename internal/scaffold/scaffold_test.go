package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erikolson/ratchet/internal/manifest"
)

func run(t *testing.T, root string) (Result, string) {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(Options{RepoRoot: root, Stdout: &out})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return res, out.String()
}

func readManifest(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "ratchet.yaml"))
	if err != nil {
		t.Fatalf("ratchet.yaml not written: %v", err)
	}
	return string(b)
}

func TestInit_DetectsGoAndProposesCommented(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)

	res, _ := run(t, root)
	if len(res.Detected) == 0 {
		t.Fatal("expected go.mod detection")
	}
	man := readManifest(t, root)
	if !strings.Contains(man, "go test ./...") {
		t.Fatalf("expected proposed go test command:\n%s", man)
	}
	if !strings.Contains(man, "detected go.mod") {
		t.Fatalf("proposal must be labeled as a detected guess:\n%s", man)
	}
	// Every proposed command is commented — nothing is active.
	for _, line := range strings.Split(man, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "name:") {
			t.Fatalf("found an ACTIVE (uncommented) capability line: %q", line)
		}
	}
}

func TestInit_WrittenManifestIsInertUntilRatified(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	run(t, root)

	// The freshly-init'd manifest must not enforce anything: parsing it fails
	// with the empty-capabilities error until a human uncomments (Q8 / ADR-0009).
	man := readManifest(t, root)
	if _, err := manifest.Parse([]byte(man), root); err == nil {
		t.Fatal("init wrote a manifest that enforces something before ratification")
	}
}

func TestInit_NothingDetectedRefusesAndAsks(t *testing.T) {
	root := t.TempDir() // no ecosystem files
	res, out := run(t, root)
	if len(res.Detected) != 0 {
		t.Fatalf("expected no detection, got %v", res.Detected)
	}
	man := readManifest(t, root)
	// No invented commands.
	for _, guess := range []string{"go test", "pytest", "npm test", "cargo test"} {
		if strings.Contains(man, guess) {
			t.Fatalf("init invented a command with nothing detected: %q\n%s", guess, man)
		}
	}
	if !strings.Contains(strings.ToLower(out+man), "no ecosystem") &&
		!strings.Contains(strings.ToLower(out+man), "nothing") {
		t.Fatalf("expected a refuse-and-ask message:\nSTDOUT+MANIFEST:\n%s\n%s", out, man)
	}
}

func TestInit_RefusesToClobber(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "ratchet.yaml"), []byte("version: 0\n"), 0o644)
	if _, err := Run(Options{RepoRoot: root, Stdout: &bytes.Buffer{}}); err == nil {
		t.Fatal("expected refusal to overwrite an existing ratchet.yaml")
	}
}

func TestInit_WritesGitignore(t *testing.T) {
	root := t.TempDir()
	run(t, root)
	b, err := os.ReadFile(filepath.Join(root, ".ratchet", ".gitignore"))
	if err != nil {
		t.Fatalf(".ratchet/.gitignore not written: %v", err)
	}
	if !strings.Contains(string(b), "verdicts.jsonl") {
		t.Fatalf(".ratchet/.gitignore should ignore verdicts.jsonl:\n%s", b)
	}
}
