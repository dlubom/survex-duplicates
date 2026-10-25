# Survex Duplicate Detector

Find geometric duplicate candidates in a Survex `.3d` model or a complete
`dump3d` text dump. Every reported pair matches at **both endpoints**, allowing
reversed direction. The default tolerance is **5% of the shorter leg**.

Legs already marked `DUPLICATE` are excluded by default: they acknowledge
intentional repeat surveys. Use `--include-duplicates` for a full geometric
audit, and `--exclude-splays` to omit radial measurements as well.

The detector preserves the input, record IDs, survey metadata and all station
names found at the exact endpoint coordinates. It writes an ordered CSV report.
Matches are candidates for human review, not automatic proof of erroneous data.

## Quick start: the complete Tatra Cave Registry

The repository includes a complete registry model with **18,584 legs**, its
text dump and verified example reports. The text demo works without Survex.
Python 3.12 or newer is required.

```bash
git clone https://github.com/dlubom/survex-duplicates.git
cd survex-duplicates
python3.12 -m venv .venv
source .venv/bin/activate
python -m pip install .

python -m survex_duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
python -m survex_duplicates examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

On Windows, activate the environment with `.venv\Scripts\activate`.
You can use `survex-duplicates` in place of `python -m survex_duplicates`.
The legacy `python znajdz_duplikaty.py ...` wrapper invokes the same CLI.

Expected results for the bundled snapshot:

| Tolerance | Include `DUPLICATE` | Exclude SPLAY | Analysed legs | Pairs |
| --- | --- | --- | ---: | ---: |
| 5% | no (default) | no | 18,582 | **218** |
| 5% | no (default) | yes | 12,151 | **60** |
| 5% | yes | no | 18,584 | **220** |
| 5% | yes | yes | 12,153 | **62** |
| 0.05 m | no (default) | no | 18,582 | **103** |
| 0.05 m | no (default) | yes | 12,151 | **50** |

The two acknowledged Mietusia repeat-survey pairs are omitted by default.
The six Wysoka repeat-sequence candidates remain. Historical counts of 220/62
refer to the explicit `--include-duplicates` audit. Full pairs, orientations
and both endpoint errors are checked, rather than just the totals.

See [demo provenance, licensing and reports](examples/kataster/README.md).

## Binary input and absolute tolerance

Binary `.3d` input additionally requires Survex's `dump3d` executable in PATH,
with support for `--legs` and `--show-dates`. Tested with Survex 1.4.24.

```bash
survex-duplicates examples/kataster/KATASTER.3d --out duplicates-binary.csv
survex-duplicates examples/kataster/KATASTER.legs.dump --epsilon 0.05 --out duplicates-5cm.csv
survex-duplicates examples/kataster/KATASTER.3d --include-duplicates --out duplicates-audit.csv
survex-duplicates --help
```

`--relative-percent 5` means five percent, not the fraction 0.05.
`--epsilon 0.05` selects an absolute tolerance of five centimetres for each
endpoint. Both accept exact decimal and scientific notation, such as `5e-2`.
They are mutually exclusive; zero, negative and non-finite values are rejected.

## CLI options

| Option | Meaning |
| --- | --- |
| `INPUT` | One existing binary model or complete UTF-8 dump. |
| `--out PATH` | Required CSV destination; its parent directory must exist. |
| `--relative-percent P` | Positive percentage of the shorter leg; default `5`. |
| `--epsilon M` | Positive absolute endpoint tolerance in metres. |
| `--include-duplicates` | Include legs carrying the `DUPLICATE` flag. |
| `--exclude-splays` | Exclude legs carrying the `SPLAY` flag, including combined flags. |
| `--input-format auto\|3d\|dump` | Default `auto`: detect the Survex binary signature, otherwise validate a full dump. |
| `--dump3d PATH` | Executable to use instead of looking up `dump3d` in PATH. |
| `--batch-size B` | Positive number of query legs per batch; default `1`. |
| `--force` | Replace an existing result after successful completion. |

`SURFACE` alone does not exclude a leg. Filters select legs before indexing,
using the union of exclusions, while IDs retain their original input numbering.
All records, including excluded legs, are still validated and all NODE records
remain available for station names.

Exit status: **0** for success, including no matches; **2** for invalid
arguments; **1** for input, process, computation or output errors.
Diagnostics and the JSON summary go to stderr; stdout is unused.
The summary includes `n_input`, `n_excluded`, `n`, candidates `C`, results `K`,
both filter settings, tolerance, batch size, input format and stage timings.

A temporary CSV is created in the destination directory, flushed and synced
before atomic publication. Errors preserve an existing result. Without `--force`,
publication uses an atomic create-if-absent hardlink, so the destination filesystem
must support hardlinks. With `--force`, publication uses `os.replace`.
The output cannot refer to the input, including via a symlink or hardlink.

## Geometry and report semantics

For each orientation, take the larger endpoint error; choose the orientation
with the smaller result. Ties select the direct orientation. The tolerance boundary
is inclusive. At 5%, qualification is the exact integer test
`400 * D_squared <= min(length_a_squared, length_b_squared)` in square centimetres.
Python integers prevent overflow across the entire int32 coordinate range.
Floating-point roots are used only when formatting accepted results.

Input coordinates must lie on the centimetre grid and fit signed int32
centimetres. `0.001 m` is rejected rather than rounded. Coordinates are treated
as one common East/North/Up system; there is no reprojection. Extended elevation
models are rejected. Zero-length legs are valid: in relative mode, only two
point legs at exactly the same location match; absolute mode applies its radius.
Every record keeps its identity, including identical records. Three eligible
identical records yield three pairs. Similarity is not closed transitively.

The UTF-8 CSV has 47 columns in a fixed order and remains identical across
batch sizes. [The specification](docs/specification.md#csv-report) lists them all.
Key fields include:

- `leg_a`, `leg_b`, surveys, flags, styles, shot dates and one-based dump line numbers;
- `distance_m`, `distance_cm` and the two separate endpoint errors;
- original endpoint coordinates, lengths, relative error and actual pair tolerance;
- `reversed` as 0/1, with coordinates and station names kept in their original order;
- `a1/a2/b1/b2_station_names` as JSON arrays of every distinct, sorted full name
  at exactly that endpoint position, with Unicode preserved;
- station status: `named`, `unnamed` (a station without a name), or `missing_node`
  (no station record), plus an independent anonymous-NODE indicator.

Several names at the same position do not identify which name a source reading
used. Survey is not a station name. Missing names do not exclude a leg.
The relative error is empty when the shorter length is zero. Distances use
17 significant digits; coordinates use two decimal places. `tolerance_m` uses
Decimal formatting and represents the pair's actual tolerance, before integer
threshold truncation or outward rounding of an index query radius.

## Development and verification

```bash
python -m pip install -e '.[dev]'
python -m pytest -q
python -m mypy
python -m ruff check survex_duplicates tests benchmarks znajdz_duplikaty.py
python -m ruff format --check survex_duplicates tests benchmarks znajdz_duplikaty.py
python -m benchmarks.verify_kataster
python -m benchmarks.run --out benchmarks/publication-results.json
python -m benchmarks.run --include-duplicates --out benchmarks/audit-results.json
python -m build
```

The registry is bundled: tests have no dependency on a contributor's checkout
or home directory. Real binary tests require `dump3d` and are skipped if it is
unavailable. An optional `KATASTER_PATH` selects another binary snapshot, but
historical expectations are used only after matching its SHA-256.

An independent exhaustive Python-integer oracle checks complete pairs,
orientations and both squared errors on seeded data and a 512-leg registry
sample. The full registry is checked against preserved exhaustive experimental
evidence for all eight combinations of tolerance and flags. See the
[verification record](docs/verification.md), [benchmark results](benchmarks/README.md),
[fixture provenance](tests/fixtures/README.md) and [agent instructions](AGENTS.md).

Tested environment: Python 3.12.14, NumPy 2.3.5, SciPy 1.16.3, pytest 9.1.1,
Survex 1.4.24, mypy 1.20.2, Ruff 0.17.2; macOS 27.0.1 arm64, Apple M4,
16 GiB RAM. Exact development dependencies are in `requirements-tested.txt`.

## Limits

Survex models contain adjusted geometry. Repeated readings may already have
been averaged before this detector sees them. It cannot recover missing source
rows or identify exact SVX/SRV line numbers from a model alone. It performs no
deletion, representative selection, clustering, translation/rotation alignment
or semantic classification of survey errors.
[Survex processing](https://survex.com/docs/manual/cmdline.htm) and
[repeated readings](https://survex.com/docs/manual/svxhowto.htm#repeated-readings)
describe these upstream behaviours.

The KDTree uses component-wise endpoint minima/maxima and the maximum norm in
six dimensions. There is no neighbour or result cap. No **O(n+K)** guarantee
is claimed for KDTree: output can contain `n*(n-1)/2` pairs. With batch size 1,
working memory is O(n + station records); batching B legs can add O(B*n)
neighbour-list memory. Measured performance on specific data is not a guarantee
for arbitrary dense models.

## License

Software: [MIT](LICENSE). Bundled registry data and survey-derived evidence:
[CC BY-SA 4.0, with attribution](NOTICE.md). The registry model and text dump
are unchanged snapshots; generated reports and publication metadata are
identified separately in their provenance records.
