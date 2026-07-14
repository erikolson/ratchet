// Package verdict defines the normalized verdict, its content-addressed identity
// (ADR-0001), and the append-only JSONL log (ADR-0005).
//
// A verdict's identity is (capability, subject, oracle, kind). `head` and
// `dirty` are provenance only and must never be keyed on. `findings` is retained
// in the schema but is always empty under the v0 exit adapter — a finding is a
// structured parsed claim, which no exit-code adapter can produce.
package verdict

import (
	"encoding/json"
	"fmt"
)

// FormatVersion is stamped on every log line so a later evidence fold can migrate.
const FormatVersion = 1

// Status is the outcome. `fail` is a claim about the code; `error` is a claim
// about the harness (ADR-0002).
type Status string

const (
	StatusPass  Status = "pass"
	StatusFail  Status = "fail"
	StatusError Status = "error"
)

// Kind is the subject of judgment (ADR-0001). `gate` is reserved for the step-5
// hook and is not written in v0.
type Kind string

const (
	KindCheck       Kind = "check"
	KindCalibration Kind = "calibration"
	KindGate        Kind = "gate"
)

// Finding is a structured, parsed claim. Reserved; never populated by the exit
// adapter (ADR-0005). Kept so the schema does not migrate when json/* adapters land.
type Finding struct {
	Message string `json:"message,omitempty"`
}

// ProbeRecord is one probe nested inside a calibration verdict. A calibration
// verdict is one claim ("this oracle is calibrated") with the probes as its
// evidence, so the probes are wrapped here and never escape as top-level verdicts
// (Q2 invariant). MutatedSubject is provenance — the tree that actually ran under
// the probe — and is never keyed on (identity stays subject=HEAD).
type ProbeRecord struct {
	Name           string `json:"name"`
	Expected       string `json:"expected"` // "pass" | "fail"
	Observed       Status `json:"observed"`
	MutatedSubject string `json:"mutated_subject"`
}

// Verdict is one normalized adjudication.
type Verdict struct {
	// Identity components (ADR-0001). Capability and Oracle are absent on a gate
	// verdict, which is a decision about a whole check run, not one capability.
	Capability string `json:"capability,omitempty"`
	Subject    string `json:"subject"`
	Oracle     string `json:"oracle,omitempty"`
	Kind       Kind   `json:"kind"`

	// Outcome (check/calibration). Absent on a gate verdict.
	Status Status `json:"status,omitempty"`

	// Gate decision (kind=gate only). A gate verdict references the check
	// verdicts it acted on by identity (ADR-0005 / Q4); it never re-embeds them.
	Decision string   `json:"decision,omitempty"` // "block" | "allow"
	Action   string   `json:"action,omitempty"`   // what was gated, e.g. "git commit"
	Refs     []string `json:"refs,omitempty"`     // referenced verdict identities

	// Provenance — human-readable, never load-bearing.
	Head  string `json:"head,omitempty"`
	Dirty bool   `json:"dirty"`

	// Execution facts.
	DurationMs int64     `json:"duration_ms"`
	Findings   []Finding `json:"findings"`

	// Probes is populated only for calibration verdicts (kind=calibration): the
	// nested evidence that established the oracle's calibration. Absent for check.
	Probes []ProbeRecord `json:"probes,omitempty"`

	// Metadata.
	Timestamp string `json:"timestamp"`
	V         int    `json:"v"`
}

// Identity returns the stable content-addressed key. Capability names are charset
// restricted and kind is a fixed enum, so no component can contain the `@`/`:`
// delimiters — the encoding is unambiguous.
func (v Verdict) Identity() string {
	return fmt.Sprintf("%s@%s@tree:%s@oracle:%s", v.Capability, v.Kind, v.Subject, v.Oracle)
}

// GateVerdict builds a kind=gate verdict recording a block/allow decision over a
// check run, referencing that run's verdicts by identity (ADR-0005 / Q4).
func GateVerdict(subject, head string, dirty bool, decision, action string, refs []string, durationMs int64, timestamp string) Verdict {
	return Verdict{
		Subject:    subject,
		Kind:       KindGate,
		Decision:   decision,
		Action:     action,
		Refs:       refs,
		Head:       head,
		Dirty:      dirty,
		DurationMs: durationMs,
		Timestamp:  timestamp,
	}
}

// Marshal renders a verdict as a single-line JSON object, normalizing nil
// findings to `[]` and stamping the format version.
func Marshal(v Verdict) ([]byte, error) {
	if v.Findings == nil {
		v.Findings = []Finding{}
	}
	if v.V == 0 {
		v.V = FormatVersion
	}
	return json.Marshal(v)
}
