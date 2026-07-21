package ossification

import (
	"path/filepath"
	"testing"
)

func ratified(cap, base, new, req, rat string) Entry {
	return Entry{
		Type:       TypeOracleRatification,
		Capability: cap, BaseOracle: base, NewOracle: new,
		Requester: req, Ratifier: rat, Decision: Ratified,
		Timestamp: "2026-07-21T00:00:00Z",
	}
}

func TestAppendLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ratchet", "ossification.jsonl")
	in := []Entry{
		ratified("test", "aaa", "bbb", "agent", "alice"),
		{Capability: "lint", BaseOracle: "ccc", Requester: "bob", Ratifier: "alice", Decision: Rejected, Timestamp: "2026-07-21T01:00:00Z"},
	}
	for _, e := range in {
		if err := Append(path, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(got))
	}
	// type and format version are stamped on write.
	if got[0].Type != TypeOracleRatification || got[0].V != FormatVersion {
		t.Fatalf("entry not stamped: type=%q v=%d", got[0].Type, got[0].V)
	}
	if got[1].Decision != Rejected {
		t.Fatalf("second entry decision=%q, want rejected", got[1].Decision)
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("missing log should not error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing log should load nil, got %v", got)
	}
}

func TestRatifies_ClearsMatchingDifferentlyAuthored(t *testing.T) {
	entries := []Entry{ratified("test", "aaa", "bbb", "agent", "alice")}
	if _, ok := Ratifies(entries, "test", "aaa", "bbb"); !ok {
		t.Fatal("a ratified, differently-authored entry should clear the change")
	}
}

func TestRatifies_RejectsSelfRatification(t *testing.T) {
	entries := []Entry{ratified("test", "aaa", "bbb", "agent", "agent")}
	if _, ok := Ratifies(entries, "test", "aaa", "bbb"); ok {
		t.Fatal("ratifier == requester must NOT clear (proposer ≠ ratifier, SEED §5)")
	}
}

func TestRatifies_RejectedDoesNotClear(t *testing.T) {
	entries := []Entry{{
		Type:       TypeOracleRatification,
		Capability: "test", BaseOracle: "aaa", NewOracle: "bbb",
		Requester: "agent", Ratifier: "alice", Decision: Rejected,
	}}
	if _, ok := Ratifies(entries, "test", "aaa", "bbb"); ok {
		t.Fatal("a rejected entry must not clear the change")
	}
}

func TestRatifies_HashMismatchDoesNotClear(t *testing.T) {
	// A ratification of a *different* new oracle must not clear this one: the
	// weakening is content-addressed, so ratifying b→c does not bless b→d.
	entries := []Entry{ratified("test", "aaa", "bbb", "agent", "alice")}
	if _, ok := Ratifies(entries, "test", "aaa", "ddd"); ok {
		t.Fatal("a ratification of a different new-oracle must not clear")
	}
}

func TestRatifies_RemovalMatchesEmptyNewOracle(t *testing.T) {
	// A removed capability has NewOracle "".
	entries := []Entry{ratified("lint", "ccc", "", "agent", "alice")}
	if _, ok := Ratifies(entries, "lint", "ccc", ""); !ok {
		t.Fatal("a removal ratification (empty new oracle) should clear")
	}
}

func TestRatifies_MostRecentWins(t *testing.T) {
	// An earlier self-ratification followed by a valid one clears; order matters.
	entries := []Entry{
		ratified("test", "aaa", "bbb", "agent", "agent"), // invalid
		ratified("test", "aaa", "bbb", "agent", "alice"), // valid, later
	}
	if _, ok := Ratifies(entries, "test", "aaa", "bbb"); !ok {
		t.Fatal("a later valid ratification should supersede an earlier invalid one")
	}
}
