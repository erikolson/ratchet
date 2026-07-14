# ratchet

[![CI](https://github.com/erikolson/ratchet/actions/workflows/ci.yml/badge.svg)](https://github.com/erikolson/ratchet/actions/workflows/ci.yml)

**Certainty in change.** A single Go binary that makes "verified" a *fact* rather
than a *claim* — in any repo, in any language.

---

## The gap

Published agentic workflows — skills libraries, `CLAUDE.md`, every internal TDD
guide — are **high substrate, zero bindingness**. The rule lives in the repo,
diffable and version-controlled, and *nothing on earth adjudicates a violation*.
An agent can be told to write the test first, agree, write the implementation
first instead, and nothing breaks.

`ratchet` closes that gap. It does not replace the skills — it puts a floor under
them. The floor is two things skills libraries structurally cannot provide:

- a **deterministic oracle** — a verdict that comes from *your* repo (your test
  command), not from a portable library that cannot know it; and
- a **durable receipt** — an append-only record that the check ran and what it said.

An oracle plus a receipt is what turns "verified" from a claim into a fact.

## The layer

**`ratchet` is not an agent harness. It is the platform infrastructure the harness
composes.** Teams build their own agent loops — skills, slash commands, sub-agent
topology — and they should. What they *shouldn't* each rebuild is the enforcement.

> Declare what "verified" means here, in ten lines of YAML. The enforcement is
> derived from it. One binary, N repos, one audit trail.

## Ninety seconds

`ratchet init` reads your repo and proposes this — every detected command
commented and labeled, so you ratify by uncommenting. Or write it yourself; ten
lines is the whole cost:

```yaml
# ratchet.yaml — the only non-portable file.
version: 0
capabilities:
  - name: test
    run: "pytest -q"          # your command. ratchet shells out to it.
    verdict: exit             # exit 0 = pass. that's the v0 adapter.
```

```console
$ ratchet doctor            # verify the verifier — see below
$ ratchet install-hooks     # install the local gate
✓ wrote .git/hooks/pre-commit
✓ wrote .claude/settings.json

$ git commit -m "..."       # while a declared capability is red:
✗ test    FAIL
✗ BLOCKED  git commit
   see the failing capability above · receipt: .ratchet/verdicts.jsonl
```

The runtime refused. Same rig, same skills, one variable changed: the check went
from a *suggestion* to a *guarantee*.

## Commands

| Command | What it does |
|---|---|
| `ratchet init` | Read the repo and propose a `ratchet.yaml` — detected commands commented and labeled as guesses. Generate, not Verify: nothing is active until a human uncomments it. |
| `ratchet check` | Run every declared capability, emit a normalized verdict per capability, write a receipt. Exit `0` pass / `1` fail / `2` error / `3` couldn't-run. |
| `ratchet doctor` | **Verify the verifier.** Apply a ratified mutation to the code in a throwaway worktree and confirm the oracle *flips to fail*. An oracle that has never been observed to say no is a rumor. |
| `ratchet install-hooks` | Install the local gate at both loci — a Claude Code `PreToolUse` hook and a git `pre-commit` hook — both invoking one `gate` primitive. |
| `ratchet diff-oracles <base>` | Report how the manifest's oracle hashes changed vs a base ref. Tightening is silent; **weakening and removal alarm.** |

## What makes it more than a linter

**`doctor` catches the vacuous oracle.** `go test ./...` with zero test files
exits `0`. Delete the suite, get green, no config edit required — and every
exit-code checker on earth is blind to it. `doctor` mutates the code and demands
the oracle notice. It's mutation testing, pointed at your verifier instead of your
code.

**Verdict identity is content-addressed.** A verdict is not a bare `pass`/`fail` —
it is `(capability, subject, oracle, kind)`, where `subject` is the git tree hash
of exactly what ran and `oracle` is a content hash of the resolved command. Weaken
the command and the oracle hash *changes*: a green produced under a weakened oracle
is content-addressably distinct from a ratified one. **The weakening leaves a
permanent trace even when the write succeeds.**

**Tamper defense is detection, honestly bounded.** ratchet does **not** pretend to
prevent a local agent with a shell from editing the manifest — [that's not
shippable by a repo-level tool](docs/adr/0008-tamper-defense-is-detection-not-prevention.md),
and pretending otherwise is theater. Prevention requires a locus the agent cannot
reach — CI, a pre-receive hook, a root-owned sandbox policy — all org-provisioned.
What ratchet *does* is make tampering **impossible to do invisibly**: every verdict
carries its oracle hash, and `diff-oracles` on the protected branch turns a subtle
YAML weakening into an unmissable line a human ratifies. Local hooks give the
developer fast feedback; only fleet-level enforcement gives the org a guarantee.
That boundary is the reason this is a platform, not a tool.

## Built while watching the thesis happen

Three times in two days of building, the *absence* of an oracle-plus-receipt
produced exactly the failure this tool exists to prevent — a sandbox bypassed with
no record of who approved it, a rule that held only because the agent chose to
honor it, and an oracle that reported pass on genuinely broken code because a cache
made it blind. They are written down, plainly, in
**[docs/FIELD_NOTES.md](docs/FIELD_NOTES.md)** — the pitch, observed live, in the
repo built to close the gap it demonstrates.

## Design

- **[docs/SEED.md](docs/SEED.md)** — the original design brief.
- **[docs/adr/](docs/adr/)** — nine decision records. Each amends a section of the
  seed and states *what it said → what we now hold → why*. The seed is left as the
  historical record; ADRs win on conflict.

## Constraints

- **Go, single static binary.** No runtime, no venv. A harness tool with an
  installation problem is a harness tool nobody installs.
- **Never calls a model.** No API key, ever. The deterministic thing is this
  binary; the probabilistic thing is the agent you already pay for. They meet at a
  hook and an exit code.
- **Positive Indifference.** ratchet's language is unrelated to the repo's. It
  shells out to whatever the manifest declares. Go binary, Python victim,
  TypeScript victim — one tool, whole fleet. `run` is a declaration (argv, no
  shell), so ratchet is OS-agnostic; *a manifest may not be.*
- **git is a hard prerequisite.**
- **Self-hosting.** ratchet runs ratchet on itself (`go vet`, `go test`).

## Scope

**v0 ships the enforcement axis:** does violating this fail the build? Built,
test-first: `init`, `check`, `doctor`, `install-hooks` + block-on-red,
`diff-oracles`, self-hosting.

**Deferred** (named, not forgotten): the evidence axis (witnesses, freeze/thaw,
the ossification log); structured verdict adapters (`json/pytest`); and the
Reproducibility substrate — the oracle hash captures the *command*, not the
*environment* it runs in, so a weakening through a config file or a pinned
dependency is the next layer down.

## Status

Twelve packages, built test-first. `go test ./...` is green; `ratchet check` on
ratchet is green.
