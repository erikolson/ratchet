// Package ratify implements `ratchet ratify`: record a differently-authored
// ratification of an oracle change into the committed ossification log (ADR-0010).
// It is the act that turns a detected weakening into an adjudicated one — the
// human's "yes" (or "no") made durable, content-addressed to the exact base→new
// oracle move it blesses, so `diff-oracles` can clear only that move.
package ratify

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/erikolson/ratchet/internal/gitx"
	"github.com/erikolson/ratchet/internal/manifest"
	"github.com/erikolson/ratchet/internal/ossification"
	"github.com/erikolson/ratchet/internal/verdict"
)

// Options configures a ratification.
type Options struct {
	RepoRoot   string
	Capability string
	BaseRef    string // the ratified reference (protected branch), e.g. "origin/main"
	Requester  string // who proposed the change (required)
	Ratifier   string // who approves; defaults to git user.email
	Reject     bool   // record a rejection (a tooth) instead of an approval
	Reason     string // optional free text
	Now        string // RFC3339 UTC timestamp (injected for testability)
	Stdout     io.Writer
}

// Result reports the entry written.
type Result struct {
	Entry ossification.Entry
}

// Run resolves the base→new oracle move for the capability, refuses anything that
// is not an actual weakening awaiting adjudication, and appends the ratification.
func Run(opts Options) (Result, error) {
	if opts.Capability == "" {
		return Result{}, fmt.Errorf("a capability to ratify is required")
	}
	if opts.Requester == "" {
		return Result{}, fmt.Errorf("--requester is required: name who proposed this change (proposer ≠ ratifier, SEED §5)")
	}
	ratifier := opts.Ratifier
	if ratifier == "" {
		ratifier = gitx.UserEmail(opts.RepoRoot)
	}
	if ratifier == "" {
		return Result{}, fmt.Errorf("no ratifier: pass --ratifier or set git user.email")
	}
	decision := ossification.Ratified
	if opts.Reject {
		decision = ossification.Rejected
	}
	// A ratification whose two authors coincide is not a decision — refuse to write
	// a known-invalid tooth. A rejection by the proposer is legitimate (you may
	// withdraw your own proposal), so this guard applies only to approvals.
	if decision == ossification.Ratified && opts.Requester == ratifier {
		return Result{}, fmt.Errorf("ratifier (%s) must differ from requester (%s): proposer ≠ ratifier (SEED §5)", ratifier, opts.Requester)
	}

	baseHash, baseHave, err := oracleHashAt(opts.RepoRoot, opts.BaseRef, opts.Capability)
	if err != nil {
		return Result{}, err
	}
	newHash, newHave, err := oracleHashWorking(opts.RepoRoot, opts.Capability)
	if err != nil {
		return Result{}, err
	}

	switch {
	case !baseHave && !newHave:
		return Result{}, fmt.Errorf("unknown capability %q: not in the manifest at %s nor the working tree", opts.Capability, opts.BaseRef)
	case !baseHave && newHave:
		return Result{}, fmt.Errorf("nothing to ratify: %q is an addition — tightening is silent and needs no ratification (ADR-0008)", opts.Capability)
	case baseHave && newHave && baseHash == newHash:
		return Result{}, fmt.Errorf("nothing to ratify: %q's oracle is unchanged vs %s", opts.Capability, opts.BaseRef)
	}
	// baseHave && (a changed oracle, newHave=true) OR (a removal, newHave=false → newHash "").

	entry := ossification.Entry{
		Type:       ossification.TypeOracleRatification,
		Capability: opts.Capability,
		BaseOracle: baseHash,
		NewOracle:  newHash, // "" for a removal
		Requester:  opts.Requester,
		Ratifier:   ratifier,
		Decision:   decision,
		Reason:     opts.Reason,
		Timestamp:  opts.Now,
	}
	if err := ossification.Append(ossification.Path(opts.RepoRoot), entry); err != nil {
		return Result{}, err
	}
	printSummary(opts.Stdout, opts.BaseRef, entry)
	return Result{Entry: entry}, nil
}

// oracleHashWorking returns the oracle hash of capability in the working-tree
// manifest, and whether the capability is present.
func oracleHashWorking(root, capability string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, "ratchet.yaml"))
	if err != nil {
		return "", false, fmt.Errorf("reading working-tree ratchet.yaml: %w", err)
	}
	return oracleHashIn(data, capability, "working-tree manifest")
}

// oracleHashAt returns the oracle hash of capability in the manifest at ref.
func oracleHashAt(root, ref, capability string) (string, bool, error) {
	data, err := gitx.ShowFile(root, ref, "ratchet.yaml")
	if err != nil {
		return "", false, fmt.Errorf("reading ratchet.yaml at %s: %w", ref, err)
	}
	return oracleHashIn(data, capability, fmt.Sprintf("manifest at %s", ref))
}

func oracleHashIn(data []byte, capability, where string) (string, bool, error) {
	m, err := manifest.Parse(data, "")
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", where, err)
	}
	for _, c := range m.Capabilities {
		if c.Name == capability {
			h := verdict.OracleSpec{
				Adapter: "exit", Version: m.Version, Argv: c.Argv,
				Pass: c.Pass, Fail: c.Fail, Timeout: c.Timeout,
			}.Hash()
			return h, true, nil
		}
	}
	return "", false, nil
}

func printSummary(w io.Writer, baseRef string, e ossification.Entry) {
	if w == nil {
		return
	}
	verb := "ratified"
	if e.Decision == ossification.Rejected {
		verb = "REJECTED"
	}
	fmt.Fprintf(w, "✓ wrote ossification entry: %s %s (vs %s)\n", verb, e.Capability, baseRef)
	fmt.Fprintf(w, "  requester: %s   ratifier: %s\n", e.Requester, e.Ratifier)
	fmt.Fprintln(w, "  Commit .ratchet/ossification.jsonl — it is a tooth, tracked in git (ADR-0010).")
	fmt.Fprintln(w, "  Locally this is advisory; the guarantee is CI binding the two acts to")
	fmt.Fprintln(w, "  distinct git authors on the protected branch (ADR-0008).")
}
