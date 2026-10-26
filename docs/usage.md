# Using Survex Duplicate Detector

The detector compares every eligible pair of leg records in one adjusted
spatial model. A pair is reported only when **both endpoints** are close enough,
either in the original direction or after reversing one leg. The CSV is a
review aid: it does not change the model or decide whether survey data is wrong.

## Start with the bundled model

Download and extract a native application archive when one is available on
[GitHub Releases](https://github.com/dlubom/survex-duplicates/releases). Choose
your system and processor using the [platform guide](platforms.md). The
executable, `docs/` and `examples/` belong together in the extracted directory.
No Go, Python or Survex installation is needed to run the bundled text demo.
If a suitable archive is not yet available, use the
[source build instructions](https://github.com/dlubom/survex-duplicates#build-from-source).

The following macOS/Linux commands assume your terminal is in the extracted
archive directory:

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

The same commands in Windows PowerShell are:

```powershell
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --out duplicates.csv
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

Expect **218 pairs** in the first CSV and **60** in the second. The terminal
prints a JSON summary; its `K` field is the number of pairs written. Open the
CSV in a spreadsheet or text editor. Select UTF-8 and a comma delimiter if the
spreadsheet asks how to import it.

For a source build, run from the repository root and substitute
`./bin/survex-duplicates` or `.\bin\survex-duplicates.exe` in every example.
The arguments and expected reports are identical. The
[demo guide](../examples/kataster/README.md) links all eight saved reports.

## Choose a tolerance

The default is `--relative-percent 5`: each endpoint may differ by up to
**5% of the shorter leg's length**. The alternative `--epsilon 0.05` sets a
fixed **0.05 m = 5 cm** limit at each endpoint.

| Shorter leg | Default 5% endpoint limit | Absolute 5 cm endpoint limit |
| --- | ---: | ---: |
| 0.10 m | 0.005 m | 0.05 m |
| 1 m | 0.05 m | 0.05 m |
| 10 m | 0.50 m | 0.05 m |

Relative mode can therefore be stricter for short legs and more permissive for
long legs. It has no hidden 5 cm cap and is not a measure of survey accuracy.
On the input's centimetre grid, a limit below 1 cm permits only zero endpoint
error. The boundary is inclusive.

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --relative-percent 5 --out relative-5percent.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --epsilon 0.05 --out absolute-5cm.csv
```

Use one tolerance option at a time. `--relative-percent 0.05` means **0.05%**,
not 5%. Positive decimal values and scientific notation such as `5e-2` are
accepted; zero, negatives, `NaN` and infinity are rejected. Qualification uses
exact squared distances; rounded CSV values do not decide whether a pair matches.

## Choose which legs to review

By default, legs carrying `DUPLICATE` are excluded. That Survex flag records
acknowledged repeat surveying. Use `--include-duplicates` when you also want
to inspect those records. SPLAY legs are included by default; add
`--exclude-splays` to omit radial measurements flagged as SPLAY.

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --include-duplicates --out full-audit.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --include-duplicates --exclude-splays --out audit-without-splays.csv
```

These produce 220 and 62 pairs respectively for the bundled model.
`SURFACE` alone does not exclude a leg. Combined flags obey both switches;
a leg excluded by both rules is counted only once. Filtering preserves original
leg IDs and does not remove station names used by the remaining legs. The
whole input is validated even if some legs are excluded.

## Read binary models or text dumps

A saved **complete UTF-8 text dump** is self-contained. It must retain the
header, all relevant records and the final `STOP`; a list of copied LEG lines
is not a valid input. Both LEG and MOVE/LINE dump forms are supported, but they
must not be mixed in one dump.

Binary `.3d` input requires Survex's `dump3d`, with `--legs` and `--show-dates`
support. Survex 1.4.24 was used for the bundled fixtures. With `dump3d` in PATH:

```bash
./survex-duplicates examples/kataster/KATASTER.3d --out binary-results.csv
```

You can give an explicit executable path. For example, in Windows PowerShell,
with Survex installed under a local `tools/Survex/` directory:

```powershell
.\survex-duplicates.exe "my cave.3d" --dump3d ".\tools\Survex\dump3d.exe" --out binary-results.csv
```

The detector invokes `dump3d --legs --show-dates INPUT` itself. To prepare a
reusable text dump separately in a macOS/Linux shell:

```bash
dump3d --legs --show-dates "my cave.3d" > "my cave.dump"
./survex-duplicates "my cave.dump" --out my-cave-results.csv
```

Keep a dump only if `dump3d` completed successfully. The detector checks its
own `dump3d` process's final status and does not publish a report after failure.

Format detection uses the binary signature, not the filename extension.
`--input-format 3d` or `--input-format dump` can select a format explicitly.
Use an ordinary spatial model in one East/North/Up coordinate system. Extended
elevation is rejected; the detector does not reproject or align separate models.

## Read a CSV row

Each row is one unordered pair of distinct input records. Rows are ordered
by `leg_a`, then `leg_b`, with `leg_a < leg_b`. Three identical eligible records
produce three pairs, not one group. A pair count is not a count of errors,
stations, passages or records that can be deleted.

| Fields | How to use them |
| --- | --- |
| `leg_a`, `leg_b` | Stable IDs assigned in input order before filtering; gaps are normal. |
| `survey_a`, `survey_b` | Survey labels attached to the records, not station identities. |
| `source_line_a`, `source_line_b` | One-based lines in the complete text dump, not original SVX/SRV lines. |
| `flags_a/b`, `style_a/b`, `date_a/b` | Model metadata; dates are survey dates or date ranges, and may be empty. |
| `endpoint_1_error_m`, `endpoint_2_error_m` | The two matched endpoint errors, in metres, in the chosen orientation. |
| `distance_m`, `distance_cm` | The larger of the two endpoint errors, shown in metres and centimetres. |
| `reversed` | `0`: a1 matches b1 and a2 matches b2. `1`: a1 matches b2 and a2 matches b1. |
| `length_a_m`, `length_b_m`, `relative_error_percent` | Leg lengths and maximum endpoint error as a percentage of the shorter length. |
| `tolerance_mode`, `tolerance_value`, `tolerance_m` | The chosen mode, parameter and actual limit for this pair. |
| `a_x1`…`b_z2` | Original endpoint coordinates in metres; x/y/z mean East/North/Up. |
| `a1_station_names`…`b2_station_names` | JSON arrays of all distinct full names at exactly each endpoint's position. |

**Reversal changes the comparison, not the stored endpoint order.** For a
reversed row, compare `a1_station_names` with `b2_station_names` and a2 with b1.
The first endpoint error always refers to a1's chosen match, the second to a2's.
If both orientations have the same maximum error, the direct one is reported.

Names such as `["MietusiaWyznia:mr_23", "MietusiaWyznia:pe_2"]` mean both
labels occur at that position. They do not establish which label an original
reading used. Empty name lists are `[]`. The corresponding `*_station_status`
distinguishes `named`, `unnamed` (NODE exists without an available name), and
`missing_node` (no NODE record at the endpoint). Missing names do not invalidate
geometry. `*_has_anonymous_node=1` can coexist with a named endpoint.

The CSV has 47 columns; the [specification](specification.md#csv-report) lists
the complete order. Coordinates have two decimal places. Distances retain
17 significant digits, so 10 cm may appear as `0.10000000000000001` metres.
The relative-error field is empty when the shorter length is zero.

## Three real examples

These rows come from the unchanged bundled snapshot. Values below are rounded
for readability; the linked CSVs retain their full precision. Locate a row by
both leg IDs, rather than by its spreadsheet row number.

### Exact geometry with reversed direction

In the [60-pair report](../examples/kataster/results/kataster-relative_percent-no-splays.csv),
legs **1414 and 1415** belong to `Wysoka7Progow`:

| Leg | First endpoint | Second endpoint | Dump line |
| --- | --- | --- | ---: |
| 1414 | `Wysoka7Progow:W7-501` | `Wysoka7Progow:W7-500` | 1701 |
| 1415 | `Wysoka7Progow:W7-500` | `Wysoka7Progow:W7-501` | 1702 |

`reversed=1` and both endpoint errors are zero. The records cover exactly the
same adjusted geometry in opposite directions. This explains the match; it
does not establish why the two records exist or whether either can be removed.

### A match at 5% that fails at 5 cm

The same [60-pair report](../examples/kataster/results/kataster-relative_percent-no-splays.csv)
contains legs **1449 and 7949**, also in `Wysoka7Progow`:

| Leg | First endpoint | Second endpoint | Dump line |
| --- | --- | --- | ---: |
| 1449 | `Wysoka7Progow:W-*/2/3` | `Wysoka7Progow:W-*/2/4` | 1744 |
| 7949 | `Wysoka7Progow:W-*/2/28` | `Wysoka7Progow:W-*/2/29` | 8422 |

This is a direct match (`reversed=0`). Its endpoint errors are approximately
**7.071 cm and 13.601 cm**. The shorter leg is 9.10543 m, so the 5% limit is
45.527 cm at each endpoint. The maximum error is approximately 1.494% of that
length: both endpoints pass. They exceed the absolute 5 cm limit, so the pair
is absent from the [50-pair report](../examples/kataster/results/kataster-absolute_m-no-splays.csv).

This pair belongs to a six-leg matching sequence documented in the
[historical tolerance investigation in the source repository](https://github.com/dlubom/survex-duplicates/blob/main/experiments/tolerance-2026-10-25/README.md#wysokaza-siedmiu-progami-matching-six-leg-sequences).
The sequence merits source review. Geometry and matching measurements alone
do not establish physical station identity or the cause of the repetition.

### An acknowledged repeat survey

Legs **2837 and 14228** in `MietusiaWyznia` appear in the
[62-pair full audit](../examples/kataster/results/kataster-relative_percent-no-splays-include-duplicates.csv),
but not in the default 60-pair report:

| Field | Leg 2837 | Leg 14228 |
| --- | --- | --- |
| First endpoint names | `MietusiaWyznia:mr_23`, `MietusiaWyznia:pe_2` | `MietusiaWyznia:mr_23`, `MietusiaWyznia:pe_2` |
| Second endpoint names | `MietusiaWyznia:mr_22`, `MietusiaWyznia:ot_22` | `MietusiaWyznia:pe_1` |
| Survey date | `2024.02.13` | `2024.12.07` |
| Flags | empty | `DUPLICATE` |
| Dump line | 3215 | 14701 |

The direct endpoint errors are **0 cm and 10 cm**. The shorter leg is 4.56503 m,
giving a 5% limit of 22.825 cm. The pair passes relative 5% but fails absolute
5 cm. It is excluded by default because leg 14228 already carries `DUPLICATE`.
The [source investigation](https://github.com/dlubom/survex-duplicates/blob/main/experiments/tolerance-2026-10-25/README.md#mietusia-wyżnia-deliberately-acknowledged-repeat-surveys)
documents deliberate repeat measurements. A detected match can therefore be
expected survey information worth retaining.

## Paths, output files and failures

Paths are relative to the terminal's current directory unless absolute paths
are supplied. Quote paths containing spaces. The output directory must already
exist; for example, from an extracted archive on macOS/Linux:

```bash
mkdir -p reports
./survex-duplicates "examples/kataster/KATASTER.legs.dump" --out "reports/cave review.csv"
```

For Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force reports | Out-Null
.\survex-duplicates.exe "examples/kataster/KATASTER.legs.dump" --out "reports/cave review.csv"
```

An existing output is an error unless `--force` is given. To rerun deliberately:

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --out "reports/cave review.csv" --force
```

The detector writes a temporary file beside the destination, flushes and syncs
it, and publishes it only after successful processing. Processing failures leave
an existing report unchanged. Without `--force`, the destination filesystem
must support hardlinks for atomic create-if-absent publication. Replacement with
`--force` uses the operating system's rename operation; see the
[platform guide](platforms.md) for its filesystem guarantees. The output cannot
be the input, including through a symlink or hardlink.

Exit status **0** means success, including a header-only CSV with no matches.
Status **2** means invalid arguments; **1** means an input, subprocess,
computation or output failure. Diagnostics and the JSON summary go to stderr;
stdout is unused. A successful summary includes:

| Field | Meaning |
| --- | --- |
| `n_input` | All input leg records. |
| `n_excluded` | Legs excluded by the active flag filters, counted once. |
| `n` | Eligible legs analysed. |
| `C` | Unordered candidate pairs checked after the spatial index search. |
| `K` | Accepted pairs written to CSV. |

The summary also records the tolerance, both flag settings, batch size, input
format and read/build/search-with-write times. `C` is not the result count.
`--help` and `--version` instead print to stdout and require no input or output
path. Use `--version` when reporting a problem: it identifies the version and
source revision embedded in the executable. Ordinary source builds report
`survex-duplicates dev (unknown)`.

## Option reference

| Option | Meaning |
| --- | --- |
| `INPUT` | One existing binary model or complete UTF-8 text dump. |
| `--out PATH` | Required CSV destination. |
| `--relative-percent P` | Positive percentage of the shorter leg; default `5`. |
| `--epsilon M` | Positive absolute endpoint tolerance in metres. |
| `--include-duplicates` | Include legs carrying `DUPLICATE`. |
| `--exclude-splays` | Exclude legs carrying `SPLAY`. |
| `--input-format auto\|3d\|dump` | Default `auto`; otherwise require the selected input format. |
| `--dump3d PATH` | Executable path; default lookup of `dump3d` in PATH. |
| `--batch-size B` | Positive query batch size; default `1`. Larger values use more memory and do not change results. |
| `--force` | Replace an existing output after successful processing. |
| `--help` | Print command help. |
| `--version` | Print the executable version and source revision. |

## Interpretation and limits

Review station labels, coordinates, flags, dates and the original survey sources
together before deciding what a candidate means. Several names at a position
are possible aliases, not a choice made by the detector. Dump line numbers can
locate model records, but cannot identify exact original SVX/SRV readings.

The model contains adjusted geometry; Survex may already have averaged repeated
readings. The detector cannot recover records absent from the model. It performs
no deletion, representative selection, clustering, reprojection or alignment of
translated/rotated models. Matching is not transitive: if A matches B and B
matches C, A need not match C.

Coordinates must be signed int32 centimetres; off-grid values such as 0.001 m
are rejected rather than rounded. Zero-length legs are valid. In relative mode
they match only another zero-length leg at the same position; absolute mode
applies its endpoint radius normally.

There is no result cap. A dense model can produce `n*(n-1)/2` pairs, so processing
time and CSV size can grow quadratically. At batch size 1, working memory is
O(n + station records); a batch of B legs can add O(B*n) neighbour storage.
Measured performance on the bundled registry is not a guarantee for every model.
