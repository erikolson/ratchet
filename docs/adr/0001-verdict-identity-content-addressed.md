# ADR-0001 — Verdict identity is content-addressed over `(capability, subject, oracle, kind)`

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §6 (the verdict); closes §10.2 open question
- **Origin:** grill Q1, Q10

## What SEED said

A verdict is `capability + fixture + commit`, "stably identified." The §6 example
carried `commit: "a3f9c21"` and `fixture: "working-tree"` together.

## What we now hold

Verdict **identity = `(capability, subject, oracle, kind)`**. All four are
content-addressed. `commit` and `fixture` are retired.

- **`subject`** — the git tree hash of *the code under judgment*. Computed via a
  temporary index (`GIT_INDEX_FILE`, `git add -A`, `git write-tree`) so that
  untracked-but-not-ignored files are included and a dirty tree gets a distinct
  hash that may never be committed. **Excludes exactly `{ratchet.yaml,
  .ratchet/}`** — the files ratchet owns — and (via gitignore) `verdicts.jsonl`.
  Excluding ratchet's own bookkeeping is not language knowledge (which §8
  forbids); it is ratchet knowing its own files.
- **`oracle`** — a content hash of the capability's *fully resolved definition*:
  the tokenized argv (post shell-word split, ADR-0003), the adapter (`exit`), the
  `pass` and `fail` code sets, the `timeout`, and the manifest `version`.
  Per-capability, not per-manifest. For a `calibration` verdict the oracle hash
  **also** includes `prepare` (ADR-0006); for a `check` verdict it does not,
  because `prepare` never runs for `check`.
- **`kind`** ∈ `{check, calibration, gate}` — the subject of judgment. `check`
  judges the code; `calibration` judges the oracle; `gate` is reserved for the
  step-5 hook and is not written in v0.
- **`head` + `dirty`** are retained as **provenance only**. Human-readable, never
  load-bearing. **Nothing may key off them.**

Identity is a function of the **declared contract**, not the **implementation**: a
ratchet *binary* upgrade changes no identity and preserves accumulated evidence; a
*schema* change that alters semantics must bump the manifest `version`, which is in
the oracle hash.

## Why

Location-addressing collides. Two problems, the same bug on two axes:

1. **Code axis (Q1).** `fixture: working-tree` + `commit: HEAD` gives two
   contradictory runs (before/after an uncommitted edit) the *same* identity,
   while `commit` describes what was last committed, not what ran.
2. **Oracle axis (Q10).** Excluding the manifest from `subject` (correct) means
   weakening a capability's command (`pytest -q` → `pytest -q --ignore=tests/`)
   leaves `subject` unchanged — same identity, different oracle, different meaning.

Content-addressing *both* inputs closes both. A weakened oracle produces a new
identity; the old verdict stays honest evidence about the old oracle.

## Consequences for v0 (steps 1–3)

- `check` and `doctor` both compute `subject` and `oracle` before emitting a
  verdict. The `oracle` hash needs a pinned canonical byte encoding (tested).
- The identity encoding must round-trip the four components. The name charset
  restriction (ADR-0004) exists partly to protect this encoding.
- `subject` excludes `{ratchet.yaml, .ratchet/}`, so editing a probe patch or the
  manifest never fragments a code verdict's identity.
