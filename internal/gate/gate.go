// Package gate is the single enforcement primitive both hook surfaces invoke
// (PreToolUse and git pre-commit), so they emit an identical kind=gate verdict
// (ADR-0008). It runs the full check (all capabilities, never a subset — the gate
// is not reachable by the developer-convenience subset form, Q7), records a
// block/allow decision that references the check verdicts by identity (Q4), and
// fails closed: anything nonzero denies.
package gate

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/erikolson/ratchet/internal/check"
	"github.com/erikolson/ratchet/internal/verdict"
)

// Options configures a gate run.
type Options struct {
	RepoRoot string
	Action   string // what is being gated, e.g. "git commit"
	Now      func() time.Time
	Stdout   io.Writer
	Stderr   io.Writer
}

// Result is the gate decision. ExitCode is 0 to allow, nonzero to deny.
type Result struct {
	Blocked  bool
	ExitCode int
	Refs     []string
}

// Run runs the full check and gates on its result.
func Run(opts Options) (Result, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	res, err := check.Run(check.Options{
		RepoRoot: opts.RepoRoot,
		Only:     nil, // always all capabilities — the gate never accepts a subset
		JSON:     false,
		Now:      now,
		Stdout:   opts.Stdout,
		Stderr:   opts.Stderr,
	})
	if err != nil {
		// Couldn't run (bad/missing manifest, not a git repo): fail closed.
		fmt.Fprintf(opts.Stdout, "\n✗ BLOCKED  %s\n   %v\n   (fail closed: a harness that cannot run is not a control)\n", opts.Action, err)
		return Result{Blocked: true, ExitCode: 3}, nil
	}

	refs := make([]string, len(res.Verdicts))
	for i, v := range res.Verdicts {
		refs[i] = v.Identity()
	}
	var subject, head string
	var dirty bool
	if len(res.Verdicts) > 0 {
		subject = res.Verdicts[0].Subject
		head = res.Verdicts[0].Head
		dirty = res.Verdicts[0].Dirty
	}

	decision := "allow"
	blocked := false
	if res.ExitCode != 0 {
		decision = "block"
		blocked = true
	}

	gv := verdict.GateVerdict(subject, head, dirty, decision, opts.Action, refs, 0, now().UTC().Format(time.RFC3339))
	logPath := filepath.Join(opts.RepoRoot, ".ratchet", "verdicts.jsonl")
	if err := verdict.Append(logPath, gv); err != nil {
		return Result{}, err
	}

	if blocked {
		fmt.Fprintf(opts.Stdout, "\n✗ BLOCKED  %s\n   see the failing capability above · receipt: .ratchet/verdicts.jsonl\n", opts.Action)
	}

	return Result{Blocked: blocked, ExitCode: res.ExitCode, Refs: refs}, nil
}
