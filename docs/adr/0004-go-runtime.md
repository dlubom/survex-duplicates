# ADR 0004: Go runtime with an executable Python reference

Date: 2026-10-26. Status: accepted. The implementation was promoted after
the recorded comparison demonstrated result parity and improved performance
on the measured workloads. Publication remains a separate operation.

The active Python-reference retention decision below is superseded by
[ADR 0005](0005-retire-python-reference.md). The Go runtime decision and
recorded migration evidence remain valid; the old executable reference is
preserved in Git at the fixed pre-cleanup commit.

## Context

The owner requested a Go migration and an investigation of speed and identical
results, then explicitly included the complete transition in the authorized
scope. Python/SciPy already provides a native spatial index; language alone is
not evidence of faster complete runs. Overflow and CSV decimal formatting are
particularly important compatibility constraints.

## Decision

Use Go 1.26 and its standard library for the primary command; release and CI
builds pin Go 1.26.3. Preserve the
reader, geometry, search, report and CLI boundaries. Use arbitrary-precision
integer/rational arithmetic for qualification and exact decimal arithmetic for
the existing report representation. Retain the orientation-independent six-axis
maximum-norm index, exact query radii and ordered streaming output.

Keep `survex_duplicates/`, its tests and locked Python environment as the
executable reference from commit `1143a96`. They are development dependencies,
not requirements for running the Go executable. Keep the independent exhaustive
oracle separate from both implementations.

The migration does not change input data, filters, IDs, tolerances, CSV columns,
station attribution, source dates or the interpretation of geometric candidates.
Existing historical reports and data-license notices remain intact.

## Evidence and consequences

`experiments/go-2026-10-26/` records the comparison method, measurements and
limits. The archived
[`benchmarks/compare_go.py`](https://github.com/dlubom/survex-duplicates/blob/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1/benchmarks/compare_go.py)
reproduces exact tuple, metadata and CSV-byte checks, followed by alternating fresh-process timing runs. Registry/fixture
hashes must match provenance before expectations are used.

The runtime is one executable with no NumPy/SciPy installation. `dump3d` remains
necessary for binary input. The standard-library KDTree has O(n log² n) build
cost; dense output still requires quadratic work. Measured gains do not imply a
universal speed guarantee or a claim that the index build alone is faster.

The release configuration prepares native ZIPs for Linux, macOS and Windows,
each on amd64 and arm64, with the full bundled demo. See the
[platform guide](../platforms.md) and [release guide](../releasing.md).
Preparing this migration does not itself publish an upstream release.
Cross-compilation validates buildability only; execution checks are required
on each of the six target systems before release publication.
