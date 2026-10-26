# Go migration implementation plan

**Goal:** Measure whether Go is faster with identical results, then make Go the
primary implementation if both are demonstrated. The owner explicitly requested
the full transition after the comparative investigation.

**Architecture:** Preserve the reader, exact predicate, ordered streaming search,
report and CLI responsibilities as separate Go files. Keep the previous Python
implementation as a reproducible development reference. Preserve all source data,
historical reports and their provenance.

**Tech stack:** Go 1.26, standard library only; Survex `dump3d` for binary input.
Python 3.12 plus the existing locked NumPy/SciPy environment for differential
verification and measurement, never required to run the Go detector.

**Specification:** `docs/specification.md`, `docs/adr/0001-station-names-from-model.md`,
`docs/adr/0002-default-relative-tolerance.md`,
`docs/adr/0003-exclude-acknowledged-duplicates.md`, `GLOSSARY.md`.

## Design choices

- Full Go: removes Python runtime dependencies and startup cost; needs exact
  arithmetic, parser and formatting parity. Selected by the owner.
- Hybrid Go search/Python reader and writer: less migration work but retains
  Python startup, dependency management and potentially dominant parsing cost.
- Keep Python/SciPy: established reference, but does not answer the requested
  migration. Retain it for verification rather than as the default entry point.

The initial experimental directory provides a place to measure before promotion.
The final Go entry point is the repository root (`go run .`, `go build .`).
No publication, push, merge, release or registry-source change is in scope.

## Global constraints and review focus

- Qualify using exact centimetres and arbitrary precision where int64 is unsafe.
- Query component-wise min/max keys in six dimensions with the maximum norm.
- Preserve filters, IDs, ordering, both endpoint errors, ties and every CSV column.
- Validate excluded records; preserve NODE aliases, anonymity, dates and UTF-8.
- Preserve input and existing output on all failures, including subprocess errors.
- Retain only the current query batch; batch size cannot affect output.
- Verify fixture hashes before using saved expectations.
- New measurements are separate from historical evidence, with machine/tool
  versions, commands, input hashes, repeated runs and readback checks.

## Tasks and verification

- [x] Reader: `reader.go`, `reader_test.go`; `ParseDump(io.Reader)` and
  `ReadModel(path, format, dump3d)` return ordered geometry and metadata.
  Exercise real fixtures, malformed headers, dates, UTF-8, names containing
  brackets, int32 bounds, exact decimal parsing and failed binary subprocesses.
- [x] Geometry/search: `geometry.go`, `search.go` and tests;
  `MatchLegs` exposes both exact squared errors; `Search.Pairs` streams ordered
  pairs via a callback. Check boundaries, orientation, point legs, huge decimal
  tolerances, int32 extremes and completeness against independent arithmetic.
- [x] Report: `report.go`, `report_test.go`; `WriteReport` streams all 47 fields
  and atomically publishes; `ValidateDestination` rejects aliases of the input.
  Compare Python Decimal rounding, float formatting, JSON names and CSV bytes.
- [x] CLI: `cli.go`, `cli_test.go`, `main.go`; keep existing arguments, status
  codes and stderr summary; check options before reading input and errors before
  publication. Keep test-only inspection in a Go test probe.
- [x] Differential harness: compare exact ordered tuples and full CSV for all
  eight registry settings, two batch sizes, real fixtures, generated boundary
  cases and seeded cases. Use `tests/oracle.py` independently for small inputs.
- [x] Benchmark: identical saved inputs, fresh processes, alternating language
  order, warm-up excluded, repeated timed runs; include startup, read, index,
  search plus synced CSV and peak RSS. Verify result files outside timed section.
- [x] Promotion: root Go module, updated README/specification/ADR/CI, retain Python
  reference and historical validation commands with their role made explicit.
- [x] Final validation: Go tests and race detector, vet, formatting, cross-builds,
  Python reference tests/types/lint, full registry verification, differential
  readback and fresh sequential benchmark. Review final diff independently.
