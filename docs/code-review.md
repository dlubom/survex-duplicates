# Initial implementation review — 2026-10-25

Historical report from `mattpocock-skills:code-review`, with independent Standards
and Spec contexts. The approved local specification replaced an issue tracker.
The following local IDs are evidence labels; they are not public commit links
or ancestors of the clean publication root.

- Base: `d7655786c701b521a5ae1e1c0ce9bc574950a70f`.
- Implementation: `f129f3744b955bffd5e4b134e6d7672e8e2bf4b1`.
- Correction: `c5120920ac8c890d087d7cf650ac7012d6317fd7`.
- Standards sources: glossary, ADR 0001–0002, the specification's organisation
  and exact-arithmetic/memory rules, agreed seams, README, pyproject and the
  Fowler smell baseline. AGENTS.md was added later during publication preparation.

The review preceded the owner's DUPLICATE-default amendment. Historical registry
counts in this report are reproduced today using `--include-duplicates`.

## Standards

English translation of the independent report:

> Hard violations of documented standards: **0**. Responsibilities, public
> interfaces, exact arithmetic and streaming agree with the documented requirements.
>
> **P3 — possible Duplicated Code:** reader.py and geometry.py repeat the same
> decimal-number grammar. A syntax change would need two updates. Share the
> pattern while retaining separate Fraction/Decimal conversion. This is a
> maintainability judgement, not a demonstrated behaviour defect.

The patterns were confirmed identical and moved into private `_decimal_syntax.py`.
The four public seams and context-specific validation stayed the same. Reader
and geometry tests after the correction: **98 passed**; strict mypy and Ruff passed.
The Standards reviewer independently inspected the correction and confirmed
resolution without finding a new issue.

## Spec

English translation of the independent report:

> No confirmed departures from the specification: no missing/partial requirements,
> scope expansion or incorrectly implemented rules.
>
> **192 tests passed, 0 skipped.** Additional public-interface probes covered
> parser validation, named/anonymous NODE, orientation ties, `1e2000`, input
> protection and **30 oracle comparisons** across full int32 coordinates,
> tolerances, filters and batches.
>
> After SHA-256 validation, CLI comparisons reproduced all ordered pairs,
> orientations and both endpoint errors: **220 / 62 / 104 / 51**, without
> discrepancies. Hashes of 11 fixtures and four saved CSVs and completeness of
> 12 benchmark cases were checked. Spec findings: **0**.

The axes remain separate: Standards had one resolved P3 heuristic and no hard
violations; Spec had no findings. Publication changes have their own verification
record and review, rather than being represented as part of this earlier review.
