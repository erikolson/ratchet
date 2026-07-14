// Package doctor verifies the verifier (SEED §7.2, ADR-0006). It calibrates each
// oracle in a throwaway worktree from HEAD: apply a ratified mutation patch, run
// the capabilities, and assert that the ones the probe declares it flips go to
// fail (not error) while the rest stay pass. An oracle that never says no is a
// rumor; doctor is where it is made to say no.
//
// Each probe (and the baseline) runs in its OWN fresh worktree. This is not just
// for parallelism-safety: a shared, mutated-in-place worktree lets one run's
// build caches (pyc, incremental build output, keyed on file mtime) leak into the
// next run's differently-mutated code, so a mutation can appear not to flip when
// it did. Fresh checkouts are the only language-agnostic way to guarantee each
// run sees exactly its own code. Doctor is not the hot path; check is.
package doctor

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

// prepareTimeout is generous: doctor is not the hot path, and prepare (npm ci,
// venv creation) is slow.
const prepareTimeout = 10 * time.Minute

// Options configures a doctor run.
type Options struct {
	RepoRoot string
	Now      func() time.Time
	Stdout   io.Writer
	Stderr   io.Writer
}

// Calibration is one capability's calibration outcome (pass=calibrated,
// fail=broken oracle, error=inconclusive).
type Calibration struct {
	Capability string
	Status     verdict.Status
	Detail     string
	Probes     []verdict.ProbeRecord
}

// Report is the outcome of a doctor run.
type Report struct {
	Calibrations   []Calibration
	Uncalibrated   []string
	BaselineErrors []baselineError
	ExitCode       int
}

type baselineError struct {
	capability string
	outcome    runner.Outcome
}

// probeRun is the result of running one probe (or the baseline) in its worktree.
type probeRun struct {
	stale          bool
	mutatedSubject string
	status         map[string]runner.Outcome
}

// Run calibrates every oracle declared in RepoRoot/ratchet.yaml.
func Run(opts Options) (Report, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	data, err := os.ReadFile(filepath.Join(opts.RepoRoot, "ratchet.yaml"))
	if err != nil {
		return Report{}, fmt.Errorf("reading ratchet.yaml: %w", err)
	}
	m, err := manifest.Parse(data, opts.RepoRoot)
	if err != nil {
		return Report{}, err
	}

	head, err := gitx.Head(opts.RepoRoot)
	if err != nil {
		return Report{}, err
	}
	if head == "" {
		return Report{}, fmt.Errorf("doctor requires at least one commit (HEAD is unborn)")
	}

	gitx.PruneWorktrees(opts.RepoRoot)

	// Baseline: a fresh HEAD checkout, unmodified. Its subject tree IS the
	// canonical subject at HEAD (clean, ratchet's own files excluded).
	base, err := runInWorktree(opts.RepoRoot, head, m, "")
	if err != nil {
		return Report{}, err
	}
	if base.prepareFail != nil {
		renderPrepareFailure(opts.Stdout, m.Prepare, *base.prepareFail)
		return Report{ExitCode: 1}, nil
	}
	subjectHEAD := base.run.mutatedSubject
	baseline := base.run.status

	// Each probe in its own fresh worktree.
	runs := map[string]probeRun{}
	for _, p := range m.Probes {
		res, err := runInWorktree(opts.RepoRoot, head, m, filepath.Join(opts.RepoRoot, p.Patch))
		if err != nil {
			return Report{}, err
		}
		if res.prepareFail != nil {
			// prepare failed in this probe's worktree; the probe cannot be judged.
			runs[p.Name] = probeRun{status: allError(m)}
			continue
		}
		runs[p.Name] = res.run
	}

	flippedBy := map[string][]manifest.Probe{}
	for _, p := range m.Probes {
		for _, c := range p.Flips {
			flippedBy[c] = append(flippedBy[c], p)
		}
	}

	logPath := filepath.Join(opts.RepoRoot, ".ratchet", "verdicts.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return Report{}, err
	}
	ts := now().UTC().Format(time.RFC3339)

	var report Report
	for _, c := range m.Capabilities {
		if baseline[c.Name].Status != verdict.StatusPass {
			report.BaselineErrors = append(report.BaselineErrors, baselineError{c.Name, baseline[c.Name]})
		}

		probes := flippedBy[c.Name]
		if len(probes) == 0 {
			report.Uncalibrated = append(report.Uncalibrated, c.Name)
			continue // uncalibrated emits no verdict — its absence is the state
		}

		cal := evaluate(c, m, baseline[c.Name], probes, runs, subjectHEAD)
		report.Calibrations = append(report.Calibrations, cal)

		oracle := verdict.OracleSpec{
			Adapter:        "exit",
			Version:        m.Version,
			Argv:           c.Argv,
			Pass:           c.Pass,
			Fail:           c.Fail,
			Timeout:        c.Timeout,
			IncludePrepare: true,
			Prepare:        m.PrepareArgv,
		}.Hash()
		v := verdict.Verdict{
			Capability: c.Name,
			Subject:    subjectHEAD,
			Oracle:     oracle,
			Kind:       verdict.KindCalibration,
			Status:     cal.Status,
			Head:       head,
			Timestamp:  ts,
			Probes:     cal.Probes,
		}
		if err := verdict.Append(logPath, v); err != nil {
			return Report{}, err
		}
	}

	report.ExitCode = exitCode(report)
	render(opts.Stdout, m, report)
	return report, nil
}

