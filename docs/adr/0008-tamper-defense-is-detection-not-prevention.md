# ADR-0008 — Tamper defense is detection + locus, not local prevention

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §7.1; closes §10.1 (lockout mechanism) and §10.4 (install-hooks location)
- **Origin:** §7.1 threat-model grill

## What SEED said

§7.1 frames manifest tamper defense as prevention — "the agent has no write path
to the manifest, period" — and §10.1 asks *which* local mechanism (file
permissions, hash-in-git, hook-level path deny) achieves it.

## What we now hold

**Local prevention against an agent with unrestricted shell and the developer's
uid is not achievable by anything ratchet can ship, and pretending otherwise is
the theater SEED §7 exists to avoid.**

The precise boundary (not "prevention is impossible" — that invites a correct
rebuttal):

> **Prevention requires a locus the agent cannot reach. There are exactly two —
> off-machine (CI, pre-receive hook) or root-owned (managed OS / kernel sandbox
> policy: Seatbelt, Landlock). Both are org-provisioned; neither is
> repo-provisioned. So prevention is real, but it is never something a repo-level
> tool can deliver.**

Why every repo-level mechanism fails against a shell agent sharing the uid:

- **PreToolUse path-deny** matches strings; `printf > "$F"`, `sed -i`, temp-then-
  `mv`, `python -c` all evade it, and detecting "this shell command writes that
  path" is undecidable. (A *kernel* sandbox with a root-owned policy defeats these
  — but that is the org-provisioned locus, not ratchet.)
- **File permissions** — same uid rewrites them; `rm`+recreate needs only
  directory write, which the agent must have to do its job.
- **git hooks** — `git commit --no-verify`, or delete `.git/hooks/*`.
- **`.claude/settings.json`** — the hook that would guard it is registered in the
  file being edited.
- **proposer ≠ ratifier** — the agent controls `git config user.email`; author
  identity is self-asserted unless signed by a key the agent lacks, and on one
  machine there is no such key.

### Detection survives, and it is free

Every verdict carries the **oracle hash** (ADR-0001 / Q10) — a content hash of the
resolved capability (argv, pass/fail, timeout, version). So a weakened capability
is **content-addressably distinct** from the ratified one: a green produced under a
weakened oracle has a different oracle hash. The weakening leaves a permanent trace
in the append-only stream **even when the write succeeds and the local gate is
bypassed.** This property fell out of an identity decision made for unrelated
reasons — the strongest evidence yet that the identity model is load-bearing.

Detection becomes a *guarantee* only when the reference and the log live where the
agent cannot rewrite them — the same off-machine locus. Locally, ratchet buys fast
feedback, raised cost, and a trace.

### The ratified reference is the protected branch, not a synced file

A committed `.ratchet/ratified.*` the agent can also edit buys nothing (an agent
that edits both consistently passes both checks) and adds a sync burden. **Rejected.**

The reference already exists: the manifest on the protected branch. A weakening is
a **diff a human ratifies** — proposer ≠ ratifier built from branch protection,
machinery every org already owns, no keys, no new file. The artifact is a command:

```
ratchet diff-oracles <base>
```

- Reports **added** (tightening — silent), **changed** (loud), **removed**
  (loudest). The tightening/weakening asymmetry is the ratchet thesis, made visible.
- **Informational with a signal exit code**, not a hard blocker: blocking every
  oracle change would forbid legitimate test-command updates. ratchet emits the
  unmissable signal; the org decides whether to wire it as a required check.
- Works in CI (against the protected branch) and locally (`diff-oracles origin/main`
  as fast feedback). `check` needs no local ratified set — it already records the
  oracle hash; the comparison happens where the base ref lives.

### The layer boundary (state it; it is roadmap, not gap)

`diff-oracles` sees the **declared command**, not the environment it runs in. A
weakening *through* the environment — a config file the command reads, an env var,
a pinned dependency — is invisible to it. That is field-note #3 (the `.pyc` bug)
generalized: **the oracle hash covers the contract; the Reproducibility substrate
covers the environment; v0 ships the first, not the second.** Detection is complete
over what v0 makes content-addressable. The environment is the next substrate down.

## Consequences for the build

- **No file-lock, no `chmod`, no synced ratified file.** None ship.
- `install-hooks` writes a local gate at **both** loci SEED §10.4 names — a
  PreToolUse hook (`.claude/settings.json`, fast in-agent feedback) and a git
  `pre-commit` hook (`.git/hooks`, the human backstop) — both invoking one
  `ratchet gate` primitive so the two surfaces emit an **identical `kind:gate`
  verdict** (one readable stream for act-two).
- The `kind:gate` verdict **references the check verdicts by identity** (Q4 /
  ADR-0005), never re-embeds them.
- The local gate is documented as fast-feedback-and-trace, **not** a guarantee
  against a determined local agent. Prevention lives off-machine — stated in the
  README as a design boundary, not an apology.
- `diff-oracles` ships **with** `install-hooks` (both are step 5): the honest
  tamper coda of the demo is `diff-oracles`, so it cannot lag behind the gate.
