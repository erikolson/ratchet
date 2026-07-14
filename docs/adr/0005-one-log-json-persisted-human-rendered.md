# ADR-0005 — One verdict log in v0; JSON persisted / human rendered; no findings or `--explain`

- **Status:** Accepted
- **Date:** 2026-07-14
- **Amends:** SEED §6, §7.3, §7.4
- **Origin:** grill Q4, Q5

## What SEED said

A single `.ratchet/receipts.jsonl` whose *example content* was space-aligned human
text mixing `BLOCK` (a hook action) and `VERDICT` (a check action). Verdicts had
`findings: []`; §7.3 showed `FAIL (3 findings)` and `ratchet check --explain`. §7.4
described "two logs, never merged" as though both shipped.

## What we now hold

### Two surfaces, never merged

- **Terminal** — human, coloured, ephemeral, never parsed by anything. On a red,
  `check` streams the *failing tool's* stdout/stderr live, at the moment of
  failure (as `make`/`cargo`/`go test` do).
- **Log** — machine-first **JSONL**, one verdict object per line, append-only, at
  **`.ratchet/verdicts.jsonl`**. (§6's `BLOCK/VERDICT` example was terminal output
  wearing a filename — deleted.)

The word **"receipt"** is reserved for act-two *governance* artifacts (per-gate
snapshots for thaw/freeze/retire) and is **not** a v0 filename.

### v0 writes exactly ONE log

Discriminated by `kind` (ADR-0001):

- `check` → written by `ratchet check`.
- `calibration` → written by `ratchet doctor`.
- `gate` → **reserved in the schema, not written in v0** (step-5 hook). A gate
  entry, when it lands, references verdicts *by identity* and never re-embeds them.

The ossification log (act two) **does not exist in v0** — not a file, not a stub.

### No findings, no `--explain`

- `findings` stays in the schema but is **`[]` by construction** under the exit
  adapter. Raw output is *not* a finding; a finding is a structured parsed claim,
  which only a later `json/*` adapter produces. §7.3's "(3 findings)" and
  `--explain` are deleted as act-two leakage.
- **Nothing is captured or persisted** beyond the verdict. `--explain` is cut:
  output has exactly one destination — the terminal, live, at the moment of the
  run.
- **Re-run-to-explain is rejected on *correctness*, not performance:** re-running
  executes against a *different `subject`* and would explain something the verdict
  never adjudicated (ADR-0001).
- **`--json`** writes verdict objects to **stdout**; any tool output goes to
  **stderr**.

### File inventory

- **Committed substrate:** `ratchet.yaml`, `.ratchet/probes/*.patch`,
  `.ratchet/.gitignore` (written by `init`, ignores `verdicts.jsonl`).
- **Local exhaust:** `.ratchet/verdicts.jsonl` (gitignored, compactable).
- **Does not exist in v0:** `.ratchet/state`, the ossification log.

**Coverage is derived, never stored** — `doctor` recomputes it each run; nothing
persists, so nothing can be tampered with. General rule: **anything that gates
must be tamper-defended; anything that cannot be tamper-defended must not gate.**
In v0 exactly two things gate — the manifest and the probes — both committed and
human-ratified (§7.1).

### Housekeeping

Timestamps RFC3339 UTC. `O_APPEND` writes kept under 4096 bytes so lines are
atomic. Each line carries a format-version field for later migration.
`verdicts.jsonl` is compactable (exhaust); the future ossification log will not be
(teeth) — *that* is the real reason §7.4 says never merge them.

## Why

An append-only file written on every check would conflict on every merge, so the
verdict stream must be gitignored; teeth that aren't in version control aren't
teeth, so the ossification log must be committed. One compactable, one not; one in
git, one not.

## Consequences for v0 (steps 1–3)

- `check` and `doctor` append newline-delimited JSON verdict objects.
- README limitation: gitignored verdicts give the *developer* certainty in the
  record; fleet-wide audit needs a different transport (CI artifact, telemetry)
  and is out of scope for v0.
