# Architecture Decision Records

These ADRs record design decisions that **amend or correct** `docs/SEED.md`.
SEED remains the historical seed; each ADR below cites the SEED section it
supersedes and states *what SEED said → what we now hold → why*. When SEED and an
ADR conflict, the ADR wins.

This is deliberate dogfooding of the "spec-clause provenance" idea SEED §5 defers:
the binding spec moves forward in visible, diffable steps rather than being
silently rewritten.

| ADR | Decision | Amends |
|---|---|---|
| [0001](0001-verdict-identity-content-addressed.md) | Verdict identity is content-addressed over `(capability, subject, oracle, kind)` | §6; closes §10.2 |
| [0002](0002-three-state-status-and-exit-codes.md) | Three-state status (`pass`/`fail`/`error`), fail-closed, exit-code scheme | §7.3 |
| [0003](0003-run-is-a-declaration.md) | `run` is a declaration (argv, no shell, metacharacters rejected) | §6, §8 |
| [0004](0004-manifest-shape-and-strict-validation.md) | Manifest shape + strict validation: a manifest cannot express a vacuous oracle | §6 |
| [0005](0005-one-log-json-persisted-human-rendered.md) | One verdict log in v0; JSON persisted / human rendered; no findings/`--explain` | §6, §7.3, §7.4 |
| [0006](0006-doctor-calibrates-via-patch-probes.md) | Doctor calibrates the oracle via ratified patch probes | §7.2; closes §10.2 |

**Scope note.** ADRs 0001–0006 cover SEED build-order steps 1–3 (manifest schema,
`check`, `doctor`). Steps 4–7 (`init`, `install-hooks`, tamper defense,
self-host) are not yet decided. In particular the §7.1 tamper-defense *mechanism*
(§10.1 open question) remains unresolved; these ADRs only ensure steps 1–3 do not
foreclose it.
