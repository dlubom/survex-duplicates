# Publication preparation

Historical plan for the initial repository publication. Current Go runtime and
package publication decisions are documented in
[ADR 0004](adr/0004-go-runtime.md) and the [release guide](releasing.md).

## Goal

Prepare a portable, English-language repository with an immediately runnable
full-registry demo and concise instructions for coding agents.

## Decisions

- Use `AGENTS.md` for repository instructions; no Agents SDK dependency.
- Bundle the unchanged registry snapshot and its full text dump under
  `examples/kataster/`, with SHA-256, upstream provenance, attribution and CC BY-SA 4.0 notice.
- Use the bundled model as the default integration input; retain `KATASTER_PATH`
  for explicitly selected snapshots. Check its hash before using expectations.
- Translate documentation and user-facing CLI diagnostics into English.
  Preserve station names, survey names and Unicode test data verbatim.
- Redact machine-specific paths from published metadata, retaining original
  artifact hashes and documenting exactly which metadata changed.
- Preserve existing local development history. Prepare a clean root commit
  on `main` for publication so old path-containing commits are not its ancestors.
- The owner selected MIT for the software and `dlubom/survex-duplicates` as the destination.

## Execution

1. Add demo inputs and provenance; make integration portable.
2. Translate CLI diagnostics through the approved reader and CLI test seams.
3. Write README, AGENTS.md and English reference documentation; document the amended DUPLICATE default and explicit audit option.
4. Replace private paths in historical metadata and local source links with
   portable paths and upstream links. Preserve numerical evidence and data hashes.
5. Run the full suite, type checking, lint/format checks, the documented demo,
   complete pair/orientation comparisons and package builds.
6. Audit the publication tree and its reachable history for private paths,
   then create the clean publication commit. Publish only to a confirmed destination.
