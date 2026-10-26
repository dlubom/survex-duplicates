# Development and quality checks

Run commands from the repository root using the version in `.go-version`.
The runtime, tests and release tools use the Go standard library. Git is also
required by package tests, which build a real archive in a temporary checkout.
Install Survex for real binary-input integration; missing `dump3d` is reported
as a test skip. Saved dumps and the bundled registry need no external checkout.

## Fast checks

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
go build ./...
go run ./cmd/release matrix
```

Formatting passes only when `gofmt -l` prints nothing. Windows ARM64 omits the
race check because the pinned Go toolchain does not support it. All six native
CI targets run the ordinary suite; Linux additionally installs real Survex.

Tests follow the four public seams in [AGENTS.md](../AGENTS.md): reader,
geometry predicate, ordered search iterator and CLI. They compare complete
pairs, orientation, both exact endpoint errors and all eight saved registry
CSVs. The rational exhaustive oracle remains independent of the production
predicate and spatial index. File-operation failures use a private dependency
passed to each report invocation, with real temporary files and observable
CLI status/diagnostics. There are no mutable global fault hooks.

## Static analysis and known vulnerabilities

Install the pinned development tools separately from the application:

```bash
go install honnef.co/go/tools/cmd/staticcheck@v0.7.0
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
staticcheck ./...
govulncheck ./...
```

The executables are installed in `GOBIN`, or `$(go env GOPATH)/bin` when unset;
include that directory in `PATH`. Staticcheck v0.7.0 is the
[2026.1 release](https://github.com/dominikh/go-tools/releases/tag/2026.1),
which supports Go 1.26. [Govulncheck](https://go.dev/doc/security/vuln/)
consults the current vulnerability database and fails for affected calls
reachable from the application. Its output may also list vulnerabilities in
imported packages whose affected functions are not called. A passing scan is
not a claim that every possible vulnerability is known or ruled out.

For workflow edits, run actionlint 1.7.12. The workflow pins its download and
checksum. Tool version changes should be deliberate and tested.

## Coverage

With Bash available (Git Bash on Windows):

```bash
bash scripts/coverage.sh
```

This runs the complete unit suite, then builds an instrumented release command
and exercises `matrix`, native snapshot `build`, `verify` and extracted `smoke`.
The smoke check compares complete 218-pair and 60-pair CSVs. Do not edit source
files during this command: package fingerprints must describe one snapshot.
The temporary snapshot archive is deleted after the run.

Results in ignored `reports/coverage/` include `unit.out`, `integration.out`,
the statement-block union `combined.out`, per-package `packages.txt`,
per-function `functions.txt` and browsable `index.html`. The union counts a
statement as covered if either profile executed it; percentages are never
added. Keep the profiles separate when diagnosing gaps. Coverage identifies
unexecuted code; it does not prove assertion strength. There is no arbitrary
global percentage gate.

## Curated mutation regressions

```bash
go run ./cmd/mutate
```

The reviewed cases in [quality/mutations.json](../quality/mutations.json)
perturb geometry boundaries, orientation, filters, parser metadata, CSV safety
and archive verification. The runner copies current tracked and nonignored
untracked files to an isolated temporary Git repository, checks a passing
baseline, then runs the whole suite for each single mutation. The original
checkout is never mutated. Results and individual logs are saved under ignored
`reports/mutations/`.

A mutation is killed only by a failing test. Build errors, stale or ambiguous
replacement anchors and timeouts are reported separately and fail the check;
they do not inflate the score. Surviving mutations fail the check too. This is
a small regression suite of deliberately selected faults, not a percentage of
all possible mutations. Update its exact source anchors when refactoring and
check that each mutant still represents the intended fault.

## Fuzzing and benchmarks

The ordinary suite executes the fuzz seeds. Run bounded native campaigns with:

```bash
go test -run '^$' -fuzz '^FuzzParseDump$' -fuzztime=30s -parallel=2 .
go test -run '^$' -fuzz '^FuzzSearchOracle$' -fuzztime=30s -parallel=2 .
go test -run '^$' -bench . -benchmem -benchtime=100ms -count=1 .
```

Parser seeds include hash-verified real fixtures. Search inputs contain at most
ten legs and compare complete results to the rational oracle across tolerance,
filter and batch settings, plus translation, reversal and permutation checks.
These workload bounds apply to fuzzing, not to application input. Preserve any
failure corpus emitted under `testdata/fuzz/`, reduce it to an understandable
regression and fix the cause before accepting the result. See
[Go fuzzing](https://go.dev/doc/security/fuzz/) for longer campaigns.

Benchmarks separate parsing, index construction and streaming search, report
allocations and verify expected workload results before timing. See the
[benchmark guide](../benchmarks/README.md#current-go-benchmarks) for construction
and interpretation. CI saves measurements without enforcing a wall-clock
threshold; compare repeated runs on an otherwise idle, consistent host.

## CI artifacts and release checks

The reusable checks workflow adds Linux analysis, coverage, bounded fuzzing,
benchmarks and mutations to the existing six native checks. Artifacts are
named `quality-reports` and `mutation-reports`; failing fuzz corpora are included
when present. Release assembly waits for all checks and all native package
smokes. Publication tests use an offline `gh` stub and never publish anything.

Follow the [release guide](releasing.md) for six-archive verification. Cross-built
archive verification does not establish native execution on another platform.
Record executed checks and skips in [verification history](verification.md),
keeping historical inputs and measurements unchanged.
