package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitCmd(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitCmd(t, root, "init", "-b", "main")
	return root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T, root string) {
	t.Helper()
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "x")
}

func TestRepoRoot(t *testing.T) {
	root := initRepo(t)
	write(t, root, "pkg/keep.txt", "x")
	sub := filepath.Join(root, "pkg")
	got, err := RepoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Fatalf("RepoRoot(%q) = %q, want %q", sub, gotResolved, want)
	}
}

func TestHead(t *testing.T) {
	root := initRepo(t)
	write(t, root, "a.txt", "x")
	commitAll(t, root)
	h, err := Head(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 40 {
		t.Fatalf("Head() = %q, want 40-char hash", h)
	}
}

func TestSubjectTree_ExcludesRatchetOwnedFiles(t *testing.T) {
	root := initRepo(t)
	write(t, root, "src/app.py", "code")
	write(t, root, "ratchet.yaml", "version: 0\n")
	write(t, root, ".ratchet/probes/p.patch", "x")
	commitAll(t, root)

	t1, _, err := SubjectTree(root, RatchetOwnedPaths)
	if err != nil {
		t.Fatal(err)
	}

	// Editing ratchet's own files must NOT change the subject.
	write(t, root, "ratchet.yaml", "version: 0\n# edited\n")
	write(t, root, ".ratchet/probes/p.patch", "changed")
	t2, _, err := SubjectTree(root, RatchetOwnedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if t1 != t2 {
		t.Fatalf("editing ratchet.yaml/.ratchet changed subject %s -> %s (ADR-0001 violation)", t1, t2)
	}

	// Editing real code MUST change the subject.
	write(t, root, "src/app.py", "code2")
	t3, _, err := SubjectTree(root, RatchetOwnedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if t3 == t1 {
		t.Fatal("editing source did not change subject")
	}
}

func TestSubjectTree_DirtyProvenance(t *testing.T) {
	root := initRepo(t)
	write(t, root, "src/app.py", "code")
	commitAll(t, root)

	if _, dirty, _ := SubjectTree(root, RatchetOwnedPaths); dirty {
		t.Fatal("clean tree reported dirty")
	}
	write(t, root, "src/app.py", "edited")
	if _, dirty, _ := SubjectTree(root, RatchetOwnedPaths); !dirty {
		t.Fatal("edited tracked file not reported dirty")
	}
}

func TestSubjectTree_IgnoredExcluded_UntrackedIncluded(t *testing.T) {
	root := initRepo(t)
	write(t, root, ".gitignore", "*.log\n")
	write(t, root, "src/a.py", "x")
	commitAll(t, root)

	base, dirty, err := SubjectTree(root, RatchetOwnedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Fatal("committed tree reported dirty")
	}

	// An ignored file must not affect the subject or dirtiness.
	write(t, root, "debug.log", "junk")
	s2, d2, _ := SubjectTree(root, RatchetOwnedPaths)
	if s2 != base {
		t.Fatal("ignored file changed the subject")
	}
	if d2 {
		t.Fatal("ignored untracked file marked tree dirty")
	}

	// An untracked, non-ignored file must be included.
	write(t, root, "src/b.py", "new")
	s3, d3, _ := SubjectTree(root, RatchetOwnedPaths)
	if s3 == base {
		t.Fatal("untracked non-ignored file was not included in the subject")
	}
	if !d3 {
		t.Fatal("untracked non-ignored file did not mark tree dirty")
	}
}
