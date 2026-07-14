// Package gitx wraps the git plumbing ratchet needs: repo-root resolution, the
// content-addressed subject tree hash (ADR-0001), and HEAD/dirty provenance.
//
// git is a hard prerequisite for ratchet v0.
package gitx

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RatchetOwnedPaths are excluded from the subject tree: ratchet knows its own
// bookkeeping, so editing the manifest or a probe never fragments a code
// verdict's identity (ADR-0001). This is not language knowledge (which the
// design forbids) — just ratchet excluding its own files.
var RatchetOwnedPaths = []string{"ratchet.yaml", ".ratchet"}

// run executes git in root with optional extra environment, returning trimmed
// stdout. stderr is folded into the error for legibility.
func run(root string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// RepoRoot returns the absolute worktree root containing dir.
func RepoRoot(dir string) (string, error) {
	return run(dir, nil, "rev-parse", "--show-toplevel")
}

// Head returns the HEAD commit hash, or "" on an unborn branch (no commits yet).
func Head(root string) (string, error) {
	h, err := run(root, nil, "rev-parse", "--verify", "HEAD")
	if err != nil {
		// Unborn branch is not an error for provenance purposes.
		return "", nil
	}
	return h, nil
}

// SubjectTree computes the git tree hash of the code under judgment: the working
// tree (tracked plus untracked-but-not-ignored files) with exclude paths removed,
// via a throwaway index so the real index is never touched (ADR-0001). It also
// reports whether the working tree is dirty (provenance only).
func SubjectTree(root string, exclude []string) (tree string, dirty bool, err error) {
	idx, err := os.CreateTemp("", "ratchet-index-*")
	if err != nil {
		return "", false, err
	}
	idxPath := idx.Name()
	idx.Close()
	os.Remove(idxPath) // git creates it fresh; we only need a unique path
	defer os.Remove(idxPath)

	env := []string{"GIT_INDEX_FILE=" + idxPath}

	if _, err := run(root, env, "read-tree", "--empty"); err != nil {
		return "", false, err
	}
	// Stage everything in the working tree (respects .gitignore).
	if _, err := run(root, env, "add", "-A"); err != nil {
		return "", false, err
	}
	// Remove ratchet's own files from the throwaway index.
	for _, ex := range exclude {
		if _, err := run(root, env, "rm", "-r", "--cached", "--ignore-unmatch", "--", ex); err != nil {
			return "", false, err
		}
	}
	tree, err = run(root, env, "write-tree")
	if err != nil {
		return "", false, err
	}

	// Dirty = any uncommitted change or untracked non-ignored file (real index).
	status, err := run(root, nil, "status", "--porcelain")
	if err != nil {
		return "", false, err
	}
	return tree, status != "", nil
}
