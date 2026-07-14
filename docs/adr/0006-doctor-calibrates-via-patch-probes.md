# ADR-0006 — Doctor calibrates the oracle via ratified patch probes

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §7.2; closes §10.2 open question
- **Origin:** grill Q3, Q9

## What SEED said

`doctor` "deliberately breaks the code, runs each declared capability, confirms
the verdict flips to fail, then restores." SEED did not say *how* a language-blind
tool (§8) breaks arbitrary code, nor where the break runs.

## What we now hold

**Ratchet does not generate mutations; it applies them.** Language knowledge lives
at the edge (the agent, which proposes) and enforcement lives at the center (the
flip invariant). Same division of labor as the whole product.

### Probes are patches

- A probe is a `git apply` patch — language-blind, precise, reviewable, diffable.
  `{ name, patch, flips }`. Patches live version-controlled in `.ratchet/probes/`.
- **Ratified, not agent-owned:** the agent proposes; the human ratifies (§7.1).
- Mutations must be **semantically valid but behaviorally wrong** — negate a
  return, flip a comparison, off-by-one a boundary. **Never a syntax error:**
  corrupting the source so the tool can't parse it produces `error`, not `fail`,
  and tests nothing (ADR-0002). This is mutation testing.

### `flips` is declared, never discovered

Declaring `flips` makes the probe an **assertion** (which can fail), not an
**observation** (which cannot). "Run everything and see what goes red" would
reopen the vacuous-oracle hole: a probe that flips nothing would be
indistinguishable from an oracle that detects nothing.

**Doctor's assertion, per probe** — run **all** capabilities against the mutated
tree:

- capabilities **in** `flips` → must be `fail` (**not** `error`);
- capabilities **not in** `flips` → must be `pass` (free check against sloppy /
  over-broad patches).

`mutation: none` is the baseline: empty patch + empty `flips`, must be `pass`
(ADR-0004). Same rule, no special case.

### Where it runs

- **Worktree created from `HEAD`.** Doctor's claim is about the *oracle*, not your
  uncommitted edits — editing `src/x` does not change whether the oracle can
  detect a broken `x`, so `HEAD` is the *right* subject, not an approximation. The
  receipt records `subject = HEAD tree`; doctor prints `calibrated at HEAD
  a3f9c21`. Edge case, documented: if your uncommitted edits are *to the test
  suite itself*, commit and re-run.
- **Manifest and probes are read from the working tree**, so a new probe can be
  validated before it is committed.
- **Worktree only — never the working tree.** Symlink/copy of gitignored deps is
  rejected: a different absolute path corrupts tool caches and can write *through*
  into the developer's real `.mypy_cache`/`target/`. Patch-in-place-then-revert is
  rejected: a crash mid-run corrupts the tree.

### Dependencies: optional `prepare`

- Top-level `prepare` (ADR-0004), **doctor-only**, argv rules of ADR-0003, runs
  **once** before the probe loop, generous timeout. `check` never invokes it.
- `prepare` failing → `error` (the harness couldn't be set up).
- `prepare` is included in a `calibration` verdict's `oracle` hash (ADR-0001).

### States and output

- `calibrated` · `uncalibrated` (a capability with no probe — **loud warning,
  exits 0**; the honest day-0 state, still enforced, just unverified) · `broken` (a
  probe failed to flip) · `error` (stale patch, `prepare` failure, or baseline
  `error`).
- **Stale patch** (`git apply --check` fails) → `error`, "probe stale, regenerate"
  — never pretends to have calibrated.
- **Legible failure:** on a baseline `error`, doctor names the likely cause
  (missing deps in a fresh worktree) and suggests a `prepare` step. **Doctor may
  guess at causes in prose; it never guesses at verdicts.**
- Proposed exit codes: `0` all-calibrated-or-only-uncalibrated; `1` ≥1 probe
  broken or errored; `3` ratchet-couldn't-run.
- Worktree in a temp dir, removed on exit **and crash**; stale ratchet worktrees
  pruned on startup.

### Evidence

`calibration` verdicts are written with `kind = calibration`. A probe's raw
pass/fail never escapes as a *code* verdict. Neither `calibration` nor `error` is
ever a witness for the code (ADR-0001, ADR-0002).

## Why

Two cheats this defends against, both invisible to every exit-code adapter:

- **Vacuous suite:** `go test ./...` with zero test files exits 0 — delete the
  tests, get green, no manifest edit needed. Doctor mutates the code; if the
  oracle still says `pass`, the oracle is empty.
- **Un-calibration:** removing a probe is a loosening move, gated everywhere
  (ADR-0004).

The probe is a witness for the *oracle* exactly as a verdict is a witness for the
*code* — the evidence axis applied one level up. Calibration coverage is a number
that goes up as you work: a real ratchet in v0 with no freeze/thaw machinery.

## Consequences for v0 (steps 1–3)

- `doctor` needs: worktree create/prune, `prepare` runner, per-probe apply +
  full-capability run + flip assertion, calibration verdict emission, the
  three-state output.
- Testing `doctor` (step 3) uses **hand-authored** probe fixtures; `init`'s probe
  *generation* is step 4, out of scope here.