type worktreeResult struct {
	run         probeRun
	prepareFail *runner.Outcome
}

// runInWorktree checks out HEAD into a fresh worktree, optionally runs prepare,
// optionally applies patchAbs, then runs every capability. Empty patchAbs is the
// baseline.
func runInWorktree(root, head string, m *manifest.Manifest, patchAbs string) (worktreeResult, error) {
	wt, err := gitx.AddWorktree(root, head)
	if err != nil {
		return worktreeResult{}, fmt.Errorf("creating calibration worktree: %w", err)
	}
	defer gitx.RemoveWorktree(root, wt)

	if m.PrepareArgv != nil {
		out := runner.Run(wt, m.PrepareArgv, []int{0}, []int{1}, prepareTimeout)
		if out.Status != verdict.StatusPass {
			return worktreeResult{prepareFail: &out}, nil
		}
	}

	if patchAbs != "" {
		if err := gitx.ApplyCheck(wt, patchAbs); err != nil {
			return worktreeResult{run: probeRun{stale: true}}, nil
		}
		if err := gitx.Apply(wt, patchAbs); err != nil {
			return worktreeResult{run: probeRun{stale: true}}, nil
		}
	}

	mutated, _, err := gitx.SubjectTree(wt, gitx.RatchetOwnedPaths)
	if err != nil {
		return worktreeResult{}, fmt.Errorf("computing mutated subject: %w", err)
	}
	status := map[string]runner.Outcome{}
	for _, c := range m.Capabilities {
		status[c.Name] = runner.Run(wt, c.Argv, c.Pass, c.Fail, c.Timeout)
	}
	return worktreeResult{run: probeRun{mutatedSubject: mutated, status: status}}, nil
}

func allError(m *manifest.Manifest) map[string]runner.Outcome {
	out := map[string]runner.Outcome{}
	for _, c := range m.Capabilities {
		out[c.Name] = runner.Outcome{Status: verdict.StatusError, Reason: "prepare failed in calibration worktree"}
	}
	return out
}

// evaluate determines a capability's calibration status from its probes.
func evaluate(c manifest.Capability, m *manifest.Manifest, base runner.Outcome, probes []manifest.Probe, runs map[string]probeRun, subjectHEAD string) Calibration {
	cal := Calibration{Capability: c.Name}
	cal.Probes = append(cal.Probes, verdict.ProbeRecord{
		Name: "none", Expected: "pass", Observed: base.Status, MutatedSubject: subjectHEAD,
	})

	if base.Status != verdict.StatusPass {
		cal.Status = verdict.StatusError
		cal.Detail = fmt.Sprintf("baseline (unmodified code) returned %s, not pass — cannot calibrate without a clean baseline", base.Status)
		for _, p := range probes {
			cal.Probes = append(cal.Probes, probeRecord(p, c.Name, runs))
		}
		return cal
	}

	worst := verdict.StatusPass
	for _, p := range probes {
		cal.Probes = append(cal.Probes, probeRecord(p, c.Name, runs))
		ps, detail := probeVerdict(p, c.Name, m, runs)
		if worseThan(ps, worst) {
			worst = ps
			if detail != "" {
				cal.Detail = detail
			}
		}
	}
	cal.Status = worst
	return cal
}

// probeVerdict judges whether probe p correctly calibrates capability c.
func probeVerdict(p manifest.Probe, cap string, m *manifest.Manifest, runs map[string]probeRun) (verdict.Status, string) {
	r := runs[p.Name]
	if r.stale {
		return verdict.StatusError, fmt.Sprintf("probe %q is stale (patch no longer applies) — regenerate it", p.Name)
	}
	switch r.status[cap].Status {
	case verdict.StatusError:
		return verdict.StatusError, fmt.Sprintf("probe %q made the oracle error, not fail — the mutation broke the tool, not the behavior", p.Name)
	case verdict.StatusPass:
		return verdict.StatusFail, fmt.Sprintf("probe %q did not flip %q — the oracle does not detect this breakage", p.Name, cap)
	}
	flips := map[string]bool{}
	for _, f := range p.Flips {
		flips[f] = true
	}
	for _, other := range m.Capabilities {
		if flips[other.Name] {
			continue
		}
		if r.status[other.Name].Status != verdict.StatusPass {
			return verdict.StatusError, fmt.Sprintf("probe %q also disturbed %q (over-broad patch)", p.Name, other.Name)
		}
	}
	return verdict.StatusPass, ""
}

func probeRecord(p manifest.Probe, cap string, runs map[string]probeRun) verdict.ProbeRecord {
	r := runs[p.Name]
	rec := verdict.ProbeRecord{Name: p.Name, Expected: "fail", MutatedSubject: r.mutatedSubject}
	if r.stale {
		rec.Observed = verdict.StatusError
		return rec
	}
	rec.Observed = r.status[cap].Status
	return rec
}

func exitCode(r Report) int {
	if len(r.BaselineErrors) > 0 {
		return 1
	}
	for _, c := range r.Calibrations {
		if c.Status == verdict.StatusFail || c.Status == verdict.StatusError {
			return 1
		}
	}
	return 0
}

func rank(s verdict.Status) int {
	switch s {
	case verdict.StatusError:
		return 2
	case verdict.StatusFail:
		return 1
	default:
		return 0
	}
}

func worseThan(a, b verdict.Status) bool { return rank(a) > rank(b) }
