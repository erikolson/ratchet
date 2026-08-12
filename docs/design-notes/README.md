# Design notes

Open explorations that are **not yet decisions**. A design note captures a problem,
the reasoning around it, and candidate shapes — the material that would be premature
to freeze as an ADR because nothing has been chosen yet.

The distinction from the neighbours:

- **[ADRs](../adr/)** — decisions *made*. Binding; supersede SEED on conflict.
- **[FIELD_NOTES](../FIELD_NOTES.md)** — things that *happened* while building, and
  what they taught.
- **Design notes** (here) — problems *identified*, thinking *in progress*. When a note
  resolves into a choice, it graduates to an ADR and the note links to it.

| Note | Topic |
|---|---|
| [boundary-scoped-enforcement](boundary-scoped-enforcement.md) | Enforcement belongs at boundaries; current ratchet is repo/git-scoped, and broader boundary levels would need explicit subject identity, receipt storage, manifest discovery, and gate adapters |
| [oracle-change-direction](oracle-change-direction.md) | Telling a *tightening* from a *weakening* when an oracle is modified — why v1 refuses to, and what earning the right to infer it would take |
