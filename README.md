# Survex Duplicate Detector

Find geometric duplicate candidates in a Survex `.3d` model or its complete
`dump3d` text dump. Every reported pair matches at **both endpoints**, allowing
reversed direction. The default tolerance is **5% of the shorter leg**.

The detector writes a CSV with station names, survey metadata, coordinates and
both endpoint errors. It preserves the input. A match is a candidate for review,
not proof that a measurement should be removed.

## Get the executable

The primary implementation is **Go**. The six native packages contain a standalone
executable that requires neither Go nor Python. A saved text dump also needs
no Survex installation; binary `.3d` input requires Survex's `dump3d`.

Binary archives are prepared by the release workflow; this Go transition does
not itself publish a release. When a version is published, download the archive
for your system from [GitHub Releases](https://github.com/dlubom/survex-duplicates/releases)
and extract it. See [platforms and archive contents](docs/platforms.md).
Until a suitable archive is available, [build from source](#build-from-source).

## Try the complete Tatra Cave Registry

Each application archive includes the complete **18,584-leg** KATASTER model,
its text dump and eight verified CSV reports. From the extracted directory,
run these commands with the native executable on macOS or Linux:

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

On Windows, use PowerShell:

```powershell
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --out duplicates.csv
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

The first command produces **218 pairs**; excluding SPLAY leaves **60**.
Legs marked `DUPLICATE` are excluded by default because the flag acknowledges
intentional repeat surveying. Add `--include-duplicates` for a full audit:
**220 pairs**, or **62** with `--exclude-splays`.

Open the CSV as UTF-8 with a comma delimiter. Review the two leg IDs, station
names, `reversed`, and **both** endpoint errors together. The
[usage guide](docs/usage.md) explains the columns, tolerance choices, binary
input and real examples. The [demo guide](examples/kataster/README.md) links
all eight expected reports and records their provenance and data license.
The [documentation index](docs/index.md) separates usage, development, releases
and historical evidence.

## Use your own model

```bash
./survex-duplicates "my cave.3d" --exclude-splays --out review.csv
./survex-duplicates "my cave.dump" --epsilon 0.05 --out review-5cm.csv
./survex-duplicates --version
./survex-duplicates --help
```

For Windows, replace `./survex-duplicates` with `.\survex-duplicates.exe`.
`--relative-percent 5` means **5%**, while `--epsilon 0.05` means **5 cm**.
Both limits apply to each matched endpoint; they are separate modes.
Existing output files are protected. Choose a new filename or use `--force`
to replace a report after successful processing.

## Build from source

Install [Go 1.26.3](https://go.dev/dl/) and Git, then clone the repository.
The build uses only the Go standard library. On macOS or Linux:

```bash
git clone https://github.com/dlubom/survex-duplicates.git
cd survex-duplicates
go build -o bin/survex-duplicates .
./bin/survex-duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
```

On Windows, use PowerShell:

```powershell
git clone https://github.com/dlubom/survex-duplicates.git
cd survex-duplicates
go build -o bin/survex-duplicates.exe .
.\bin\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --out duplicates.csv
```

For the other examples, use the executable under `bin/` instead of the archive's
root executable. `go run . INPUT --out OUTPUT` also works from a source checkout.
An ordinary source build reports `dev (unknown)` with `--version`; release
packages report their version and source revision.
The detector, tests and release tools use Go. The former Python implementation
is archived in Git; it is not installed or built by the current project.

## Development and verification

Run the Go checks from the repository root:

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
go build ./...
go run ./cmd/release matrix
```

`gofmt -l` must print nothing. On Windows ARM64, omit the race command because
Go 1.26.3 does not support that combination; ordinary tests still run.
The Go suite checks complete ordered pairs, orientation, both squared endpoint
errors, reader metadata and CSV bytes. An independent exhaustive oracle uses
exact rational arithmetic without calling the production predicate or spatial
index. Tests cover seeded inputs, int32 extremes, the 512-leg registry sample
and all eight complete KATASTER reports. Publication tests use an offline GitHub
command stub; ordinary tests do not publish releases.

For workflow edits, run `actionlint` 1.7.12; CI downloads that pinned version and
verifies its checksum. The six native CI targets run Go checks and extracted
package tests, with real `dump3d` integration on Linux.

See the [quality guide](docs/development.md) for pinned Staticcheck and
govulncheck, unit plus release-CLI coverage, curated mutation regressions,
native fuzzing and allocation benchmarks. CI saves coverage, mutation logs and
benchmark reports. These checks supplement the exact-result assertions above.

The registry is bundled; no external checkout is needed. Binary integration
requires `dump3d`, and tests report a skip if it is unavailable. Optional
`KATASTER_PATH` selects another binary, but historical expectations apply only
when its SHA-256 matches. New results belong in ignored `reports/`; binaries
belong in ignored `bin/`. Preserve the input snapshots and historical evidence.

The completed Go/Python comparisons remain reproducible from the fixed
pre-cleanup commit. See [historical benchmark reproduction](benchmarks/README.md#reproduce-the-archived-python-reference)
and [ADR 0005](docs/adr/0005-retire-python-reference.md). Python, NumPy, SciPy
and uv are needed only when deliberately replaying that archived checkout.

See the [verification record](docs/verification.md),
[Go migration evidence](experiments/go-2026-10-26/README.md),
[benchmarks](benchmarks/README.md), [fixture provenance](tests/fixtures/README.md),
[specification](docs/specification.md) and [agent instructions](AGENTS.md).
The [release guide](docs/releasing.md) describes package verification and tag
publication; [prepared v1.1.0 notes](docs/releases/1.1.0.md) describe this transition.

## Limits and license

The detector examines adjusted model geometry. It cannot recover readings
already averaged by Survex, locate exact original SVX/SRV rows, or establish
physical station identity. It does not delete data, reproject coordinates,
align displaced models or classify survey errors. See
[interpretation and limits](docs/usage.md#interpretation-and-limits).

Software: [MIT](LICENSE). Bundled registry data and survey-derived evidence:
[CC BY-SA 4.0 with attribution](NOTICE.md). These are separate licenses.
