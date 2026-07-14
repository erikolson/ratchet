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

// ShowFile returns the raw bytes of path at ref (e.g. "origin/main:ratchet.yaml"),
// used to read the ratified manifest on the base branch (ADR-0008).
func ShowFile(root, ref, path string) ([]byte, error) {
	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = root
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w: %s", ref, path, err, strings.TrimSpace(errBuf.String()))
	}
	return []byte(out.String()), nil
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

// AddWorktree checks out commit into a fresh temporary worktree (detached) and
// returns its path. Doctor calibrates in a worktree, never the working tree
// (ADR-0006).
func AddWorktree(root, commit string) (string, error) {
	dir, err := os.MkdirTemp("", "ratchet-wt-*")
	if err != nil {
		return "", err
	}
	if _, err := run(root, nil, "worktree", "add", "--detach", "--quiet", dir, commit); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// RemoveWorktree tears down a worktree created by AddWorktree.
func RemoveWorktree(root, path string) {
	_, _ = run(root, nil, "worktree", "remove", "--force", path)
	os.RemoveAll(path)
}

// PruneWorktrees clears git's administrative records for worktrees whose
// directories have already been deleted (e.g. by a previous crash).
func PruneWorktrees(root string) {
	_, _ = run(root, nil, "worktree", "prune")
}

// ApplyCheck reports whether patchPath applies cleanly in worktree, without
// applying it. A failure here means the probe is stale (ADR-0006).
func ApplyCheck(worktree, patchPath string) error {
	_, err := run(worktree, nil, "apply", "--check", patchPath)
	return err
}

// Apply applies patchPath in worktree.
func Apply(worktree, patchPath string) error {
	_, err := run(worktree, nil, "apply", patchPath)
	return err
}

// ApplyReverse reverses patchPath in worktree, restoring the clean baseline.
func ApplyReverse(worktree, patchPath string) error {
	_, err := run(worktree, nil, "apply", "-R", patchPath)
	return err
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
