// Package scaffold implements `ratchet init`: propose a manifest for a human to
// ratify. init is Generate, not Verify (ADR-0009) — ecosystem detection is
// legitimate here because a human ratifies. Every detected command is written
// commented and labeled as a guess; nothing is active until a human uncomments
// it, so a fresh manifest enforces nothing and `check` refuses until ratified.
package scaffold

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options configures init.
type Options struct {
	RepoRoot string
	Stdout   io.Writer
}

// Result reports what init did.
type Result struct {
	Path     string
	Detected []string // ecosystem marker files recognized
	Wrote    bool
}

type capSuggestion struct {
	name string
	run  string
}

type detector struct {
	file string
	caps []capSuggestion
}

// detectors is a small, deliberately incomplete table. It is right for common
// ecosystems and silently wrong for the sixth (a Makefile, a `just` recipe, a
// monorepo) — which is exactly why every suggestion is commented and labeled a
// guess, to provoke ratification rather than substitute for it (ADR-0009).
var detectors = []detector{
	{"go.mod", []capSuggestion{{"test", "go test ./..."}, {"vet", "go vet ./..."}}},
	{"package.json", []capSuggestion{{"test", "npm test"}}},
	{"pyproject.toml", []capSuggestion{{"test", "pytest -q"}}},
	{"setup.py", []capSuggestion{{"test", "pytest -q"}}},
	{"Cargo.toml", []capSuggestion{{"test", "cargo test"}}},
}

// Run writes a proposed ratchet.yaml (refusing to overwrite an existing one) and
// a .ratchet/.gitignore.
func Run(opts Options) (Result, error) {
	res := Result{Path: filepath.Join(opts.RepoRoot, "ratchet.yaml")}

	if _, err := os.Stat(res.Path); err == nil {
		return res, fmt.Errorf("ratchet.yaml already exists — refusing to overwrite a ratified manifest")
	}

	var blocks []string
	seen := map[string]bool{}
	for _, d := range detectors {
		if _, err := os.Stat(filepath.Join(opts.RepoRoot, d.file)); err != nil {
			continue
		}
		res.Detected = append(res.Detected, d.file)
		for _, c := range d.caps {
			if seen[c.name] {
				continue
			}
			seen[c.name] = true
			blocks = append(blocks, commentedCapability(d.file, c))
		}
	}

	content := render(res.Detected, blocks)
	if err := os.WriteFile(res.Path, []byte(content), 0o644); err != nil {
		return res, err
	}
	res.Wrote = true

	// .ratchet/.gitignore keeps the verdict stream local (ADR-0005).
	ratchetDir := filepath.Join(opts.RepoRoot, ".ratchet")
	if err := os.MkdirAll(ratchetDir, 0o755); err != nil {
		return res, err
	}
	gi := filepath.Join(ratchetDir, ".gitignore")
	if _, err := os.Stat(gi); err != nil {
		if err := os.WriteFile(gi, []byte("# the verdict stream is local exhaust, not committed substrate (ADR-0005)\nverdicts.jsonl\n"), 0o644); err != nil {
			return res, err
		}
	}

	printSummary(opts.Stdout, res)
	return res, nil
}

func commentedCapability(file string, c capSuggestion) string {
	return fmt.Sprintf(
		"  # detected %s — is this how you verify here? uncomment to ratify.\n"+
			"  # - name: %s\n"+
			"  #   run: %q\n"+
			"  #   verdict: exit\n",
		file, c.name, c.run)
}

func render(detected, blocks []string) string {
	var b strings.Builder
	b.WriteString("# ratchet.yaml — declare what \"verified\" means in this repo.\n#\n")
	b.WriteString("# ratchet ran no command below. Every line is a PROPOSAL to ratify: uncomment\n")
	b.WriteString("# the capabilities that are true here, fix the commands, delete the rest.\n")
	b.WriteString("# Nothing is enforced until you uncomment it. One capability = one command =\n")
	b.WriteString("# one verdict (exit 0 = pass); for a pipeline, put it in a script.\n#\n")

	if len(detected) == 0 {
		b.WriteString("# ratchet detected no ecosystem it recognizes, so it guessed nothing.\n")
		b.WriteString("# Tell it what \"verified\" means here — there is nothing it can invent for you.\n\n")
	} else {
		fmt.Fprintf(&b, "# detected: %s\n\n", strings.Join(detected, ", "))
	}

	b.WriteString("version: 0\ncapabilities:\n")
	if len(blocks) == 0 {
		b.WriteString("  # - name: test\n  #   run: \"<your test command>\"\n  #   verdict: exit\n")
	} else {
		for _, blk := range blocks {
			b.WriteString(blk)
		}
	}
	return b.String()
}

func printSummary(w io.Writer, res Result) {
	if w == nil {
		return
	}
	if len(res.Detected) == 0 {
		fmt.Fprintf(w, "Wrote %s — but ratchet recognized no ecosystem, so it proposed nothing.\n", "ratchet.yaml")
		fmt.Fprintln(w, "Edit it to declare what \"verified\" means here, then `ratchet check`.")
		return
	}
	fmt.Fprintf(w, "Wrote ratchet.yaml with proposals from: %s\n", strings.Join(res.Detected, ", "))
	fmt.Fprintln(w, "Every proposal is commented and inactive. Review, uncomment what's true,")
	fmt.Fprintln(w, "then `ratchet doctor` and `ratchet install-hooks`.")
}
