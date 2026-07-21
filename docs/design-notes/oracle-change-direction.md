# Design note — oracle change direction

- **Status:** Open exploration (not a decision). The decision in force is
  [ADR-0011](../adr/0011-oracle-pins-declared-repo-artifacts.md)'s fail-closed posture;
  this note records the reasoning behind it and what a more permissive future would cost.
- **Relates:** ADR-0008 (added/changed/removed classification), ADR-0011 (the opaque-hash
  friction wrinkle), the deferred structured verdict adapters (`json/pytest`).

## The problem

The ratchet's organizing asymmetry is *tightening is free, loosening is gated* (SEED §5).
To apply it, ratchet must decide, for a given manifest change, **which direction it goes.**
Today that decision is made structurally, in three buckets (`internal/oracles/oracles.go`):

- **Added** a capability → tightening → **silent** (exit 0).
- **Removed** a capability → weakening → **alarms** (needs ratification).
- **Changed** an oracle (its command, pass/fail set, or timeout) → **alarms** — with no
  attempt to judge whether it got stricter or looser.

Add and remove are sound because they are *monotonic regardless of semantics*: adding an
oracle can only add constraint, removing one can only subtract it. The interesting case is
**Changed**, and the deliberate choice there is: **do not infer direction — fail closed.**

## Why a changed oracle's direction is opaque

Under the v0 exit adapter an oracle is "run this argv, check the exit code against a set."
The values that carry the standard live *inside* the command string, whose meaning ratchet
never sees. So the direction of a change is not recoverable from what ratchet holds:

- `--cov-fail-under=80 → 90` — **bigger is harder.**
- `timeout: 60s → 10s` — **smaller is harder** (a tighter budget).
- `pass: [0] → [0,1]` — accepting more exit codes is **easier.**

There is no rule "bigger is stricter": sometimes smaller is stricter. A content hash of the
command is therefore **opaque** — strengthen and weaken are indistinguishable from it. The
safe response is to treat every modification as review-worthy and let the **human ratifier
supply the direction judgment**; that judgment is what becomes the tooth (ADR-0010). Guessing
wrong in the *loosening* direction is precisely the failure ratchet exists to prevent, so a
guess is worse than a review.

This is the honest floor: it is the most the asymmetry can say soundly without semantic
insight into the values, and it fail-closes in the safe direction.

## Earning the right to infer direction

Being *more permissive* — silently allowing a provable tightening of a modified oracle —
is something ratchet would have to **earn**, not assume. Two layers would be required:

1. **Structured thresholds.** A structured adapter (the deferred `json/*` line) lets the
   manifest express a standard as a *field* instead of burying it in a command string —
   e.g. `min_coverage: 80`. Now `80 → 90` and `80 → 70` are comparable values, not opaque
   argv.
2. **Declared per-field monotonicity.** Comparability is not enough — ratchet still must
   know *which way is stricter for this field*, and that cannot be assumed (the bigger-vs-
   smaller trap above). Each structured field would **declare its direction**: "higher is
   stricter" for coverage, "lower is stricter" for timeout. Only a *declared* monotonicity,
   never a guess, licenses a silent-on-tightening decision.

The rare case where smaller is harder is not an edge case to paper over — it is the reason
inference must be declaration-driven.

## Open questions

- **Pass-set direction is already structurally detectable.** `pass: [0] → [0,1]` is a
  superset (strictly weaker); `[0,1] → [0]` a subset (stricter). Even under the exit adapter,
  this one dimension carries a computable direction. Worth exploiting now — classify a
  pass-set *widening* as a distinct, louder signal — or does special-casing one field muddy
  the clean "any change reviews" rule enough to not be worth it before structured adapters
  exist? Leaning: leave it uniform until the structured layer lands, so there is one rule,
  not a patchwork.
- **Where does the direction declaration live** — inline on the capability, or in the
  adapter definition? A per-field property feels like adapter schema, not manifest data.
- **How does an inferred tightening interact with `diff-oracles` scoring?** A silent-on-
  tightening path must still leave the oracle-hash trace (identity changed), so the change is
  never *invisible* — only *unalarmed*. That boundary must hold: silent ≠ untraceable.
- **Does doctor change the calculus?** A tightening a human can't eyeball is exactly what
  `doctor` could confirm (the stricter oracle still flips on the ratified probe). Inference +
  calibration together might license more than inference alone.

## Non-goals for now

No syntax, no adapter, no scoring change is proposed. This note exists so the reasoning
survives; the decision to build any of it waits for the structured-adapter work, and would
land as an ADR that supersedes ADR-0011's "all content changes review" line for structured
fields only.
