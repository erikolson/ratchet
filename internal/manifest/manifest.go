// Package manifest parses and validates ratchet.yaml.
//
// The parser is a security boundary (ADR-0004): its governing rule is that a
// manifest must not be able to express a vacuous oracle — one that is present
// and adjudicates nothing. Every validation rule is that one rule. All failures
// are reported as *Error, which the CLI maps to exit code 3 (ratchet could not
// run); a malformed manifest is never a verdict about the code.
package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/erikolson/ratchet/internal/shlex"
	"gopkg.in/yaml.v3"
)

// SchemaVersion is the only manifest version this ratchet understands.
const SchemaVersion = 0

// DefaultTimeout applies to a capability that does not declare `timeout`.
const DefaultTimeout = 5 * time.Minute

// nameRe is the allowed charset for capability and probe names. Names flow into
// verdict identity (ADR-0001) and CLI arguments, so they are restricted.
var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const maxNameLen = 64

// Error is a manifest parse/validation failure. All such failures map to exit 3.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func errorf(format string, a ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, a...)}
}

// Manifest is a fully validated ratchet.yaml with defaults applied.
type Manifest struct {
	Version      int
	Prepare      string   // optional; doctor-only (ADR-0006). "" when absent.
	PrepareArgv  []string // tokenized Prepare; nil when absent.
	Capabilities []Capability
	Probes       []Probe
}

// Capability is one oracle producing one verdict.
type Capability struct {
	Name    string
	Run     string
	Argv    []string // tokenized Run (ADR-0003)
	Verdict string   // only "exit" in v0
	Pass    []int    // non-empty; default [0]
	Fail    []int    // non-empty; default [1]
	Timeout time.Duration
}

// Probe is a ratified mutation used by doctor to calibrate an oracle (ADR-0006).
type Probe struct {
	Name  string
	Patch string   // path relative to repo root; "" for the mutation:none baseline
	Flips []string // capability names asserted to flip to fail
}

// ---- raw decode types (strict) ----

type rawManifest struct {
	Version      *int            `yaml:"version"`
	Prepare      string          `yaml:"prepare"`
	Capabilities []rawCapability `yaml:"capabilities"`
	Probes       []rawProbe      `yaml:"probes"`
}

type rawCapability struct {
	Name    string `yaml:"name"`
	Run     string `yaml:"run"`
	Verdict string `yaml:"verdict"`
	Pass    *[]int `yaml:"pass"`
	Fail    *[]int `yaml:"fail"`
	Timeout string `yaml:"timeout"`
}

type rawProbe struct {
	Name  string   `yaml:"name"`
	Patch string   `yaml:"patch"`
	Flips []string `yaml:"flips"`
}

// Parse decodes and fully validates a manifest. repoRoot is used to check that
// probe patch files exist; pass "" to skip that filesystem check (schema-only).
func Parse(data []byte, repoRoot string) (*Manifest, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // strict: unknown key -> error
	var raw rawManifest
	if err := dec.Decode(&raw); err != nil {
		return nil, errorf("ratchet.yaml: %v", err)
	}

	if raw.Version == nil {
		return nil, errorf("ratchet.yaml: version is required")
	}
	if *raw.Version != SchemaVersion {
		if *raw.Version > SchemaVersion {
			return nil, errorf("ratchet.yaml: version %d requires a newer ratchet (this build understands version %d)", *raw.Version, SchemaVersion)
		}
		return nil, errorf("ratchet.yaml: unknown version %d", *raw.Version)
	}

	m := &Manifest{Version: *raw.Version, Prepare: raw.Prepare}

	if raw.Prepare != "" {
		argv, err := shlex.Split(raw.Prepare)
		if err != nil {
			return nil, errorf("prepare: %v", err)
		}
		if len(argv) == 0 {
			return nil, errorf("prepare is declared but empty")
		}
		m.PrepareArgv = argv
	}

	if len(raw.Capabilities) == 0 {
		return nil, errorf("ratchet.yaml declares no capabilities — a manifest with no capabilities verifies nothing")
	}

	seenCap := map[string]bool{}
	for _, rc := range raw.Capabilities {
		c, err := validateCapability(rc)
		if err != nil {
			return nil, err
		}
		if seenCap[c.Name] {
			return nil, errorf("duplicate capability name %q", c.Name)
		}
		seenCap[c.Name] = true
		m.Capabilities = append(m.Capabilities, c)
	}

	seenProbe := map[string]bool{}
	for _, rp := range raw.Probes {
		p, err := validateProbe(rp, seenCap, repoRoot)
		if err != nil {
			return nil, err
		}
		if seenProbe[p.Name] {
			return nil, errorf("duplicate probe name %q", p.Name)
		}
		seenProbe[p.Name] = true
		m.Probes = append(m.Probes, p)
	}

	return m, nil
}

