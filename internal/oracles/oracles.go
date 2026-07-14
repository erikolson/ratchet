// Package oracles implements `ratchet diff-oracles <base>`: compare the oracle
// hashes of the working-tree manifest against the manifest at a base ref, and
// report what changed (ADR-0008). The ratified reference is the protected branch;
// a weakening is a diff a human ratifies. It is informational with a signal exit
// code — tightening is silent, weakening and removal alarm — so an org can wire it
// as a required check or not, per policy.
package oracles

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/erikolson/ratchet/internal/gitx"
	"github.com/erikolson/ratchet/internal/manifest"
	"github.com/erikolson/ratchet/internal/verdict"
)

// ChangeKind classifies an oracle change. Added is tightening (silent); Changed
// and Removed weaken or alter the contract and require review.
type ChangeKind string

const (
	Added   ChangeKind = "added"
	Changed ChangeKind = "changed"
	Removed ChangeKind = "removed"
)

// Change is one capability's oracle change between base and working tree.
type Change struct {
	Capability string
	Kind       ChangeKind
	BaseRun    string
	NewRun     string
	BaseOracle string
	NewOracle  string
}

// Report is the diff outcome. ExitCode is 0 when clean or only tightening, 1 when
// any change or removal needs review, 3 when ratchet could not run.
type Report struct {
	Changes  []Change
	ExitCode int
}

type oracleInfo struct {
	run  string
	hash string
}

// Diff compares working-tree ratchet.yaml against ratchet.yaml at baseRef.
func Diff(root, baseRef string, w io.Writer) (Report, error) {
	baseData, err := gitx.ShowFile(root, baseRef, "ratchet.yaml")
	if err != nil {
		return Report{ExitCode: 3}, fmt.Errorf("reading ratchet.yaml at %s: %w", baseRef, err)
	}
	headData, err := os.ReadFile(filepath.Join(root, "ratchet.yaml"))
	if err != nil {
		return Report{ExitCode: 3}, fmt.Errorf("reading working-tree ratchet.yaml: %w", err)
	}

	base, err := oracleMap(baseData)
	if err != nil {
		return Report{ExitCode: 3}, fmt.Errorf("base manifest (%s): %w", baseRef, err)
	}
	head, err := oracleMap(headData)
	if err != nil {
		return Report{ExitCode: 3}, fmt.Errorf("working-tree manifest: %w", err)
	}

	var rep Report
	// Added / changed, in working-tree order.
	for _, name := range order(headData) {
		h := head[name]
		b, ok := base[name]
		switch {
		case !ok:
			rep.Changes = append(rep.Changes, Change{Capability: name, Kind: Added, NewRun: h.run, NewOracle: h.hash})
		case b.hash != h.hash:
			rep.Changes = append(rep.Changes, Change{Capability: name, Kind: Changed, BaseRun: b.run, NewRun: h.run, BaseOracle: b.hash, NewOracle: h.hash})
		}
	}
	// Removed, in base order.
	for _, name := range order(baseData) {
		if _, ok := head[name]; !ok {
			b := base[name]
			rep.Changes = append(rep.Changes, Change{Capability: name, Kind: Removed, BaseRun: b.run, BaseOracle: b.hash})
		}
	}

	rep.ExitCode = renderAndScore(w, baseRef, rep.Changes)
	return rep, nil
}

func oracleMap(data []byte) (map[string]oracleInfo, error) {
	m, err := manifest.Parse(data, "") // schema-only; probes/patches irrelevant to oracle identity
	if err != nil {
		return nil, err
	}
	out := map[string]oracleInfo{}
	for _, c := range m.Capabilities {
		h := verdict.OracleSpec{
			Adapter: "exit", Version: m.Version, Argv: c.Argv,
			Pass: c.Pass, Fail: c.Fail, Timeout: c.Timeout,
		}.Hash()
		out[c.Name] = oracleInfo{run: c.Run, hash: h}
	}
	return out, nil
}

// order returns capability names in declaration order for a manifest.
func order(data []byte) []string {
	m, err := manifest.Parse(data, "")
	if err != nil {
		return nil
	}
	names := make([]string, len(m.Capabilities))
	for i, c := range m.Capabilities {
		names[i] = c.Name
	}
	return names
}

func short(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

func renderAndScore(w io.Writer, baseRef string, changes []Change) int {
	if len(changes) == 0 {
		fmt.Fprintf(w, "✓ no oracle changes since %s.\n", baseRef)
		return 0
	}
	needsReview := false
	for _, c := range changes {
		switch c.Kind {
		case Added:
			fmt.Fprintf(w, "＋ oracle added: %s  (%s)  — tightening, no review needed.\n", c.Capability, c.NewRun)
		case Changed:
			needsReview = true
			fmt.Fprintf(w, "⚠ ORACLE CHANGED: %s\n    ratified: %-32s (oracle:%s)\n    proposed: %-32s (oracle:%s)\n  This changes what \"verified\" means. Requires review.\n",
				c.Capability, c.BaseRun, short(c.BaseOracle), c.NewRun, short(c.NewOracle))
		case Removed:
			needsReview = true
			fmt.Fprintf(w, "✗ ORACLE REMOVED: %s\n    was: %-32s (oracle:%s)\n  A removed capability stops verifying something. Requires review.\n",
				c.Capability, c.BaseRun, short(c.BaseOracle))
		}
	}
	if needsReview {
		return 1
	}
	return 0 // additions only — tightening is silent
}
