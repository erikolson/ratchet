# ADR-0002 — Three-state status, fail-closed, exit-code scheme

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §7.3
- **Origin:** grill Q2, Q7

## What SEED said

Binary green/red, pass/FAIL. §7.3 spoke only of two states.

## What we now hold

**`status ∈ {pass, fail, error}`.** `fail` is a claim about the *code*; `error` is
a claim about the *harness*. Collapsing them destroys the distinction §7.3 exists
to protect.

**Classification (exit adapter).** Declared per capability with defaults:

- `pass` codes (default `[0]`), `fail` codes (default `[1]`).
- Every other exit code → `error`. **Not "nonzero = fail."** (pytest exit 5 =
  "no tests collected" is a harness failure, not a verdict on the code.)
- Always `error` regardless of manifest, detected in ratchet's own code:
  spawn failure / ENOENT (missing binary), 126 (not executable), death by signal,
  timeout exceeded. With argv exec (ADR-0003) the missing-binary case is ENOENT at
  `Start()`, not a shell's 127 — the 127/126 rules remain as belt-and-braces for a
  *tool* that returns them.

**Fail closed.** `error` blocks, per §7.3 — a harness that isn't running is not a
control. But it is a *different colour of red*: different message, different
remedy, different process exit code.

**`ratchet check` aggregate process exit code:**

| code | meaning |
|---|---|
| `0` | all `pass` |
| `1` | ≥1 `fail`, 0 `error` — your code is broken |
| `2` | ≥1 `error` — your harness is broken |
| `3` | ratchet could not run — no/malformed manifest, not a git repo, bad flag |

Precedence **`error > fail > pass`**: an untrustworthy harness makes the whole run
untrustworthy. Exit `3` exists to keep `1` clean — override Cobra's default of
exit 1 for usage errors, because a usage error is not a verdict.

**The hook rule is `nonzero → deny`, never a code enumeration.** Fail closed by
construction: a future exit code can never accidentally fail open.

**Evidence rule.** An `error` verdict is recorded in the stream but is **never a
witness** — no adjudication occurred, so it is not evidence about the code.

## Why

The missing-tool / vacuous-run cases are indistinguishable from a real failure
under "nonzero = fail," and §7.3's whole argument is that an unobservable or
mis-attributed control is no control. Three states keep "your code is broken" and
"your harness is broken" as separate, separately-actionable facts.

## Consequences for v0 (steps 1–3)

- `check` maps each capability's outcome to `pass`/`fail`/`error`, then folds to
  the aggregate exit code above.
- `doctor` reuses the same three-state classifier; its flip assertion (ADR-0006)
  is defined in terms of `fail` vs `error` specifically.
- Exit `3` is emitted for all manifest-validation failures (ADR-0004).
