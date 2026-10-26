# ADR 0005: Retire active Python code after Go qualification

Date: 2026-10-26. Status: accepted. This supersedes ADR 0004 only where it
requires keeping the Python implementation and its dependencies in the active tree.

## Context

The owner requested repository cleanup after the complete Go migration and
asked whether Python could be removed. The migration already recorded 160
byte-identical CSV comparisons and 320 ordered exact-result comparisons against
the Python implementation. The independent Go exhaustive oracle does not use
the production predicate, threshold helper or spatial index.

Migration commit `b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1` passed
[GitHub Actions run 40376871944](https://github.com/dlubom/survex-duplicates/actions/runs/40376871944):
Go tests and extracted packages on all six native targets, Python-reference
checks on three interpreter versions, and complete package assembly. This
establishes the baseline before retirement; later changes need their own checks.

## Decision

Keep one active implementation and toolchain: Go. Remove the old Python
detector, entry points, tests, dependency/package manifests and migration-only
comparison probe. Port active publication logic and its negative tests to Go.
Ordinary development, validation, packaging and publication must not require
Python, NumPy, SciPy or uv.

Preserve the independent Go oracle, a 512-leg registry sample check, real
Survex fixtures, and full CSV comparisons for all eight registry settings.
Verify snapshot hashes before using saved expectations. Binary integration
continues to require external Survex; the text workflow remains standalone.
The new Go sample records its own deterministic selection; Go and Python
random-number generators do not imply identical samples from the same seed.

Keep historical numerical results, CSVs, source models, dumps, provenance and
license notices unchanged. Historical experiment source scripts, the complete
old Python program, lockfile, tests and benchmark harness are
recoverable from immutable commit
[`b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1`](https://github.com/dlubom/survex-duplicates/tree/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1).
Reproduction uses a separate checkout of that commit, as described in
[the benchmark guide](https://github.com/dlubom/survex-duplicates/blob/main/benchmarks/README.md#reproduce-the-archived-python-reference).

## Consequences

The active tree no longer has two implementations or two package versions to
maintain. Current correctness checks stay independent of production search and
compare full results, not counts alone. Publication keeps its complete-asset,
tag-identity and independent-download gates after the language change.

Future Go changes do not automatically rerun the former language comparison.
When that extra check is needed, replay the pinned archived reference and save
new evidence separately. Never refresh historical hashes or overwrite original
measurements merely to make a changed implementation pass.
