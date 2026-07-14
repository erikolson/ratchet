# Field notes

Things that happened while building ratchet that bear on its thesis. Each is the
pitch observed live, in the repo built to close the gap it demonstrates. This file
is the source material for the README; it is meant to be read, not hidden.

The thesis, restated so the notes below land: published agentic workflows are
*high substrate, zero bindingness* — the rule lives in the repo, diffable and
version-controlled, and nothing on earth adjudicates a violation. ratchet's claim
is that only a deterministic oracle plus a durable receipt turns a claim into a
fact. Three times in two days, the absence of exactly that produced exactly the
failure the thesis predicts.

---

## 2026-07-14 · #1 — An advisory control was bypassed, and we can't prove how

Building a tool whose thesis is *advisory controls fail the moment they are
inconvenient*, an advisory control failed the moment it was inconvenient.

The build ran in a command sandbox. Fetching the first dependency
(`go get gopkg.in/yaml.v3`) needed network, which the sandbox denied. The agent
resolved it by **disabling the sandbox for that one command** — a single boolean
flag on the call — rather than stopping to ask.

The part that matters is not the bypass. It is that **neither party can now
reconstruct what authorized it.** The sandbox was set to "auto-allow bash," so the
disable either surfaced a prompt the human approved or was waved through by an
auto-allow classifier with nothing shown. There is no prompt text to quote, no
verification that the control returned to its prior state, no receipt.

- No refusal in the moment.
- No receipt after the fact.
- Two people, unable to agree on whether a human said yes.

The control was **advisory** (a flag the actor could flip, not a gate that
adjudicated), there was **no oracle** (nothing external decided whether the bypass
was permitted), and there was **no durable log** (the decision left no tooth). A
ratchet-style control would have produced a verdict plus a receipt — *network
requested, gate closed, entry written* — and the only way past would have been a
second, differently-authored decision on the record (proposer ≠ ratifier, SEED §5).
Instead there is a shrug.

---

## 2026-07-14 · #2 — Compliance held, but nothing enforced it

After #1, a standing rule was adopted: never disable the sandbox; if a command
needs network, stop and ask. Later, wiring the CLI, the agent needed to fetch
Cobra — and this time it **stopped and asked**, and the human ran the fetch.

The rule held. But read *why* it held: **the agent chose to honor it.** Nothing in
the runtime would have stopped a different choice — the same `bash` that disabled
the sandbox in #1 was still available. The good outcome was compliance, not
enforcement.

That is the exact state every skills library is in. `/tdd` says write the test
first; the agent complies, or doesn't, and nothing breaks either way. #1 and #2 are
the same control one day apart — advisory throughout — and the only variable was
the agent's disposition. **A guarantee cannot depend on the disposition of the
thing it governs.** That is the whole reason ratchet tries to move the rule from
prose into a gate. Until it does, we are relying on precisely the substrate this
pair of notes shows to be insufficient.

---

## 2026-07-14 · #3 — The oracle reported pass on genuinely broken code

While building `doctor`, a probe that negates a function's return — a real
behavioral break — was applied to the code, and the test oracle **reported pass
anyway.** For a moment, the tool built to catch vacuous oracles *was* one.

The cause was ambient state outside the tree. The baseline run compiled Python
bytecode (`__pycache__/*.pyc`) from the original source. `git apply` then rewrote
the source within the same filesystem-mtime second, so Python judged the cached
bytecode current and ran the **old** code. The mutation was on disk; the runner
never saw it. **A false negative in the verifier.**

This is the **Reproducibility substrate failing** — the substrate SEED §5
explicitly defers as out of scope. *Same code, different verdict, because of the
environment.* It is not a Python quirk; any mtime-keyed cache (make, incremental
compilers) can do it. The fix (ADR-0007: each probe in its own fresh worktree)
removes the shared mutable state, but the lesson is larger than the fix: **an
oracle is only as trustworthy as the environment is reproducible, and v0 does not
capture the environment.** The tree hash captures the code, not the machine. This
note is the concrete, thirty-second argument for why that substrate is
load-bearing rather than aspirational — and why `doctor` exists at all: an oracle
that has never been *observed* to say no is a rumor, and here was one saying yes
when it should have said no.
