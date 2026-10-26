# Reproducible performance measurements

Publication measurements were made on Apple M4 (10 logical cores), Mac16,10, 16 GiB RAM, macOS 27.0.1 arm64. Python 3.12.14, NumPy 2.3.5, SciPy 1.16.3. Each case runs sequentially in a fresh process.

```bash
uv sync --locked
mkdir -p reports
uv run --locked python -m benchmarks.run --out reports/benchmark-default.json
uv run --locked python -m benchmarks.run --include-duplicates --out reports/benchmark-audit.json
```

New measurements are saved separately in the ignored `reports/` directory.
The publication and historical results below remain unchanged.

Seed 20261025; batch size 1. Inputs are generated as full text dumps, then read by the production reader. Search/write time includes CSV formatting, flush, fsync and atomic publication. Peak RSS is ru_maxrss for the entire fresh process, including imports, generation, reading and verification. macOS reports bytes; Linux KiB is converted to bytes. Measurements are single observations, not promised runtime limits.

## Generator and expected results

[datasets.py](datasets.py) uses cells separated by 100 metres. Lengths are 4, 20, 100, 1000 or 3000 cm, sometimes on a diagonal, so relative query radii differ. Separate cells cannot match at the tested thresholds. Sparse inputs have no pairs. Repeated inputs contain two oppositely oriented legs per cell: every fourth pair is shifted by (30,40,0) cm with length at least 30 m; the others coincide. Geometry therefore accepts all at 5%, while absolute 5 cm rejects the shifted pairs.

The second leg of every tenth repeated cell is SPLAY; all other second legs are DUPLICATE. This deliberately exercises acknowledged-repeat exclusion: normal selection omits the DUPLICATE copies, and excluding SPLAY removes the remaining copies too. Explicit audit selection keeps the full repeated workload. Sparse/dense inputs mark every tenth leg SPLAY. The dense case contains 400 identical legs with alternating orientation and 79,800 geometric pairs.

Each complete CSV is read back and compared, in order, with all pair IDs and orientations implied independently by the construction. Smaller 80-leg versions of every dataset, both tolerance modes and all filter settings are also checked against the independent exhaustive oracle in tests/test_search.py. Production code does not retain a global result list.

## Publication results

Relative tolerance is 5%; absolute tolerance is 0.05 m. Times are seconds, memory is MiB. C counts unordered candidates after the larger-ID condition. All 24 cases completed their full pair/orientation check.

### Default selection: acknowledged duplicates excluded

Raw measurements: [publication-results.json](publication-results.json).

| Data | n_input | Exclude SPLAY | n_excluded | n | Mode | C | K | Read | Build | Search + CSV | RSS |
| --- | ---: | --- | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| sparse | 50000 | no | 0 | 50000 | 5% | 0 | 0 | 0.707 | 0.049 | 0.152 | 100.9 |
| sparse | 50000 | no | 0 | 50000 | 5 cm | 0 | 0 | 0.714 | 0.049 | 0.150 | 100.8 |
| repeated | 50000 | no | 22500 | 27500 | 5% | 2500 | 2500 | 0.719 | 0.028 | 0.141 | 98.1 |
| repeated | 50000 | no | 22500 | 27500 | 5 cm | 1250 | 1250 | 0.725 | 0.027 | 0.109 | 97.8 |
| sparse | 100000 | no | 0 | 100000 | 5% | 0 | 0 | 1.456 | 0.099 | 0.308 | 145.5 |
| sparse | 100000 | no | 0 | 100000 | 5 cm | 0 | 0 | 1.442 | 0.099 | 0.303 | 145.4 |
| repeated | 100000 | no | 45000 | 55000 | 5% | 5000 | 5000 | 1.482 | 0.057 | 0.289 | 139.4 |
| repeated | 100000 | no | 45000 | 55000 | 5 cm | 2500 | 2500 | 1.480 | 0.056 | 0.229 | 139.4 |
| dense | 400 | no | 0 | 400 | 5% | 79800 | 79800 | 0.006 | 0.000 | 1.525 | 55.9 |
| dense | 400 | no | 0 | 400 | 5 cm | 79800 | 79800 | 0.006 | 0.000 | 1.415 | 55.7 |
| repeated | 100000 | yes | 50000 | 50000 | 5% | 0 | 0 | 1.515 | 0.054 | 0.163 | 138.9 |
| repeated | 100000 | yes | 50000 | 50000 | 5 cm | 0 | 0 | 1.518 | 0.052 | 0.153 | 138.8 |

### Explicit audit: acknowledged duplicates included

Raw measurements: [audit-results.json](audit-results.json).

| Data | n_input | Exclude SPLAY | n_excluded | n | Mode | C | K | Read | Build | Search + CSV | RSS |
| --- | ---: | --- | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| sparse | 50000 | no | 0 | 50000 | 5% | 0 | 0 | 0.757 | 0.049 | 0.153 | 100.7 |
| sparse | 50000 | no | 0 | 50000 | 5 cm | 0 | 0 | 0.730 | 0.049 | 0.154 | 100.9 |
| repeated | 50000 | no | 0 | 50000 | 5% | 25000 | 25000 | 0.735 | 0.048 | 0.714 | 102.8 |
| repeated | 50000 | no | 0 | 50000 | 5 cm | 18750 | 18750 | 0.739 | 0.048 | 0.531 | 102.9 |
| sparse | 100000 | no | 0 | 100000 | 5% | 0 | 0 | 1.479 | 0.101 | 0.315 | 145.5 |
| sparse | 100000 | no | 0 | 100000 | 5 cm | 0 | 0 | 1.477 | 0.101 | 0.312 | 145.5 |
| repeated | 100000 | no | 0 | 100000 | 5% | 50000 | 50000 | 1.492 | 0.100 | 1.423 | 149.8 |
| repeated | 100000 | no | 0 | 100000 | 5 cm | 37500 | 37500 | 1.487 | 0.100 | 1.076 | 149.5 |
| dense | 400 | no | 0 | 400 | 5% | 79800 | 79800 | 0.006 | 0.000 | 1.527 | 55.8 |
| dense | 400 | no | 0 | 400 | 5 cm | 79800 | 79800 | 0.006 | 0.000 | 1.383 | 55.7 |
| repeated | 100000 | yes | 5000 | 95000 | 5% | 45000 | 45000 | 1.497 | 0.096 | 1.299 | 148.8 |
| repeated | 100000 | yes | 5000 | 95000 | 5 cm | 35000 | 35000 | 1.498 | 0.097 | 1.008 | 148.8 |

## Historical evidence and registry

[results.json](results.json) preserves the initial implementation's twelve measurements. Those runs included all DUPLICATE legs; this setting is now explicitly labelled in metadata. The initial unrecorded run failed while querying hardware metadata after all cases ran; only its successful full rerun was saved. Publication measurements above are separate later runs after the new selection policy.

[The current registry verification](../examples/kataster/results/verification.json) records hashes before/after analysis and full pair/orientation/endpoint-error comparisons for eight settings. At 5%, the normal report has C=292/K=218, or C=63/K=60 without SPLAY; explicit audit has C=294/K=220 or C=65/K=62. Historical all-flags CSVs remain in [kataster-results](kataster-results/verification.json).

## Interpretation limits

These datasets demonstrate the measured workloads, not general guarantees. KDTree has no claimed O(n+K) guarantee. Dense output may have n(n−1)/2 pairs and quadratic time/disk size. Batch B may add O(B*n) neighbour-list memory; all recorded runs use batch 1. Other platforms, library versions and concurrent workloads were not measured.
