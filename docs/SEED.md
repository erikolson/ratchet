# ratchet — seed spec

Handoff document. Everything decided so far. Read this first; it is the
Constrain face for the build.

---

## 1. What this is

A single Go binary that makes "verified" a **fact** rather than a **claim**,
in any repo, in any language.

Published agentic workflows (Pocock's skills, Superpowers, and every internal
`CLAUDE.md`) are **high substrate, zero bindingness**: they live in the repo,
diffable and version-controlled, and nothing on earth adjudicates a violation.
The agent can be told to TDD, decline, and nothing breaks.

`ratchet` closes that gap. It does not replace the skills. It puts a floor
under them.

**One-liner:** certainty in change.

## 1a. What layer this is — read this before writing any code

**`ratchet` is not an agent harness. It is the platform infrastructure the
agent harness composes.**

This distinction drives every design decision below. Get it wrong and you will
build a bespoke tool for one repo instead of a capability for a fleet.

- The **agent harness** is the loop: skills, slash commands, sub-agent
  topology, the model. Teams build their own, and they should.
- **`ratchet` is DSSD Layer 3** — Platform / Self-Service. Declare intent in a
  manifest; operational enforcement is *derived* from it. The repo never writes
  a hook. It declares a capability, and the platform generates the hook.

Every agent harness needs Verify and Feedback. Almost nobody builds them
properly, so they get reinvented badly per repo, or not at all. `ratchet` is
that layer, factored out and made portable. **One binary, N repos, one audit
trail.** That is the leverage, and it is the only reason this is worth
building.

The pitch:

> Every team is going to build their own agent workflows, and they should.
> What they shouldn't each build is the enforcement. That's one contract, one
> binary, N repos — declare what "verified" means here, and the enforcement is
> generated. Same audit trail across the fleet.

Note on the name: you *compose* `ratchet` in; what *ratchets up* is enforcement
over time. The install is composition. The ratcheting is the enforcement
accruing and never slipping back. Both are true; don't let the first meaning
eat the second, because one-directionality is the entire thesis.

---

## 2. The landscape — why this gap exists

The skills-library genre is enormous and getting better fast. Superpowers
(~250k stars), Pocock's skills (~167k), Anthropic's own. These are good. Do not
pitch against them; pitch *underneath* them.

**The accurate read — do not overclaim:**

- **Pocock's skills** are more than context. `code-review` spawns isolated
  sub-agents so their contexts don't pollute each other. `triage` is a state
  machine over issue labels. `handoff` promotes state out of context into a
  durable doc. That's *topology* — real harness architecture. But it's
  delivered as prose with zero bindingness: the agent can read `/tdd`, nod, and
  write the implementation first. Nothing breaks.
- **Superpowers** goes further and is actively reaching for bindingness. Its
  plugin ships a **session-start hook** so skills auto-trigger, and its evals
  drive real agent sessions and judge skill compliance with an **LLM verifier**.

Look closely at what each actually enforces:

| | What it enforces | What it does not |
|---|---|---|
| Session-start hook | **invocation** — the skill fires | **compliance** — that the agent did the thing |
| LLM compliance judge | a *probabilistic* verdict | a *deterministic* one |

**The gap is not that nobody is trying. The gap is that enforcement stops at
the boundary of the code.**

Neither repo can hand you a deterministic verdict on whether the change is
correct — because that verdict can only come from *your repo*, and no portable
skills library can know your test command.

**This is a law, not an oversight: portability is inversely proportional to
bindingness.** A hook that blocks a bad commit must name a concrete command.
Name it, and you can no longer ship to 167k strangers. That is why Pocock's
repo contains a `setup-matt-pocock-skills` skill that *bootstraps* a tracker
and doc layout — and says of itself that it is prompt-driven, not a
deterministic script. It is reaching for the substrate it needs and cannot
quite get there.

**The seam `ratchet` exploits:** don't ship the enforcement — ship the
**contract plus the generator**. The commands stay local (ten lines of YAML).
The schema and the generator travel. That is Positive Indifference applied to
the harness.

There are many, many skills repos. There are very few things trying to make
them binding.

---

## 3. The physics (the story, not the code)

- A **plumb bob** doesn't argue about vertical. It uses gravity — an external,
  deterministic force — as its reference. That is what an oracle is.
- A **ratchet** moves one way. Tightening is free; loosening is gated.
- The **teeth** are the log entries. The record of positions already passed.
- The **pawl** is the thing that drops into a tooth and stops the slip. That
  is the enforcement layer. Without a pawl, a ratchet is just a gear.

Plumb and pawl are whiteboard vocabulary. `ratchet` is the shipped name.

---

## 4. The frame

Four faces (Constrain / Generate / Verify / Feedback) on two substrates
(Reproducibility / Memory).

**Bindingness is not a fifth face. It is an axis every face sits on.**
The test is always: *who executes it?*

