# Publication review — 2026-10-25

Two independent `mattpocock-skills:code-review` axes reviewed the publication
changes after the local implementation commit. The current English specification,
ADR 0001–0003 and the owner's publication requirements were authoritative.

Local evidence identifiers, retained on the development branch:

- Base: `c5120920ac8c890d087d7cf650ac7012d6317fd7`.
- Reviewed implementation: `243b3bb754bf1e7bcdc2ab9eedba8d8c26fd479b`.

These are local history labels, not public commit links. The public branch
starts with a clean root to keep old machine-path-containing commits local.

## Standards

Independent result: **0 documented-standard violations, 0 baseline heuristics**.
Module responsibilities, public seams, exact arithmetic, independent oracle
and streaming match AGENTS.md. Selection matches ADR 0003 and software/data
licenses are distinct. The oracle's separate selection logic is intentional
independence, not a duplicated-code abstraction opportunity.

The reviewer checked 26 input/report hashes, five original artifact hashes
against the base, all unchanged historical JSON fields, identity of demo and
historical dumps, all 84 reviewed tracked files (including binaries) for private
paths, 34 local Markdown links and all 24 new benchmark records/policies.
The review did not repeat tests or tool-enforced style checks.

## Spec

Independent result: **0 confirmed findings**: no missing/partial requirements,
incorrect behaviour or unrequested scope.

The reviewer independently executed **246 tests, 0 skipped**; all eight complete
registry CLI report comparisons; and **180 additional rational/Python-integer
oracle comparisons** across full int32 coordinates, thresholds, flags and batches.
Pair IDs/order/orientation and both endpoint errors agreed. Input/license/report
hashes and unchanged historical fields were checked. Text and binary files had
no private home-directory paths. An isolated archive build confirmed the sdist
includes the full demo, AGENTS.md, specification, provenance and data license,
and the wheel includes MIT/NOTICE. The reviewer inspected all 24 benchmark
records without rerunning the benchmarks.

## Clean-copy installation and demo

The primary agent exported the reviewed commit into a new temporary directory,
created a fresh standard Python virtual environment with pip, installed pinned
dependencies and built/installed the project with pip. The clean export passed
**246 tests, 0 skipped** (10.57 s), strict mypy and Ruff checks.

From outside that source directory, the installed package imported from
site-packages and produced byte-identical saved reports for the text default
(218 pairs), binary without SPLAY (60 pairs) and explicit acknowledged-duplicate
audit (220 pairs). The example and all tests therefore work without a sibling
registry checkout or contributor-specific paths.

Public-history auditing and remote verification are the final release operations;
they are separate from the reviewed code and data above.
