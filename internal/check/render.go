package check

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/erikolson/ratchet/internal/verdict"
)

// renderHuman prints a compact green line when everything passes (forgettable,
// §7.3), or a loud per-capability breakdown with the failing tool's output when
// anything is red. The green tick is what makes the red one credible.
func renderHuman(w io.Writer, results []capResult, exit int) {
	if exit == 0 {
		names := make([]string, len(results))
		for i, r := range results {
			names[i] = r.cap.Name
		}
		fmt.Fprintf(w, "✓ %s\n", strings.Join(names, " · "))
		return
	}

	var fails, errs int
	for _, r := range results {
		switch r.outcome.Status {
		case verdict.StatusPass:
			fmt.Fprintf(w, "✓ %s\n", r.cap.Name)
		case verdict.StatusFail:
			fails++
			fmt.Fprintf(w, "✗ %s    FAIL\n", r.cap.Name)
			writeBlock(w, r.cap.Run, r.outcome.Output)
		case verdict.StatusError:
			errs++
			fmt.Fprintf(w, "✗ %s    ERROR — %s\n", r.cap.Name, r.outcome.Reason)
			writeBlock(w, r.cap.Run, r.outcome.Output)
		}
	}
	fmt.Fprintf(w, "\n%s\n", summary(fails, errs))
}

// writeBlock prints captured tool output framed and indented under the command.
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

func summary(fails, errs int) string {
	switch {
	case errs > 0 && fails > 0:
		return fmt.Sprintf("%d error(s) and %d failure(s) — your harness is broken and your code is broken.", errs, fails)
	case errs > 0:
		return fmt.Sprintf("%d error(s) — your harness is broken (the oracle did not adjudicate).", errs)
	default:
		return fmt.Sprintf("%d capabilit%s failed.", fails, plural(fails))
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// renderJSON writes one verdict object per line to stdout; failing tool output
// goes to stderr. The hook reads stdout (ADR-0005).
func renderJSON(stdout, stderr io.Writer, results []capResult) {
	for _, r := range results {
		line, err := verdict.Marshal(r.verdict)
		if err != nil {
			continue
		}
		fmt.Fprintf(stdout, "%s\n", line)
		if r.outcome.Status != verdict.StatusPass && len(bytes.TrimSpace(r.outcome.Output)) > 0 {
			fmt.Fprintf(stderr, "=== %s (%s) ===\n%s\n", r.cap.Name, r.outcome.Status, r.outcome.Output)
		}
	}
}