| Face | Advisory (model decides) | Enforced (runtime decides) |
|---|---|---|
| Constrain | skill, CLAUDE.md | PreToolUse hook, sandbox |
| Generate | the model | scaffold from spec |
| Verify | agent reads output | **the oracle** — exit code, `--json` |
| Feedback | prose in transcript | verdict + receipt |

Three connections:

1. Bindingness is an axis, not a face.
2. **Bindingness requires an oracle.** You cannot enforce what you cannot
   adjudicate. This is why Verify is the bottleneck and why `ratchet` starts
   there. The oracle is the load-bearing cell.
3. The substrates make the oracle trustworthy. Reproducibility = same verdict
   tomorrow. Memory = still there tomorrow.

Generate *should* stay advisory. You want the probabilistic thing to be
probabilistic. You just don't want it grading its own homework.

---

## 5. Scope

### v0 — ships in days, not weeks

```
ratchet init      # propose the manifest; human ratifies
ratchet doctor    # verify the verifier
ratchet check     # run capabilities, emit verdict + receipt
ratchet install-hooks
```

Plus **one hook**: block `git commit` when a declared capability is red.

That is the entire v0. Sixty seconds to value or it is dead on arrival. If the
README needs more than one screen, cut something.

### The second act — the evidence axis (deferred, but shaping v0)

**Read this even though you are not building it.** Two v0 requirements exist
*only* to serve this, and an agent that doesn't know it will cut them as
gold-plating.

v0 ships the **enforcement axis**: does violating this fail the build?
Act two adds the **evidence axis**: what witnesses do we have, and what has
this commitment earned the right to become?

**The two-axis model.** A commitment's state is `enforcement (soft/hard)` ×
`evidence (witnesses, tracked by identity, not count)`. Splitting these is what
makes the awkward cases ordinary — "frozen single-witness" (hardened despite
thin evidence) and "under review" (still enforced, thaw in flight) become
off-diagonal cells rather than special-cased states.

**Tightening is cheap; loosening is gated.** This is the organizing asymmetry
and the reason the tool is called `ratchet`. Adding evidence or hardening a
commitment can be automatic — an agent gains nothing by making a contract
stricter. Thawing, retiring, or hardening *without* evidence are gated, because
those are the directions a cheating agent wants to push. **Every gate sits
exactly where the incentive to cheat lives, and nowhere else.**

**Proposer ≠ ratifier, as a schema invariant.** The one decision the governed
agent can never own is the authority to change the contract it is measured
against. Thaw is two log entries (request, then verdict), so a validator can
mechanically reject any ratification whose author matches the requester.
Segregation of duties stops being a process people follow and becomes a
checkable property of the data.

**Append-only log; state is a fold.** The ossification log is tiny — a few
entries per commitment, ever — and never compacted, because the old entries are
the ratchet's teeth. A rejected thaw is a tooth: *we considered changing this
and held the line.* The spec can move forward but cannot un-learn.

**Receipts at every gate.** When a gated decision depends on a graph query,
freeze the answer into the log as a snapshot. Routine checks stay live and
ephemeral; gate evidence gets a permanent receipt.

**Where the agent's freedom lives.** Only in Generate. It cannot reach sideways
into Verify (weakening a test) or up into Constrain (editing the contract)
without passing a gate.

**Also deferred:** contract dependency graph, commitment lifecycle state
machine, consumer census (for retire), spec-clause provenance.

### What this means for v0 — the two things you must not cut

1. **Verdict identity.** A witness is a verdict *with identity*. Evidence is
   tracked by identity, not count. So a verdict can never be a bare
   `pass`/`fail` — it is `capability + fixture + commit`, stably identified.
   Cheap now; a migration later.
2. **Two logs, never merged** (§7.4). The verdict stream is exhaust and is
   compactable. The ossification log is teeth and is not. The second references
   the first as evidence; it never contains it.

**The division of labor:** `ratchet` v0 *implements* the enforcement axis and
*produces the material* for the evidence axis. It does not track evidence.
Act two is a fold over the verdict stream — which is why it gets easier, not
harder, once v0 exists.

**Plumb: is it true? The ratchet: what happens now that it has been true for a
while?**

---

## 6. The artifact

### `ratchet.yaml` — the only non-portable file

The irreducible core of bindingness. Someone must, once, tell the machine what
"verify" means *here*.

```yaml
version: 0
capabilities:
  typecheck:
    run: "mypy ."
    verdict: exit
  test:
    run: "pytest -q"
    verdict: exit
  lint:
    run: "ruff check"
    verdict: exit
```

Ten lines. That is the whole cost.

`verdict: exit` is the v0 adapter — zero means pass. Structured adapters
(`json/pytest`, `json/vitest`) come later; do not build them yet, but do not
close the door.

### The verdict — normalized, portable

```json
{
  "capability": "test",
  "status": "pass",
  "commit": "a3f9c21",
  "fixture": "working-tree",
  "duration_ms": 1840,
  "findings": []
}
```

