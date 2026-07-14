// Package check runs a manifest's capabilities, emits a normalized verdict per
// capability to the append-only log, renders the outcome, and folds the results
// into the aggregate exit code (0 pass / 1 fail / 2 error), with error dominating
// fail (ADR-0002, -0001, -0005).
package check

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/erikolson/ratchet/internal/gitx"
	"github.com/erikolson/ratchet/internal/manifest"
	"github.com/erikolson/ratchet/internal/runner"
	"github.com/erikolson/ratchet/internal/verdict"
)

// Options configures a check run.
type Options struct {
	RepoRoot string
	Only     []string // capability subset; empty = all
	JSON     bool
	Now      func() time.Time
	Stdout   io.Writer
	Stderr   io.Writer
}

// Result is the outcome of a check run. ExitCode is 0/1/2; couldn't-run
// conditions are returned as an error (the CLI maps those to exit 3).
type Result struct {
	Verdicts []verdict.Verdict
	ExitCode int
}

type capResult struct {
	cap     manifest.Capability
	outcome runner.Outcome
	verdict verdict.Verdict
}

// Run loads the manifest at RepoRoot/ratchet.yaml and executes it.
func Run(opts Options) (Result, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	data, err := os.ReadFile(filepath.Join(opts.RepoRoot, "ratchet.yaml"))
	if err != nil {
		return Result{}, fmt.Errorf("reading ratchet.yaml: %w", err)
	}
	m, err := manifest.Parse(data, opts.RepoRoot)
	if err != nil {
		return Result{}, err
	}

	caps, err := selectCaps(m.Capabilities, opts.Only)
	if err != nil {
		return Result{}, err
	}

	subject, dirty, err := gitx.SubjectTree(opts.RepoRoot, gitx.RatchetOwnedPaths)
	if err != nil {
		return Result{}, fmt.Errorf("computing subject tree: %w", err)
	}
	head, err := gitx.Head(opts.RepoRoot)
	if err != nil {
		return Result{}, fmt.Errorf("reading HEAD: %w", err)
	}

	logPath := filepath.Join(opts.RepoRoot, ".ratchet", "verdicts.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return Result{}, err
	}

	ts := now().UTC().Format(time.RFC3339)
	var results []capResult
	for _, c := range caps {
		oracle := verdict.OracleSpec{
			Adapter: "exit",
			Version: m.Version,
			Argv:    c.Argv,
			Pass:    c.Pass,
			Fail:    c.Fail,
			Timeout: c.Timeout,
		}.Hash()

		out := runner.Run(opts.RepoRoot, c.Argv, c.Pass, c.Fail, c.Timeout)

		v := verdict.Verdict{
			Capability: c.Name,
			Subject:    subject,
			Oracle:     oracle,
			Kind:       verdict.KindCheck,
			Status:     out.Status,
			Head:       head,
			Dirty:      dirty,
			DurationMs: out.Duration.Milliseconds(),
			Timestamp:  ts,
		}
		if err := verdict.Append(logPath, v); err != nil {
			return Result{}, err
		}
		results = append(results, capResult{cap: c, outcome: out, verdict: v})
	}

	exit := aggregate(results)
	if opts.JSON {
		renderJSON(opts.Stdout, opts.Stderr, results)
	} else {
		renderHuman(opts.Stdout, results, exit)
	}

	vs := make([]verdict.Verdict, len(results))
	for i, r := range results {
		vs[i] = r.verdict
	}
	return Result{Verdicts: vs, ExitCode: exit}, nil
}

// selectCaps filters to the requested subset, preserving declaration order, and
// errors on an unknown name (a bad argument is exit 3, not a verdict).
func selectCaps(all []manifest.Capability, only []string) ([]manifest.Capability, error) {
	if len(only) == 0 {
		return all, nil
	}
	byName := map[string]manifest.Capability{}
	for _, c := range all {
		byName[c.Name] = c
	}
	for _, name := range only {
		if _, ok := byName[name]; !ok {
			return nil, fmt.Errorf("unknown capability %q", name)
		}
	}
	var out []manifest.Capability
	for _, c := range all { // declaration order
		for _, name := range only {
			if c.Name == name {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func aggregate(results []capResult) int {
	hasFail, hasError := false, false
	for _, r := range results {
		switch r.outcome.Status {
		case verdict.StatusFail:
			hasFail = true
		case verdict.StatusError:
			hasError = true
		}
	}
	switch {
	case hasError:
		return 2
	case hasFail:
		return 1
	default:
		return 0
	}
}
