# ADR-0011 — The oracle pins declared repo artifacts, not the transitive closure

- **Status:** Proposed
- **Date:** 2026-07-21
- **Relates:** ADR-0001 (extends the oracle identity it defines), ADR-0008 (widens
  the content-addressable surface its layer-boundary note names), ADR-0010 (the
  sibling that deferred this; inherits its ratification gate), ADR-0006/0007 (doctor,
  complementary); SEED §5 (the Reproducibility substrate stays the ring beyond this)
- **Origin:** the `./verify.sh` gap named in ADR-0010

## The gap

The oracle hash covers the *command* — argv, pass/fail, timeout, version
(ADR-0001) — but **not the content the command executes.** When a repo's oracle is
`run: "./verify.sh"`, the hash is over `["./verify.sh"]`. Gut the assertions *inside*
`verify.sh` and the argv is byte-identical: the oracle hash does not move,
`diff-oracles` stays silent, and ADR-0010's ratification gate never fires. **The hash
pins the sentence, not the meaning.** That is field note #3 (the `.pyc` bug) wearing a
new hat: a weakening routed *through* what the command runs rather than through the
command itself.

## The boundary — the honest half of the problem

The naive fix ("hash everything the oracle depends on") is not shippable, for the
same reason ADR-0008 gives for path-deny: **the transitive closure is undecidable and
unbounded.** `pytest -q` runs hundreds of test files, `conftest.py`, `pyproject.toml`,
the interpreter, every installed dependency. "Everything it touches" is the
*environment*, and the environment is the Reproducibility substrate SEED §5 defers on
purpose. An ADR that pretended to pin it would be theater.

So the decision is a boundary, drawn where it is tractable:

> **The oracle pins *declared, in-tree* artifacts — content git already
> content-addresses, in the same worktree the `subject` hash covers — and nothing
> beyond the tree.**

Three concentric rings, stated so the scope is unmissable:

1. **argv + parameters** (ADR-0001) — *how the oracle is called.* Shipped.
2. **declared repo artifacts** (this ADR) — *the in-repo logic that call invokes.*
   In-tree, so git content-addresses it for free, exactly as `subject` does.
3. **the environment** (deferred) — interpreter, dependencies, OS, out-of-tree
   config. Out of the tree; needs the Reproducibility substrate, not this.

Ring 2 is tractable *precisely because it is in-tree.* This ADR ships ring 2 and does
not reach into ring 3.

## What we hold

### The mechanism folds into oracle identity — no new gate

A pinned artifact contributes `(path, content-hash)` to `OracleSpec` (ADR-0001), so
it flows into the existing oracle hash. **This adds no enforcement machinery:** change
a pinned file's content, the oracle hash changes, `diff-oracles` reports `changed`,
and ADR-0010's ratification applies unchanged. Content-pinning is not a new control —
it is an *extension of the content-addressable surface*, and everything downstream is
inherited. (This is why it is a sibling of ADR-0010, not a rewrite of it.)

### What gets pinned, by default and by declaration

- **argv[0] that resolves to a file inside the worktree is pinned automatically.** A
  repo-local script as the command *is* the oracle logic, authored beside the
  manifest; not pinning it recreates the gap by omission — and silent omission is the
  ADR-0009 failure. This activates nothing and guesses no command (the human already
  wrote `run: "./verify.sh"`); it only makes the oracle identity honest about content
  it already depends on. So it does not cross the Generate/Verify line.
- **Additional files pin only by explicit `pin: [...]`** — a fixture, a shared
  library the script sources. The parser cannot infer these, so the human declares
  them, ratified like any manifest content.
- **A pin may not reach outside the worktree.** An out-of-tree path is a manifest
  error (exit 3, ADR-0004): it is ring 3, and it is also a path-traversal boundary the
  manifest-as-security-boundary posture must hold.

### The false-confidence limit — the load-bearing caveat

**A pin is worth only as much of the oracle's decision as lives in the pinned file.**
Pinning `./verify.sh` when it is `#!/bin/sh\nexec pytest` pins a redirect and covers
nothing. Pinning `./gradlew test` pins a wrapper, not the build logic downstream of
it. A *partial* pin is dangerous exactly when it reads as total: "the oracle is
content-addressed" is false comfort if the deciding logic sits one ring out. So the
auto-pin of argv[0] is not a guarantee that the oracle is covered — it closes the
common case (a real repo-local verifier) and is explicitly inert for wrappers. The ADR
claims coverage of ring 2, never of what ring 2 shells out to.

This is why **`doctor` (ADR-0006) remains necessary and complementary.** Pinning makes
a weakening *visible in the diff*, at check/CI time. Doctor makes vacuousness *visible
in calibration*, by observing the oracle actually flip. Neither subsumes the other:
pinning cannot tell you the pinned script still adjudicates anything (doctor's job),
and doctor does not run in the `diff-oracles` path (pinning's job).

### The friction wrinkle — and why the cost is near-zero

Unlike a `pass:`-set change (where `added` reads as tightening and stays silent,
ADR-0008), a content-hash change is **opaque: we cannot tell a strengthening from a
weakening from the hash alone.** So *every* edit to a pinned verifier surfaces as
`changed` and wants ratification — heavier than the command case. We accept this,
fail-closed (ADR-0002): a script edit *can* be a weakening and the hash cannot rule it
out, so treating every edit as review-worthy is the honest posture. The cost is low in
practice — if you are editing your verifier, a human is already reviewing that diff in
the PR; ratification does not add a review, it **records the yes that review already
represents**, turning it into a tooth (the ADR-0010 / field-note-#1 move again).

## Scope

**In scope:** a `pin:` field; auto-pin of an in-tree argv[0]; `(path, content-hash)`
folded into `OracleSpec`; refusal of out-of-tree pins. **Deferred, named:** pinning
the transitive closure or the environment (ring 3 — the Reproducibility substrate);
classifying a content change as tighten-vs-weaken (opaque by construction, so all
content changes review). Rides ADR-0010's `diff-oracles` gate and `ratify`; adds no
command of its own.

## Consequences for the build

- `OracleSpec` (ADR-0001) gains a sorted list of `(path, content-hash)` pairs in its
  canonical encoding; the oracle hash changes when pinned content changes.
- The manifest parser resolves argv[0] against the worktree and auto-pins it when it
  is an in-tree file; parses `pin: [...]`; rejects out-of-tree paths as `*Error`
  (exit 3).
- `check`, `gate`, and `doctor` read pinned content from the same `subject` tree they
  already hash — no new fixture, no new I/O boundary.
- `init` may suggest pinning a detected repo-local script, commented and labeled
  (ADR-0009); it never activates a pin.
- `diff-oracles` and `ratify` are unchanged: a pinned-content weakening is just
  another `changed` oracle, adjudicated by ADR-0010.
