package doctor

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/erikolson/ratchet/internal/manifest"
	"github.com/erikolson/ratchet/internal/runner"
	"github.com/erikolson/ratchet/internal/verdict"
)

// render prints per-capability calibration status. Doctor may guess at causes in
// prose; it never guesses at verdicts (ADR-0006).
func render(w io.Writer, m *manifest.Manifest, r Report) {
	uncal := map[string]bool{}
	for _, name := range r.Uncalibrated {
		uncal[name] = true
	}
	calByName := map[string]Calibration{}
	for _, c := range r.Calibrations {
		calByName[c.Capability] = c
	}
	baseErr := map[string]runner.Outcome{}
	for _, be := range r.BaselineErrors {
		baseErr[be.capability] = be.outcome
	}

	var calibrated, broken int
	for _, c := range m.Capabilities {
		if out, ok := baseErr[c.Name]; ok {
			renderBaselineError(w, c, out)
			broken++
			continue
		}
		switch {
		case uncal[c.Name]:
			fmt.Fprintf(w, "⚠ %-12s UNCALIBRATED — no probe. This oracle has never been observed to say no.\n", c.Name)
		default:
			cal := calByName[c.Name]
			switch cal.Status {
			case verdict.StatusPass:
				calibrated++
				fmt.Fprintf(w, "✓ %-12s calibrated (%d probe(s))\n", c.Name, len(cal.Probes)-1)
			default:
				broken++
				label := "BROKEN"
				if cal.Status == verdict.StatusError {
					label = "ERROR"
				}
				fmt.Fprintf(w, "✗ %-12s %s — %s\n", c.Name, label, cal.Detail)
			}
		}
	}

	total := len(m.Capabilities)
	fmt.Fprintf(w, "\n%d/%d calibrated", calibrated, total)
	if broken > 0 {
		fmt.Fprintf(w, ", %d broken", broken)
	}
	if n := len(r.Uncalibrated); n > 0 {
		fmt.Fprintf(w, ", %d uncalibrated", n)
	}
	fmt.Fprintln(w, ".")
}

func renderBaselineError(w io.Writer, c manifest.Capability, out runner.Outcome) {
	fmt.Fprintf(w, "✗ %-12s calibration FAILED\n", c.Name)
	fmt.Fprintf(w, "  Baseline probe (unmodified code) returned %s, not pass.\n", out.Status)
	fmt.Fprintln(w, "  The oracle could not run in the calibration worktree.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  Most likely cause: missing dependencies. A fresh git worktree contains only")
	fmt.Fprintln(w, "  tracked files — .venv/, node_modules/, target/ are gitignored and therefore absent.")
	writeBlock(w, c.Run, out.Output)
	fmt.Fprintln(w, "  → Declare a prepare step in ratchet.yaml, e.g.:")
	fmt.Fprintln(w, "        prepare: \"python3 -m venv .venv\"")
}

func renderPrepareFailure(w io.Writer, prepare string, out runner.Outcome) {
	fmt.Fprintln(w, "✗ calibration FAILED — prepare step did not succeed")
	fmt.Fprintf(w, "  prepare: %s\n", prepare)
	fmt.Fprintf(w, "  returned %s", out.Status)
	if out.Reason != "" {
		fmt.Fprintf(w, " (%s)", out.Reason)
	}
	fmt.Fprintln(w, "")
	writeBlock(w, prepare, out.Output)
	fmt.Fprintln(w, "  The calibration worktree could not be set up, so nothing was calibrated.")
}

func writeBlock(w io.Writer, cmd string, output []byte) {
	if len(bytes.TrimSpace(output)) == 0 {
		return
	}
	fmt.Fprintf(w, "  ┌─ %s\n", cmd)
	sc := bufio.NewScanner(bytes.NewReader(output))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		fmt.Fprintf(w, "  │ %s\n", sc.Text())
	}
	fmt.Fprintln(w, "  └─")
}
