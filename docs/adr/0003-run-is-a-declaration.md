# ADR-0003 — `run` is a declaration, not a script

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §6, §8
- **Origin:** grill Q6

## What SEED said

Capabilities declare `run: "mypy ."`. SEED never said whether the string is handed
to a shell or split into arguments.

## What we now hold

`run` is a **declaration of one process**, not a shell script:

1. Accept the ergonomic string form.
2. Tokenize with **POSIX shell-word rules** (quoting/splitting, **no execution,
   no expansion** — `shlex`-style).
3. Exec the resulting **argv directly, with no shell**.
4. **Reject the manifest at parse time** if `run` contains a shell metacharacter:
   `&&  ||  |  ;  >  <  $  ` `` ` `` `  (  )  &  \n`.

```yaml
- { name: test, run: "pytest -q" }                 # ✓ → [pytest, -q]
- { name: lint, run: "ruff check 'src/my dir'" }   # ✓ quoting works
- { name: tc,   run: "mypy . && pytest" }          # ✗ rejected at parse time
```

Pipelines / globs / substitution go in a **version-controlled script**:
`run: "./scripts/verify.sh"` — one process, one argv, one verdict, and the shell
complexity lives in a reviewable file.

- **Environment:** inherited, unmodified (PATH, virtualenv, everything). Scrubbing
  *is* the Reproducibility substrate and is out of scope for v0. Known gap: the
  tree hash captures the code, not the environment.
- **Working directory:** always the tree root (repo root for `check`, worktree
  root for `doctor`). No per-capability `cwd` in v0 (adding one later is a
  backward-compatible field; unimplemented schema is a lie).

## Why

This is enforcement at the schema, not at the oracle:

- **`|| true`** — a vacuous oracle that always passes — becomes *unwritable*, not
  merely detectable-later.
- **`&&`** collapses two oracles into one bit, destroying per-capability verdict
  identity (ADR-0001). One capability = one oracle = one verdict. Composition
  belongs in the manifest (declare two capabilities), not in a shell string.
- **No shell ⇒ no POSIX dependency ⇒ OS-agnostic by construction**, satisfying §8
  with no caveat. *ratchet is OS-agnostic; a manifest may not be* — `pytest -q` is
  portable, `./scripts/check.sh` is the repo's choice, and that is the correct
  place for that decision to live.
- A missing binary is **ENOENT at spawn**, detected in ratchet's own code — a
  clean `error` (ADR-0002), not a shell exit-convention guess.

Rejected: `sh -c` (shell dependency, metacharacter surprises, fragile 127
pattern-matching). Rejected: `strings.Fields` tokenization (breaks on quoted
paths).

## Consequences for v0 (steps 1–3)

- Need a small POSIX shell-word tokenizer (~100 lines or one small dependency).
- Metacharacter rejection is part of manifest validation (ADR-0004) → exit 3.
- The tokenized argv feeds the `oracle` hash (ADR-0001).
