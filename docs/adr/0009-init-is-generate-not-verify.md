# ADR-0009 — `init` is Generate, not Verify

- **Status:** Accepted
- **Date:** 2026-07-14
- **Relates:** SEED §11 step 4, §5, §9; ADR-0004 (Q8, empty manifest)
- **Origin:** init design fork

## The tension

`init` "proposes the manifest; the human ratifies." Proposing means detecting
`go test` / `npm test` / `pytest` — **ecosystem knowledge**, which §8 Positive
Indifference forbids. So may `init` peek at the ecosystem or not?

## What we hold

**`init` is Generate, not Verify — and that classification resolves the tension
cleanly; it is not a compromise.** The frame (SEED §4) puts Generate on the
advisory side by design. Heuristics in the *proposal* path are legitimate because
**a human ratifies**; heuristics in the *enforcement* path are forbidden because
**nothing ratifies a verdict**. Ecosystem detection is fine in `init` for exactly
the reason it is banned in `check`. Same layer boundary as ADR-0008, applied to a
different face.

### The shape: fork C — commented template, detected commands as labeled guesses

`init` writes a `ratchet.yaml` whose detected commands are **commented and
inactive** until the human uncomments them. Nothing ratchet guessed can run until
a human blesses it — so proposer ≠ ratifier holds, and Q8's "a manifest cannot
express a vacuous oracle" holds trivially (a commented command can't run).

### Two guardrails, or the detectors are a fleet liability

A hardcoded `go.mod → go test` table is right for five ecosystems and silently
wrong for the sixth (a Makefile build, a `just` recipe, a monorepo). A
plausible-looking proposal invites rubber-stamping — the §7.3 failure again: **a
confident wrong proposal is worse than an honest blank.** So:

1. **Every detected command is commented AND labeled as a guess**, never presented
   as fact: `# detected go.mod — is this how you verify here? uncomment to ratify`.
   The label is what forces a real ratification instead of a rubber-stamp.
2. **When `init` detects nothing, it refuses and asks** (Q8): writes the documented
   skeleton, does not invent. No silent empty manifest.

**The detectors earn their place only if they provoke ratification rather than
substitute for it.**

## Consequences

- `init` writes a commented, documented `ratchet.yaml`; a small detector table
  (`go.mod`, `package.json`, `pyproject.toml`/`setup.py`, `Cargo.toml`) produces
  commented, labeled suggestions.
- `init` never activates a capability and never writes an empty active manifest.
- This is the lowest-stakes command in v0: a wrong proposal is caught by
  ratification, not shipped as a false verdict, so it does not need the Q1–Q11
  grill the enforcement path got.
