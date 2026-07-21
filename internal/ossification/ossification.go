// Package ossification implements the ossification log (ADR-0010): the committed,
// append-only, never-compacted record that turns a detected oracle weakening into
// an adjudicated one. It is the second log of SEED §7.4 — teeth, not exhaust — and
// unlike the verdict stream it lives in version control.
//
// v1 has exactly one entry type, oracle-ratification. Each entry names both the
// requester (who proposed the change) and the ratifier (who approved it), so
// proposer ≠ ratifier is a checkable property of the data. The stronger binding —
// that the two acts were authored by two different git identities — is an
// off-machine (CI) check on top of this, not something the local tool can enforce
// (ADR-0008); the log is the material that check folds over.
package ossification

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FormatVersion is stamped on every entry so a later evidence fold can migrate.
const FormatVersion = 1

// maxLine bounds a serialized entry so a single O_APPEND write is atomic
// (mirrors verdict.Append; ADR-0005).
const maxLine = 4096

// EntryType discriminates ossification-log entries. v1 has exactly one.
type EntryType string

const TypeOracleRatification EntryType = "oracle-ratification"

// Decision is the ratification outcome. A rejection is a first-class tooth — we
// considered loosening this and held the line (SEED §5).
type Decision string

const (
	Ratified Decision = "ratified"
	Rejected Decision = "rejected"
)

// Entry is one line of the ossification log: a ratified or rejected oracle change,
// content-addressed by the base→new oracle hashes it adjudicates (ADR-0001).
// A removal is NewOracle == "".
type Entry struct {
	Type       EntryType `json:"type"`
	Capability string    `json:"capability"`
	BaseOracle string    `json:"base_oracle"`
	NewOracle  string    `json:"new_oracle,omitempty"`
	Requester  string    `json:"requester"`
	Ratifier   string    `json:"ratifier"`
	Decision   Decision  `json:"decision"`
	Reason     string    `json:"reason,omitempty"`
	Timestamp  string    `json:"timestamp"`
	V          int       `json:"v"`
}

// Path is the committed ossification log's location for a repo root.
func Path(root string) string {
	return filepath.Join(root, ".ratchet", "ossification.jsonl")
}

// Marshal renders an entry as one JSON line, stamping type and format version.
func Marshal(e Entry) ([]byte, error) {
	if e.Type == "" {
		e.Type = TypeOracleRatification
	}
	if e.V == 0 {
		e.V = FormatVersion
	}
	return json.Marshal(e)
}

// Append writes e as one newline-terminated JSON line to the log at path, creating
// it (and its parent) if absent. The log is append-only; callers never rewrite it.
func Append(path string, e Entry) error {
	line, err := Marshal(e)
	if err != nil {
		return err
	}
	if len(line)+1 > maxLine {
		return fmt.Errorf("ossification line is %d bytes, exceeds %d (atomic-append guard)", len(line)+1, maxLine)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// Load reads every entry from path. A missing log is not an error — it means no
// ratification has ever been recorded (nil, nil).
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []Entry
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, maxLine), maxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("ossification log %s: %w", path, err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// Ratifies reports whether entries contain a valid ratification clearing the move
// base→new for capability: an oracle-ratification with decision=ratified, matching
// hashes, and ratifier ≠ requester (both non-empty). The most recent match wins, so
// a later re-ratification supersedes an earlier one. A rejected or self-ratified
// entry never clears. Returns the clearing entry for rendering.
func Ratifies(entries []Entry, capability, baseOracle, newOracle string) (Entry, bool) {
	var match Entry
	var found bool
	for _, e := range entries {
		if e.Type != TypeOracleRatification || e.Decision != Ratified {
			continue
		}
		if e.Capability != capability || e.BaseOracle != baseOracle || e.NewOracle != newOracle {
			continue
		}
		if e.Requester == "" || e.Ratifier == "" || e.Requester == e.Ratifier {
			continue // proposer ≠ ratifier is a schema invariant (SEED §5)
		}
		match, found = e, true
	}
	return match, found
}