func validateName(kind, name string) error {
	if name == "" {
		return errorf("%s name is required", kind)
	}
	if len(name) > maxNameLen {
		return errorf("%s name %q exceeds %d characters", kind, name, maxNameLen)
	}
	if !nameRe.MatchString(name) {
		return errorf("%s name %q is invalid: must match %s", kind, name, nameRe.String())
	}
	return nil
}

func validateCapability(rc rawCapability) (Capability, error) {
	if err := validateName("capability", rc.Name); err != nil {
		return Capability{}, err
	}

	if rc.Verdict == "" {
		return Capability{}, errorf("capability %q: verdict is required", rc.Name)
	}
	if rc.Verdict != "exit" {
		return Capability{}, errorf("capability %q: unknown verdict adapter %q (only \"exit\" is supported in v0)", rc.Name, rc.Verdict)
	}

	argv, err := shlex.Split(rc.Run)
	if err != nil {
		return Capability{}, errorf("capability %q: run: %v", rc.Name, err)
	}
	if len(argv) == 0 {
		return Capability{}, errorf("capability %q: run must not be empty", rc.Name)
	}

	pass := []int{0}
	if rc.Pass != nil {
		if len(*rc.Pass) == 0 {
			return Capability{}, errorf("capability %q: pass must be non-empty (a capability that can never pass is vacuous)", rc.Name)
		}
		pass = *rc.Pass
	}
	fail := []int{1}
	if rc.Fail != nil {
		if len(*rc.Fail) == 0 {
			return Capability{}, errorf("capability %q: fail must be non-empty (a capability that can never fail can never say the code is wrong)", rc.Name)
		}
		fail = *rc.Fail
	}
	if code, ok := overlap(pass, fail); ok {
		return Capability{}, errorf("capability %q: exit code %d is in both pass and fail (ambiguous oracle: pass and fail overlap)", rc.Name, code)
	}

	timeout := DefaultTimeout
	if rc.Timeout != "" {
		d, err := time.ParseDuration(rc.Timeout)
		if err != nil {
			return Capability{}, errorf("capability %q: invalid timeout %q: %v", rc.Name, rc.Timeout, err)
		}
		if d <= 0 {
			return Capability{}, errorf("capability %q: timeout must be greater than zero", rc.Name)
		}
		timeout = d
	}

	return Capability{
		Name:    rc.Name,
		Run:     rc.Run,
		Argv:    argv,
		Verdict: rc.Verdict,
		Pass:    pass,
		Fail:    fail,
		Timeout: timeout,
	}, nil
}

func validateProbe(rp rawProbe, caps map[string]bool, repoRoot string) (Probe, error) {
	if err := validateName("probe", rp.Name); err != nil {
		return Probe{}, err
	}

	// Vacuous-oracle rule, one level up (ADR-0006): a patch that flips nothing
	// can never fail; a flip asserted with no mutation has nothing to cause it.
	// Only the baseline (no patch, no flips) or a real probe (both) are legal.
	if rp.Patch == "" && len(rp.Flips) > 0 {
		return Probe{}, errorf("probe %q: declares flips but no patch (a flip with no mutation cannot be caused)", rp.Name)
	}
	if rp.Patch != "" && len(rp.Flips) == 0 {
		return Probe{}, errorf("probe %q: declares a patch but no flips (a mutation that flips nothing can never fail — vacuous probe)", rp.Name)
	}

	for _, f := range rp.Flips {
		if !caps[f] {
			return Probe{}, errorf("probe %q: flips names unknown capability %q", rp.Name, f)
		}
	}

	if rp.Patch != "" && repoRoot != "" {
		abs := filepath.Join(repoRoot, rp.Patch)
		if _, err := os.Stat(abs); err != nil {
			return Probe{}, errorf("probe %q: patch file %s is missing — removing a probe is a loosening move and is not allowed", rp.Name, rp.Patch)
		}
	}

	return Probe{Name: rp.Name, Patch: rp.Patch, Flips: rp.Flips}, nil
}

func overlap(a, b []int) (int, bool) {
	set := map[int]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, y := range b {
		if set[y] {
			return y, true
		}
	}
	return 0, false
}
