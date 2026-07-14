# ADR-0007 — Each probe runs in its own fresh worktree

- **Status:** Accepted
- **Date:** 2026-07-14
- **Refines:** ADR-0006 (doctor)
- **Origin:** implementation of step 3; see FIELD_NOTES 2026-07-14 #3

## What ADR-0006 said

"Probes apply in a `git worktree`, never in my working tree." The naive reading is
one worktree for the whole doctor run: apply a probe, run, `git apply -R` to
revert, apply the next.

## What we now hold

**Each probe — and the baseline — runs in its own fresh worktree checked out from
HEAD.** No run inherits another run's working-tree state.

## Why

A shared, mutated-in-place worktree leaks **build caches keyed on file mtime**
across runs. Observed concretely (FIELD_NOTES #3): the baseline run compiled
`__pycache__/*.pyc` from the original source; `git apply` rewrote the source within
the same filesystem-mtime second; Python judged the cached bytecode current and ran
the **old** code. The mutation was on disk but the runner never saw it — a **false
negative in the verifier.** Any mtime-keyed cache (make, incremental compilers) can
do this.

Reverting with `git apply -R` does not help: it fixes the *source*, not the
*caches* the previous run left behind. Clearing caches would require per-language
knowledge (which files are cache), which the design forbids (SEED §8). A fresh
checkout is **the only language-agnostic guarantee that each run sees exactly its
own code.** Doctor is not the hot path (`check` is), so the cost is affordable.

## Cost — stated so nobody thinks we missed it

`prepare` now runs **once per probe**, not once per doctor run. For P probes doctor
creates P+1 worktrees and runs `prepare` P+1 times. With a slow `prepare`
(`npm ci`) and many probes this is real wall-clock time — acceptable because doctor
is run occasionally (the hook runs `check`, never `doctor`) and is explicitly
allowed to be slow.

## Deferred optimization (not built in v0)

**Prepare once into a template worktree, then copy it per probe.** This would run
the slow `prepare` a single time. It is **blocked by the same absolute-path problem
that killed symlinking in Q9/ADR-0006**: tools embed absolute paths in their caches
(`.mypy_cache`, `target/`, webpack), so a worktree copied to a different absolute
path can misbehave or write back through to the wrong location. Making copies
path-independent is the hard part and is out of scope for v0. This is the obvious
first optimization if `prepare` × N probes becomes real pain for an adopter.

## Consequences

- `gitx` gains `AddWorktree` / `RemoveWorktree` / `PruneWorktrees`.
- `doctor` runs baseline and each probe through one `runInWorktree` helper; the
  baseline worktree's subject tree IS the canonical `subject` at HEAD (a clean
  checkout, ratchet's own files excluded), which also fixed a latent bug where the
  possibly-dirty original working tree was used as the HEAD subject.