**Verdict identity, not just verdict.** A verdict is `capability + fixture +
commit`, stably identified. Evidence is later tracked by *identity, not count*
— so this is cheap now and expensive to retrofit. Do not ship a bare
`pass`/`fail`.

### The receipt log

Append-only. `.ratchet/receipts.jsonl`.

```
2026-07-13T14:02Z  BLOCK    test  oracle-red        commit=a3f9c21
2026-07-13T14:19Z  VERDICT  test  pass              commit=b7e2d04
```

---

## 7. Non-negotiables

These four are what separate this from theater. Do not cut them for scope.

### 7.1 Manifest tamper defense

The cheating move is not subtle: agent hits a red test, edits `ratchet.yaml`,
deletes the check.

The gate sits exactly where the incentive to cheat lives.

- **Adding** a capability: free. Tightening is cheap.
- **Removing or weakening** one: **blocked.** The agent has no write path to
  the manifest, period.

`ratchet init` proposes; the human ratifies; the agent is locked out
afterward. This is proposer/ratifier at its minimum viable form. Without it,
`ratchet` is decoration.

### 7.2 `doctor` — verify the verifier

Nobody builds this and it is the one that matters. `doctor` deliberately
breaks the code, runs each declared capability, confirms the verdict **flips**
to fail, then restores and confirms it flips back to pass.

An oracle that has never been observed to say *no* is a rumor.

### 7.3 Emit on green *and* red

Silence is ambiguous. It could mean "everything passed" or "the harness died
three hours ago." A control you cannot see running is not a control.

```
✓ test · typecheck · lint                          (green: one line, forgettable)
✗ BLOCKED  commit
   capability: test → FAIL (3 findings)
   run `ratchet check --explain test`              (red: loud, actionable, cites the rule)
```

Corollary: **liveness is itself a check.** No heartbeat within N tool calls →
warn. A silent harness must fail *closed*, not open.

**The green tick is what makes the red one credible.**

### 7.4 Two logs, never merged

- **Verdict stream** (`ratchet`) — high volume, one entry per check,
  compactable. Execution exhaust.
- **Ossification log** (the later ratchet layer) — tiny, a few entries per
  commitment *ever*, never compacted, because the old entries are the teeth.

The ossification log *references* verdicts as evidence. It does not contain
them. Merge these and the ratchet drowns in noise and stops being a ratchet.

---

## 8. Design constraints

- **Go.** Single static binary, no runtime, no venv. A harness tool with an
  installation problem is a harness tool nobody installs. Cobra, matching
  `cperm`.
- **`ratchet` never calls a model.** No API key, ever. It is the non-generative
  faces by definition. The probabilistic thing is the agent you already pay
  for; the deterministic thing is this binary. They meet at a hook and an exit
  code.
- **Positive Indifference.** The language of `ratchet` is unrelated to the
  language of the repo it inspects. It shells out to whatever the manifest
  declares. Go binary, Python victim, TypeScript victim. One tool, whole fleet.
- **Self-hosting.** `ratchet` runs `ratchet` on itself. Highest-credibility
  signal a platform tool can send, and it costs nothing extra.
- **Fleet-shaped, not repo-shaped.** Nothing in `ratchet` may assume it is the
  only consumer of a repo, or that the repo is this repo. No hardcoded
  commands, no assumed language, no project-specific defaults. If a design
  choice would need to change for the second repo that adopts it, it is wrong.
  **Portability is the product.**

---

## 9. The demo

Victim repo: `trim-demo-harness` (purpose-built to expose what happens when
Verify is weak — exactly the axis needed).

1. `npx skills@latest add mattpocock/skills` → run `/setup-matt-pocock-skills`
2. Run `/implement`. The agent is *told* to TDD. Sometimes it doesn't. Nothing
   breaks. **Suggestion.**
3. `ratchet init` → `ratchet doctor` → `ratchet install-hooks`
4. Run `/implement` again. Runtime refuses. Receipt written. **Guarantee.**

Same rig, same skills, one variable changed. Ninety seconds.

---

## 10. Open questions

- Manifest lockout mechanism: file permissions, hash-in-git, or hook-level
  path deny? Hook-level is probably cheapest and most portable.
- Does `doctor` mutate the working tree, or run against a temp clone? Temp
  clone is safer and probably not much slower.
- Heartbeat: where does the counter live so it survives across tool calls?
- Where does `install-hooks` write — `.claude/settings.json`, `.git/hooks`, or
  both? Both, probably, with the git hook as the backstop for humans.

---

## 11. Build order

1. `ratchet.yaml` schema + parser
2. `ratchet check` → run capabilities, emit normalized verdict, write receipt
3. `ratchet doctor` → the flip test
4. `ratchet init` → propose manifest (agent reads repo, human ratifies)
5. `ratchet install-hooks` → PreToolUse block on red
6. Manifest tamper defense
7. Self-host

Steps 1–3 are the product. Everything else is delivery.
