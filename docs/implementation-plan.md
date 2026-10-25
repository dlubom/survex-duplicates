# Initial implementation plan — 2026-10-25

Historical record of the original implementation, before publication and the
DUPLICATE selection amendment. Current behaviour is defined by
[specification.md](specification.md) and ADR 0003.

The approved original specification, glossary and ADR 0001–0002 were the
requirements. Local review base: `d7655786c701b521a5ae1e1c0ce9bc574950a70f`;
local development branch: `codex/implement-survex-duplicates`. These identifiers
refer to preserved local history, not ancestors of the clean public root.

## Agreed TDD seams

1. Reader → records and metadata: format, validation, names, dates and flags;
   no search algorithm. Interfaces: `read_model(path, input_format="auto", dump3d="dump3d")`
   and `parse_dump(lines)` returning `SurveyModel`.
2. Public geometry predicate → match: both tolerance modes, orientation,
   boundaries and overflow; no index or files. `match_legs(a1,a2,b1,b2,tolerance)`.
3. Engine → ordered pair iterator: completeness against an independent oracle,
   varied radii and batches; no tests of KDTree internals. `Search(...).pairs(batch_size=1)`
   plus C/K and build statistics. The original interface's selection policy was
   later extended by `include_duplicates=False`.
4. CLI → status, diagnostics and saved report: arguments, filters, IDs, CSV
   and atomic output, observed through user-visible behaviour. `main(argv=None)`
   and `python -m survex_duplicates`.

## Original sequence

- Generate real LEG and MOVE/LINE fixtures with Survex and preserve provenance.
- Use vertical slices: one meaningful failing test, minimal implementation, next case.
- Implement reader, exact predicate, streaming KDTree iterator and oracle, then CLI/CSV.
- Integrate on the real binary fixture and read-only registry, checking hashes first.
- Measure sparse/repeated 50k/100k and smaller dense data, both tolerance modes,
  including C/K, timings, memory and environment.
- Complete verification and README; create the local implementation commit.
- Run independent Standards and Spec reviews against the fixed base, correct
  findings, verify corrections and commit locally.

Completion meant both input forms, all required behaviours, complete matching
pairs/orientations/errors equal to the oracle, deterministic CSV and measured
performance, with an explicit record of executed and skipped checks.
