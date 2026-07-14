# ADR-0004 — Manifest shape and strict validation: a manifest cannot express a vacuous oracle

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §6
- **Origin:** grill Q7 (schema shape), Q8 (validation)

## What SEED said

`capabilities` as a YAML **map**; each with `run` and `verdict: exit`; a top-level
`version: 0`. No validation rules stated.

## What we now hold

### Shape

`capabilities` is an **ordered list**, not a map — if order matters, declare it
(no `yaml.Node` order-preservation gymnastics):

```yaml
version: 0
prepare: "npm ci"            # optional; doctor-only (ADR-0006)
capabilities:
  - { name: lint,      run: "ruff check", verdict: exit }
  - { name: typecheck, run: "mypy .",     verdict: exit }
  - { name: test,      run: "pytest -q",  verdict: exit }
probes:
  - { name: negate-threshold, patch: .ratchet/probes/negate-threshold.patch, flips: [test] }
```

- Capabilities run **serially, in declaration order** (author front-loads fast
  checks). *Reported* order = declaration order always, even if execution
  parallelizes later. Display order is contract; execution order is
  implementation.
- Optional per-capability `pass` / `fail` / `timeout` (ADR-0002); defaults
  `pass: [0]`, `fail: [1]`, a default `timeout`.

### The governing principle

**The manifest must not be able to express a vacuous oracle.** Not "should not" —
*cannot*. Every rule below is that one rule. If unsure whether to reject
something, ask: *could this produce a harness that is present and adjudicates
nothing?* If yes, reject at parse time.

### Validation (all failures → exit 3; a malformed manifest is not a verdict)

- **Strict decoding.** Unknown key → parse error. (A silently-dropped `flps:` is a
  witness that vanished.)
- **`version` required.** A newer version → refuse with "requires a newer
  ratchet." Never parse forward.
- **`verdict` required**, only legal v0 value is `exit`. `verdict: json/pytest` →
  "unknown verdict adapter." Close the door cleanly.
- **Name charset** `[a-z0-9][a-z0-9-]*`, max 64 — protects the identity encoding
  (ADR-0001) and avoids CLI-flag collisions.
- **Referential integrity, all hard errors:** duplicate capability names;
  duplicate probe names; `flips` naming a non-existent capability; a probe whose
  patch file is missing — **including under `check`** (deleting a patch is an
  attempted un-calibration = a loosening move, §7.1; loud everywhere).
- **`run` metacharacters** rejected (ADR-0003).
- **Empty manifest** (zero capabilities) → error ("verifies nothing"). `init` must
  never scaffold one; if it detects no capabilities it refuses and asks.
- **Vacuous probe:** non-empty patch with empty `flips` → error. (`mutation: none`
  is legal *only* as empty-patch + empty-flips — the ADR-0006 baseline.)
- **Vacuous exit sets:** `fail: []` → error; `pass: []` → error;
  `pass ∩ fail ≠ ∅` → error; `timeout` must be `> 0`.

## Why

Strictness is a *security property*, not a nicety: this tool exists to catch
cheats and mistakes, so a parser that silently tolerates a typo is self-defeating.
The list-not-map change makes execution order honest without fighting the YAML
parser.

## Consequences for v0 (steps 1–3)

- The parser + validator is the first thing built and is heavily test-first: every
  rejection rule above gets a red test.
- `probes[].flips` resolves against the capability list at parse time.
