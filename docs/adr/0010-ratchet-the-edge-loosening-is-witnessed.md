# ADR-0010 — Ratchet the edge: loosening is witnessed

- **Status:** Accepted (proposed by the assistant, ratified by the human — proposer ≠ ratifier, applied to its own adoption)
- **Date:** 2026-07-21
- **Amends:** SEED §5 (the evidence axis lands narrow, not as the full lifecycle); relates ADR-0008 (completes its detection story into adjudication), ADR-0005 (the ossification log now exists), ADR-0001 (verdict identity is the material this folds over)
- **Origin:** the "push change to the edge" grill

## The Physics

The physics of elegant modern software engineering **pushes change to the edge**:
a stable core depends on abstractions, and everything that varies is injected from
outside. ratchet already lives this. Two things vary per repo, and both are at the
edge:

- **the spec** — `ratchet.yaml`: what to check, and the pass/fail threshold; and
- **the verifier mechanism** — the `run:` command or repo-local script the spec
  points at: *how* it is tested.

What stays in the core is a single, fixed converter: run the argv, read the exit
code, normalize to a verdict (the `exit` adapter). It is language-indifferent and
**does not grow per application** — a thousand repos in a thousand languages reuse
the same handful of lines. This is dependency inversion, and it is the reason
Positive Indifference (§8) is coherent: the core cannot know your test command, so
the mechanism *must* be injected.

## The tension

Pushing change to the edge normally makes the edge **freely mutable** — that is the
whole point of it: cheap, local change against a stable core. But ratchet's threat
model adds a fact generic architecture does not carry:

> **The edge is exactly where the incentive to cheat lives.**

The same locality that makes a threshold cheap to *tighten* makes it cheap to
*gut*. Narrow `pytest -q` to `pytest -q tests/smoke`, delete an assertion from the
repo-local verifier, widen a `pass:` set — each is an ordinary edit at the edge, and
each is a weakening of the contract the agent is measured against. Field notes #1
and #2 are this exact shape: an advisory control at the edge, loosened the moment it
was inconvenient, with nothing to adjudicate it.

So the design cannot simply obey the physics. Nor can it drag the mechanism back
into the core to protect it — that re-centralizes measurement, bloats the binary
per ecosystem, and ADR-0008 already established that *no* repo-level locus can
prevent the edit anyway.

## What we hold

**Push change to the edge — then ratchet the edge's loosening.** The resolution is
not to constrain the edge but to constrain *one direction* of change at the edge,
asymmetrically:

- **Tightening** the spec or strengthening the verifier → free, silent. The physics
  is honored in full: an agent gains nothing by making the contract stricter, so
  strengthening needs no ceremony.
- **Loosening** either → **witnessed**: it must leave a permanent, committed tooth,
  and clearing the alarm requires a *differently-authored* ratification.

This is SEED §5's organizing asymmetry made concrete, and it places the gate
**exactly where the incentive to cheat lives, and nowhere else.**

### The mechanism: the ossification log's first entry type

The ossification log — deferred by ADR-0005 as "does not exist in v0" — now exists,
at **`.ratchet/ossification.jsonl`**: **committed** (unlike `verdicts.jsonl`,
which is gitignored exhaust), append-only, and **never compacted**, because the old
entries are the teeth. Its only entry type in this slice is `oracle-ratification`:

```json
{"v":1,"type":"oracle-ratification","capability":"test",
 "base_oracle":"5b24e3b…","new_oracle":"a91f0c4…",
 "requester":"agent","ratifier":"alice","decision":"ratified",
 "timestamp":"2026-07-21T14:02:00Z"}
```

A `decision:"rejected"` entry is equally a tooth — *we considered loosening this and
held the line* — and records `base_oracle → new_oracle` even though the change never
landed. The spec can move forward; it can never un-learn.

### The gate binds to `diff-oracles`

`diff-oracles` today (ADR-0008) reports a `changed`/`removed` oracle as an
unmissable line and exits nonzero — but nothing records the human's *yes*. That is
detection without adjudication: field note #1 reproduced inside ratchet's own tamper
story. This ADR closes it:

> A `changed` or `removed` oracle clears (exit 0) **only** when a matching
> `oracle-ratification` entry exists with `decision:"ratified"`, matching
> `base_oracle`/`new_oracle`, and **`ratifier ≠ requester`**. `added` (tightening)
> stays silent and needs no entry.

Proposer ≠ ratifier stops being a process people follow and becomes a **checkable
property of the data** (SEED §5).

### Where the authority lives — the same locus as prevention

ADR-0008 is binding here and must not be contradicted: on one machine, authorship is
self-asserted (the agent controls `git config user.email`), so `ratifier ≠
requester` is **not enforceable locally.** The ratification gate is therefore an
**off-machine check** — CI against the protected branch, where authorship derives
from branch protection or signed commits. Locally, `ratchet ratify` and the
ossification log buy the same thing the local gate buys: fast feedback, raised cost,
and a trace. The *guarantee* is off-machine, exactly as prevention is. This ADR adds
no new trust assumption; it folds over the material ADR-0001 and ADR-0008 already
produce.

## Scope — narrow on purpose

This is the minimal slice of the evidence axis that closes the ADR-0008 hole, and
nothing more. **In scope:** the committed ossification log, the
`oracle-ratification` entry (ratified and rejected), and the `diff-oracles`
ratification gate. **Explicitly deferred, named not forgotten:** the enforcement ×
evidence state machine, freeze/thaw of a commitment, hardening-earned-by-witness
(evidence tracked by count/identity), the contract dependency graph, consumer census
for retire, and spec-clause provenance (SEED §5). Those are the parts that turn
speculative before there is usage to fold over.

## The named limitation — this ratifies the sentence, not the meaning

The oracle hash covers argv/pass/fail/timeout/version (ADR-0001), **not the content
the argv executes.** So a repo whose oracle is `./verify.sh` can gut the *body* of
`verify.sh` with the oracle hash — and therefore this ratification gate — none the
wiser. That is ADR-0008's layer boundary and field note #3 (the `.pyc` bug)
restated: **the oracle hash pins the contract, the Reproducibility substrate pins
what the contract runs against, and v0.5 ships the first, not the second.** Pinning
an artifact's content by hash — generalizing the oracle from "a command" to "a repo
artifact under the ratchet" — is the natural next tooth and is the same move as this
one, applied one level down. It is left to a sibling ADR so this decision stays
about the command.

## Consequences for the build

- A new committed file `.ratchet/ossification.jsonl`; `init` writes the
  `.ratchet/.gitignore` so it is tracked while `verdicts.jsonl` stays ignored.
- A new command `ratchet ratify` writes an `oracle-ratification` entry (request →
  verdict as two authored acts, per SEED §5); a rejection is a first-class outcome.
- `diff-oracles` gains a ratification check: `changed`/`removed` clears only against
  a matching, differently-authored `ratified` entry; `added` stays silent. Wired as
  a required CI check on the protected branch is where it becomes a guarantee.
- No local enforcement of `ratifier ≠ requester` ships or is implied — the guarantee
  is off-machine, consistent with ADR-0008.
- This ADR is the provenance anchor for the README's "what's injected, what's fixed"
  articulation: the README states the principle; this ADR holds *why*.
