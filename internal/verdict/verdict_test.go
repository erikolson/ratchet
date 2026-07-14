package verdict

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func baseSpec() OracleSpec {
	return OracleSpec{
		Adapter: "exit",
		Version: 0,
		Argv:    []string{"pytest", "-q"},
		Pass:    []int{0},
		Fail:    []int{1},
		Timeout: 5 * time.Minute,
	}
}

func TestOracleHash_DeterministicAndHex(t *testing.T) {
	h1 := baseSpec().Hash()
	h2 := baseSpec().Hash()
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64 (hex sha256)", len(h1))
	}
	for _, r := range h1 {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("hash contains non-hex rune %q", r)
		}
	}
}

func TestOracleHash_PassFailAreSets(t *testing.T) {
	a := baseSpec()
	a.Pass = []int{0, 1}
	a.Fail = []int{2, 3}
	b := baseSpec()
	b.Pass = []int{1, 0} // reordered
	b.Fail = []int{3, 2} // reordered
	if a.Hash() != b.Hash() {
		t.Fatal("pass/fail are code sets: reordering must not change the oracle")
	}
}

func TestOracleHash_ArgvOrderIsSignificant(t *testing.T) {
	a := baseSpec()
	a.Argv = []string{"a", "b"}
	b := baseSpec()
	b.Argv = []string{"b", "a"}
	if a.Hash() == b.Hash() {
		t.Fatal("argv order defines the process and must change the oracle")
	}
}

func TestOracleHash_ArgvBoundariesUnambiguous(t *testing.T) {
	a := baseSpec()
	a.Argv = []string{"a", "b"}
	b := baseSpec()
	b.Argv = []string{"ab"}
	if a.Hash() == b.Hash() {
		t.Fatal(`["a","b"] must not collide with ["ab"] (canonical encoding must be length-delimited)`)
	}
}

func TestOracleHash_EachFieldMatters(t *testing.T) {
	base := baseSpec().Hash()
	mut := []struct {
		name string
		f    func(s *OracleSpec)
	}{
		{"adapter", func(s *OracleSpec) { s.Adapter = "other" }},
		{"version", func(s *OracleSpec) { s.Version = 1 }},
		{"pass", func(s *OracleSpec) { s.Pass = []int{0, 5} }},
		{"fail", func(s *OracleSpec) { s.Fail = []int{7} }},
		{"timeout", func(s *OracleSpec) { s.Timeout = time.Minute }},
	}
	for _, tc := range mut {
		s := baseSpec()
		tc.f(&s)
		if s.Hash() == base {
			t.Fatalf("changing %s did not change the oracle hash", tc.name)
		}
	}
}

func TestOracleHash_PrepareOnlyWhenIncluded(t *testing.T) {
	// prepare is ignored for check verdicts...
	a := baseSpec()
	a.IncludePrepare = false
	a.Prepare = []string{"npm", "ci"}
	if a.Hash() != baseSpec().Hash() {
		t.Fatal("prepare must not affect the oracle when IncludePrepare is false (check)")
	}

	// ...but participates for calibration.
	c1 := baseSpec()
	c1.IncludePrepare = true
	c1.Prepare = []string{"npm", "ci"}
	c2 := baseSpec()
	c2.IncludePrepare = true
	c2.Prepare = []string{"uv", "sync"}
	if c1.Hash() == c2.Hash() {
		t.Fatal("different prepare must yield a different calibration oracle")
	}

	// present-but-empty differs from absent.
	empty := baseSpec()
	empty.IncludePrepare = true
	empty.Prepare = nil
	if empty.Hash() == baseSpec().Hash() {
		t.Fatal("prepare present-but-empty must differ from prepare absent")
	}
}

func TestIdentity_EncodesAllFourComponents(t *testing.T) {
	v := Verdict{Capability: "test", Subject: "abc123", Oracle: "def456", Kind: KindCheck}
	if got, want := v.Identity(), "test@check@tree:abc123@oracle:def456"; got != want {
		t.Fatalf("Identity() = %q, want %q", got, want)
	}
	// kind is part of identity: same code+oracle, different kind => different identity.
	cal := v
	cal.Kind = KindCalibration
	if v.Identity() == cal.Identity() {
		t.Fatal("kind must be part of verdict identity (ADR-0001)")
	}
}

func TestVerdict_FindingsAlwaysMarshalsAsArray(t *testing.T) {
	v := Verdict{Capability: "test", Kind: KindCheck, Status: StatusPass} // Findings nil
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"findings":[]`) {
		t.Fatalf("findings must marshal as [] not null; got %s", data)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("verdict line is not valid JSON: %v", err)
	}
	if back["v"] == nil {
		t.Fatal("verdict must carry a format-version field 'v'")
	}
}

func TestAppend_WritesJSONLLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verdicts.jsonl")
	v1 := Verdict{Capability: "test", Subject: "a", Oracle: "o", Kind: KindCheck, Status: StatusPass}
	v2 := Verdict{Capability: "lint", Subject: "a", Oracle: "p", Kind: KindCheck, Status: StatusFail}
	for _, v := range []Verdict{v1, v2} {
		if err := Append(path, v); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), raw)
	}
	for i, line := range lines {
		if len(line) >= 4096 {
			t.Fatalf("line %d is %d bytes, must stay under 4096 for atomic O_APPEND", i, len(line))
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
	}
}
