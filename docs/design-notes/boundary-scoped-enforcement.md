# Design note — boundary-scoped enforcement

- **Status:** Open exploration (not a decision).
- **Relates:** SEED §1a (ratchet as platform infrastructure the harness composes),
  SEED §4 (Verify/Feedback and bindingness), ADR-0001 (verdict identity),
  ADR-0005 (receipts), ADR-0008 (local hooks are feedback; prevention lives
  off-machine), ADR-0010 (weakening is witnessed).

## The insight

Enforcement is useful at the boundary where a thing tries to cross from one state
or trust zone into another. The oracle must be available relative to that boundary,
and the boundary must be able to refuse passage.

That explains why ratchet is currently strongest at repo/git boundaries:

- The manifest is discovered at the repo root (`ratchet.yaml`).
- Receipts live under the repo root (`.ratchet/`).
- The subject identity is a git tree.
- Oracle change detection compares `ratchet.yaml` across git refs.
- The built-in gates are git/commit-shaped (`pre-commit`) and CI-shaped
  (`check` plus `diff-oracles <base>`).

This is not an accident or a limitation to hide. It is the current boundary
model made concrete.

## Boundary levels

Different boundaries want different subjects, receipt stores, and oracles:

| Boundary | Subject crossing | Useful oracles |
|---|---|---|
| Agent step -> "done" | Transcript span, tool calls, patch-in-progress | Task-specific evaluator, targeted tests, diff policy, no unrelated edits |
| Agent worktree -> accepted patch | Worktree tree or patch | `ratchet check`, conflict-free merge, changed-files policy, task acceptance tests |
| Working tree -> commit | Git working tree | Fast format/lint/typecheck/unit tests, secrets scan |
| Branch/PR -> protected branch | Git tree/ref | Full tests/builds, integration tests, security scans, oracle weakening detection |
| Verification contract -> weakened contract | Manifest/oracle hash move | `diff-oracles`, separate ratification, proposer != ratifier |
| Source -> release artifact | Artifact digest | Reproducible build, SBOM/provenance, signing, release smoke tests |
| Candidate -> production | Deployment id/config/env | Health checks, migration safety, rollback readiness, live smoke tests |

The agent-loop "verify face" is therefore not the same thing as repo enforcement.
It is contextual and task-shaped. Ratchet can be a deterministic primitive inside
that verifier, but it is not the whole verifier unless the task's success criteria
are exactly the repo's declared oracles.

## Design implication

To generalize ratchet beyond the current repo/git model, the boundary would need
to become explicit instead of implicit. At minimum:

- **Boundary type:** repo, worktree, agent-step, release, deploy.
- **Subject identity:** git tree, patch, artifact digest, transcript span,
  deployment id.
- **Receipt store:** repo `.ratchet/`, worktree-local store, harness database,
  CI artifact, deployment evidence store.
- **Manifest discovery:** repo-root manifest, nested manifest, harness-supplied
  manifest, pipeline-supplied manifest.
- **Gate adapter:** pre-commit, PR check, agent-loop callback, merge callback,
  release pipeline step, deploy hook.

Until those are first-class concepts, ratchet should be described as
repo/git-boundary enforcement. A harness may still apply it at an agent-worktree
boundary by running ratchet before accepting or merging the worktree, but that is
composition by the harness, not a native boundary model in ratchet itself.

## Current framing to preserve

- **Pre-commit / PR gate:** ratchet can be the primary gate.
- **Agent verify face:** ratchet is one deterministic check the verifier calls,
  alongside contextual review.
- **Agent worktree -> merge:** ratchet is a good fit when the harness treats the
  worktree as the subject and refuses acceptance on red.
- **Release/deploy:** possible when wrapped as repo-declared commands, but not
  specialized because subject identity and receipt storage are no longer simply
  the repo tree and `.ratchet/`.
