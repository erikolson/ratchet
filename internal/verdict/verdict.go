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

// Verdict is one normalized adjudication.
type Verdict struct {
	// Identity components (ADR-0001).
	Capability string `json:"capability"`
	Subject    string `json:"subject"`
	Oracle     string `json:"oracle"`
	Kind       Kind   `json:"kind"`

	// Outcome.
	Status Status `json:"status"`

	// Provenance — human-readable, never load-bearing.
	Head  string `json:"head,omitempty"`
	Dirty bool   `json:"dirty"`

	// Execution facts.
	DurationMs int64     `json:"duration_ms"`
	Findings   []Finding `json:"findings"`

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
