# Go migration: parity and performance investigation

Date: 2026-10-26. Reference: Python implementation at `1143a96292d49457ea344d46410072ecbbcec1af`.
Go became the primary runtime under ADR 0004. At the time of this investigation,
the Python source remained unchanged as an executable verification reference.
It was subsequently retired from the active tree under
[ADR 0005](../../docs/adr/0005-retire-python-reference.md). This report preserves
the original measurements and verification boundaries; its tools remain
available at immutable commit
[`b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1`](https://github.com/dlubom/survex-duplicates/tree/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1).

## Result

All 160 complete CSV comparisons were byte-identical, and all 320 ordered exact-tuple comparisons passed, including both squared endpoint errors. Go was faster in all 23 measured scenarios: **6.02–14.73 times** for the complete process on this machine. These are measured workloads, not a guarantee for arbitrary inputs.

For the default registry dump, the median dropped from **0.6335 s to 0.0492 s (12.89×)**. Median process peak RSS dropped from **84.1 MiB to 25.5 MiB**.

| Scenario (5%) | Analysed legs | Pairs | Python median, s | Go median, s | Speedup | Python/Go RSS, MiB |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Registry dump | 18582 | 218 | 0.6335 | 0.0492 | 12.89× | 84.1 / 25.5 |
| Registry dump without SPLAY | 12151 | 60 | 0.6046 | 0.0411 | 14.70× | 82.4 / 22.8 |
| 100k repeated, full audit | 100000 | 50000 | 3.1711 | 0.3941 | 8.05× | 150.4 / 74.2 |
| 400 identical legs | 400 | 79800 | 1.6895 | 0.2807 | 6.02× | 56.1 / 12.2 |
| Registry binary | 18582 | 218 | 0.6514 | 0.0537 | 12.13× | 84.1 / 25.3 |

The large-output dense case still improves by 6.02×. The benefit is therefore not limited to interpreter startup. Index construction alone is not the main gain: on the default registry, its median is 0.0180 s in Python and 0.0155 s in Go. Faster parsing and report/search processing account for much of the difference.

## Stage measurements

These are medians of each stage separately; their sum need not equal the median whole-process time. Whole-process time also includes startup, imports, process exit and other overhead.

| Scenario | Language | Read, s | Build, s | Search + CSV, s | Whole process, s |
| --- | --- | ---: | ---: | ---: | ---: |
| Registry dump | python | 0.4209 | 0.0180 | 0.0608 | 0.6335 |
| Registry dump | go | 0.0212 | 0.0155 | 0.0082 | 0.0492 |
| 100k repeated, full audit | python | 1.4931 | 0.1006 | 1.4400 | 3.1711 |
| 100k repeated, full audit | go | 0.0658 | 0.0851 | 0.2364 | 0.3941 |
| 400 identical legs | python | 0.0058 | 0.0004 | 1.5502 | 1.6895 |
| 400 identical legs | go | 0.0002 | 0.0001 | 0.2761 | 0.2807 |

## Correctness evidence

- Registry and fixture SHA-256 values were checked against provenance before use and again after the run; all remained unchanged.
- All eight tolerance/filter settings were checked on both the saved registry dump and real binary input. Complete ordered `(leg_a, leg_b, reversed, endpoint_1_sq_cm, endpoint_2_sq_cm)` tuples were compared at batch sizes 1 and 19.
- The reader comparison includes ordered geometry, original IDs/line numbers, flags, styles, dates, header fields, all station names and anonymous/missing-node status.
- Real LEG, MOVE/LINE, legacy and closure fixtures were included. Seeded data includes int32 extremes, reversals, points, ties, bracket/quote/Unicode metadata, sub-centimetre tolerances and `1e1000`.
- The independent exhaustive Python-integer oracle checked seeded data, fixtures and the 512-leg registry sample. The full registry additionally passed all eight historical exhaustive-result comparisons. No new exhaustive enumeration of all 172.7 million registry pairs is claimed.
- Full 47-column CSV files were compared as bytes, preserving CRLF, JSON names, float formatting and decimal tolerance representation.
- Every timed Python/Go output pair also had identical SHA-256. Synthetic benchmark outputs were read back and checked against the independent generator construction.
- Go: 114 tests/subtests passed; the final race detector and vet passed. Python reference: 246 tests passed with zero skips, plus mypy and Ruff. The separately invoked test probe is intentionally skipped by ordinary `go test`.
- The native executable was run outside the repository with empty PATH, reading the text snapshot. Its 218-row, 47-column CSV matched the verified default report hash.

## Measurement method

Environment: macOS 27.0.1 arm64, 10 logical CPUs, Go 1.26.3, Python 3.12.14, NumPy 2.3.5, SciPy 1.16.3, Survex 1.4.24. No CPU model or RAM capacity was inferred from previous reports.

Each scenario ran once per language as an excluded warm-up, followed by five measured runs per language. Order alternated Python→Go and Go→Python; each run used a fresh process. Filesystem caches were warm. Go compilation, synthetic input generation and verification/readback were outside the timed region. Python was invoked directly using the locked environment interpreter, so no uv startup penalty was included.

Timing uses a parent monotonic clock from process launch through `wait4`. Per-stage timings come from each implementation. Peak RSS is the entire child process `ru_maxrss` (bytes on macOS); it is not isolated algorithm memory. Search timing includes streamed CSV formatting, flushing, file fsync and publication. Both languages used batch size 1 and the same saved input bytes, with no concurrent test/build workloads during the final measurements.

The Go implementation uses the standard library and exact `math/big` arithmetic, without external Go modules. Its median-partition KDTree build is O(n log² n). It retains current-batch neighbours and streams output; dense result size remains quadratic.

Initial benchmark and build attempts encountered `ENOSPC`. Task-owned caches and scratch outputs were cleaned. The harness now keeps one timed CSV and one generated benchmark input at a time, deleting them only after hashing/readback. The complete final series starts over; preliminary partial timing values are not reused.

## Full timing table

All values are medians of five recorded runs. Raw individual runs, counters, stage times, input/output hashes and measured source fingerprints are in [results.json](results.json).

| Data | Mode | Exclude SPLAY | Include DUPLICATE | n | C | K | Python, s | Go, s | Speedup |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| registry | 5% | no | no | 18582 | 292 | 218 | 0.6335 | 0.0492 | 12.89× |
| registry | 5% | no | yes | 18584 | 294 | 220 | 0.6327 | 0.0504 | 12.56× |
| registry | 5% | yes | no | 12151 | 63 | 60 | 0.6046 | 0.0411 | 14.70× |
| registry | 5% | yes | yes | 12153 | 65 | 62 | 0.6004 | 0.0416 | 14.44× |
| registry | 0.05 m | no | no | 18582 | 125 | 103 | 0.6318 | 0.0475 | 13.29× |
| registry | 0.05 m | no | yes | 18584 | 126 | 104 | 0.6682 | 0.0477 | 14.00× |
| registry | 0.05 m | yes | no | 12151 | 50 | 50 | 0.6476 | 0.0462 | 14.00× |
| registry | 0.05 m | yes | yes | 12153 | 51 | 51 | 0.6195 | 0.0421 | 14.73× |
| sparse-50000 | 5% | no | no | 50000 | 0 | 0 | 1.0527 | 0.0844 | 12.48× |
| sparse-50000 | 0.05 m | no | no | 50000 | 0 | 0 | 1.0609 | 0.0848 | 12.52× |
| sparse-100000 | 5% | no | no | 100000 | 0 | 0 | 2.1039 | 0.1978 | 10.63× |
| sparse-100000 | 0.05 m | no | no | 100000 | 0 | 0 | 2.0238 | 0.1758 | 11.51× |
| repeated-50000 | 5% | no | no | 27500 | 2500 | 2500 | 1.0400 | 0.0771 | 13.49× |
| repeated-50000 | 5% | no | yes | 50000 | 25000 | 25000 | 1.6364 | 0.1919 | 8.53× |
| repeated-50000 | 0.05 m | no | no | 27500 | 1250 | 1250 | 1.0079 | 0.0690 | 14.61× |
| repeated-50000 | 0.05 m | no | yes | 50000 | 18750 | 18750 | 1.4576 | 0.1442 | 10.11× |
| repeated-100000 | 5% | no | no | 55000 | 5000 | 5000 | 1.9684 | 0.1522 | 12.93× |
| repeated-100000 | 5% | no | yes | 100000 | 50000 | 50000 | 3.1711 | 0.3941 | 8.05× |
| repeated-100000 | 0.05 m | no | no | 55000 | 2500 | 2500 | 1.9017 | 0.1342 | 14.17× |
| repeated-100000 | 0.05 m | no | yes | 100000 | 37500 | 37500 | 2.7995 | 0.2973 | 9.42× |
| dense-400 | 5% | no | no | 400 | 79800 | 79800 | 1.6895 | 0.2807 | 6.02× |
| dense-400 | 0.05 m | no | no | 400 | 79800 | 79800 | 1.5391 | 0.2269 | 6.78× |
| registry-binary | 5% | no | no | 18582 | 292 | 218 | 0.6514 | 0.0537 | 12.13× |

## Reproduce

First create the [archived reference checkout](../../benchmarks/README.md#reproduce-the-archived-python-reference),
then run from its root. Benchmark measurement with peak RSS requires Unix;
`--skip-benchmark` leaves the parity checks available independently.

```bash
uv sync --locked
go test ./...
go test -race ./...
go vet ./...
go build -o bin/survex-duplicates .
go test -c -o bin/differential-probe .
uv run --locked python -m benchmarks.compare_go --go-bin bin/survex-duplicates --probe-bin bin/differential-probe --out-dir reports/go-comparison --repeats 5
```

New runs belong under `reports/`; do not overwrite this measurement snapshot. The synthetic generator is [the archived benchmarks/datasets.py](https://github.com/dlubom/survex-duplicates/blob/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1/benchmarks/datasets.py) with seed 20261025. Each raw result identifies its input hash, tolerance and filters. [validation.json](validation.json) records executed checks and their limits.

## Limits and portability

At the time of this record, Linux amd64 and Windows amd64 cross-builds succeeded,
but those binaries were not executed locally. CI was configured to run Go tests
on all three operating systems and the full differential comparison on Linux;
the remote jobs had not run for that local change. Later native CI evidence is
recorded in [the platform guide](../../docs/platforms.md). No network-filesystem,
power-loss or SIGKILL guarantee is claimed. Go build time and the size/cost of
installing Go are not included in runtime comparisons.

Five repetitions on one machine support this migration decision; they do not establish a general performance bound. Individual timings and their spread can be inspected in the raw JSON.

## Sources and licensing

The executable Python reference and independent oracle are the primary comparison sources. The implementation also consulted the official [math/big](https://pkg.go.dev/math/big), [strconv](https://pkg.go.dev/strconv) and [encoding/csv](https://pkg.go.dev/encoding/csv) documentation; standard Go CSV quoting differs from the reference in some cases, so the report writer explicitly preserves the reference format.

Software remains MIT. Registry-derived verification and measurement evidence retains attribution to Tatra Cave Registry contributors under CC BY-SA 4.0; see [NOTICE.md](../../NOTICE.md). No registry or fixture input was edited.
