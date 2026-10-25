# Verification record

## Initial implementation — 2026-10-25

The initial implementation used the approved specification, glossary and ADRs.
It was developed locally with vertical TDD at the four agreed seams and reviewed
independently for Standards and Spec. See [code-review.md](code-review.md).
Historical local commits remain on the development branch; publication starts
with a separate clean root so machine-specific paths are absent from its history.

Executed checks after the initial review correction:

- **192 passed, 0 skipped**, 4.84 seconds; strict mypy for nine production modules;
  Ruff lint/import/format checks.
- Real Survex 1.4.24 fixtures: v8 binary and both geometry dumps, v3 without STYLE,
  ERROR_INFO, dated XSECT, anonymous names and extended elevation rejection.
- Exact boundaries, orientation/ties, point legs, cm-grid validation, full int32
  safety, translation and non-transitive matching.
- Independent Python-integer oracle, complete pairs/orientations/squared errors,
  fixed-seed sets and a 512-leg registry sample, both tolerances and selections.
- Full CLI arguments, ordered 47-column CSV, Unicode/bracket/quote preservation,
  IDs, names/statuses and deterministic batches.
- Atomic output with fsync/replace/link failures, preservation of prior reports,
  temporary cleanup and input protection through paths, symlinks and hardlinks.
- dump3d arguments/locale, two-megabyte stderr, missing executable/options and
  failure after valid partial stdout.
- sdist/wheel builds using setuptools 84.2.1, editable installation and a wheel
  installed in a new temporary environment, executed outside the source checkout.
- Twelve measured benchmark cases and full registry comparisons against the
  original exhaustive 172,673,236-pair experiment.

The original all-flags audit produced 220/62 pairs at 5% and 104/51 at 0.05 m.
It remains available through `--include-duplicates`. Original benchmark numbers
are retained in `benchmarks/results.json`, explicitly labelled with that policy.

## Publication preparation and DUPLICATE policy

The owner selected MIT for the software and `dlubom/survex-duplicates` as the
publication destination. The repository now includes English documentation,
English diagnostics, AGENTS.md and the full registry demo. Raw station/cave
names and intentional Unicode fixture data remain unchanged.

DUPLICATE legs are excluded by default, per [ADR 0003](adr/0003-exclude-acknowledged-duplicates.md).
SPLAY exclusion remains independent; combined exclusions are counted once.
Tests cover both policies against the independent oracle and retain original IDs.
No input records are skipped during validation.

The complete bundled model is the portable integration default. Binary SHA-256
is checked before saved expectations and again after full verification:
`55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077`.
Model and dump bytes are unchanged. Historical metadata paths were redacted;
[publication-provenance.json](../experiments/tolerance-2026-10-25/publication-provenance.json)
preserves original artifact hashes and the scope of edits. Measurements,
geometry, pair IDs, orientations and errors are unchanged.

| Tolerance | Include DUPLICATE | Exclude SPLAY | n_excluded | n | C | K |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| 5% | no | no | 2 | 18582 | 292 | 218 |
| 5% | no | yes | 6433 | 12151 | 63 | 60 |
| 5% | yes | no | 0 | 18584 | 294 | 220 |
| 5% | yes | yes | 6431 | 12153 | 65 | 62 |
| 0.05 m | no | no | 2 | 18582 | 125 | 103 |
| 0.05 m | no | yes | 6433 | 12151 | 50 | 50 |
| 0.05 m | yes | no | 0 | 18584 | 126 | 104 |
| 0.05 m | yes | yes | 6431 | 12153 | 51 | 51 |

Every row represents a full comparison of ordered pairs, orientation and both
endpoint errors, not just counts. The six Wysoka pairs remain. The two deliberately
acknowledged Mietusia pairs appear only in the explicit audit.
See [the eight saved reports and hashes](../examples/kataster/results/verification.json).

Publication measurements comprise twelve default-policy runs and twelve explicit
audit runs on sparse/repeated 50k/100k inputs and dense 400-leg data. Every output
is checked against the generator's full expected pairs and orientations.
See [benchmarks/README.md](../benchmarks/README.md) for the measured tables.

## Environment and limits

Python 3.12.14, NumPy 2.3.5, SciPy 1.16.3, pytest 9.1.1, mypy 1.20.2,
Ruff 0.17.2, Survex 1.4.24; macOS 27.0.1 arm64, Apple M4, Mac16,10,
ten logical cores and 16 GiB RAM. `requirements-tested.txt` records exact
local development dependencies. The initial package build used setuptools 84.2.1.

The tests were executed with real Survex available. Without it, binary integration
checks are skipped explicitly; ordinary parser, geometry, search and CLI tests
use saved dumps or stand-in external processes.

Not independently tested: other dependency/runtime versions, Windows/Linux,
all historical 3D versions, network filesystems, power loss and SIGKILL.
No second full new-Python-oracle enumeration of 172.7 million pairs was performed;
the full comparison uses preserved exhaustive evidence, with new seeded and
512-leg oracle checks. fsync covers the file, not a guarantee of directory-entry
durability after a system crash. KDTree has no claimed O(n+K) guarantee here.

## Publication checks executed

- Full current suite: **246 passed, 0 skipped**, 6.55 seconds. Strict mypy
  checks all nine production modules; Ruff lint and formatting pass.
- Eight complete registry comparisons passed after historical metadata redaction,
  including full pairs, orientation and both endpoint errors.
- The historical full NumPy experiment was rerun into a separate reports directory:
  all 172,673,236-pair rule counts, saved leg data and complete pair records agree
  with the preserved evidence. The independent standard-library sample again
  checked 130,816 pairs and 67 metadata records with no discrepancies.
- Original artifact hashes in publication provenance were checked against the
  preserved local implementation commit. All non-path experimental result fields
  remain equal to their original values. Every fixture and demo input matches
  its recorded hash.
- The publication file tree contains no contributor home-directory paths.
  Binary data and text were scanned; saved metadata now uses portable paths.
- Wheel and source distribution build successfully. License notices are in the
  wheel; AGENTS.md, specification, full demo, provenance and data license are
  present in the source distribution.

The independent publication review found **0 Standards violations/heuristics
and 0 Spec findings**. A fresh exported copy passed **246 tests, 0 skipped**,
mypy and Ruff after a normal pip package installation. Installed CLI execution
outside the source directory produced byte-identical demo reports.
Details: [publication-review.md](publication-review.md).
