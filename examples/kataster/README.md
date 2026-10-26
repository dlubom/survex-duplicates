# Complete Tatra Cave Registry demo

This is the full **18,584-leg** spatial model preserved on 2026-10-25 from the
[Tatra Cave Registry](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich).
The complete binary, text dump and eight expected CSV reports are bundled with
the repository and application archives. With a native package, the text demo
requires only the detector executable: no Go, Python, Survex or separate registry
checkout is needed. Choose the matching Linux, macOS or Windows package using
the [platform guide](../../docs/platforms.md).

## Run the demo

Run from the extracted application archive's top-level directory, where
`survex-duplicates` and `examples/` are located. On macOS or Linux:

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

On Windows, use PowerShell:

```powershell
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --out duplicates.csv
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
```

For a [source build](https://github.com/dlubom/survex-duplicates#build-from-source), run from the repository
root and use `./bin/survex-duplicates` or `.\bin\survex-duplicates.exe` instead.
The first command writes **218 pairs**, the second **60**. The JSON summary's
`K` is the pair count. Choose a new output name for subsequent runs, or add
`--force` to replace your own generated report after successful processing.
Keep the files under `examples/kataster/results/` unchanged.

## Expected reports

The pair counts link to the complete saved CSVs. All rows refer to the same
18,584-leg snapshot. “Include DUPLICATE” enables acknowledged repeat surveys;
“Exclude SPLAY” removes radial measurements carrying that flag.

| Tolerance | Include DUPLICATE | Exclude SPLAY | Excluded legs | Analysed legs | Pairs / expected CSV |
| --- | --- | --- | ---: | ---: | ---: |
| 5% | no | no | 2 | 18,582 | [218](results/kataster-relative_percent-all.csv) |
| 5% | no | yes | 6,433 | 12,151 | [60](results/kataster-relative_percent-no-splays.csv) |
| 5% | yes | no | 0 | 18,584 | [220](results/kataster-relative_percent-all-include-duplicates.csv) |
| 5% | yes | yes | 6,431 | 12,153 | [62](results/kataster-relative_percent-no-splays-include-duplicates.csv) |
| 0.05 m | no | no | 2 | 18,582 | [103](results/kataster-absolute_m-all.csv) |
| 0.05 m | no | yes | 6,433 | 12,151 | [50](results/kataster-absolute_m-no-splays.csv) |
| 0.05 m | yes | no | 0 | 18,584 | [104](results/kataster-absolute_m-all-include-duplicates.csv) |
| 0.05 m | yes | yes | 6,431 | 12,153 | [51](results/kataster-absolute_m-no-splays-include-duplicates.csv) |

The normal 5% report omits two Mietusia pairs whose repeat-survey legs already
carry `DUPLICATE`. All six Wysoka matching-sequence pairs remain. The
[usage guide's real examples](../../docs/usage.md#three-real-examples) explain
reversal, the difference between 5% and 5 cm, and why an acknowledged match
need not indicate an error. Every match still requires survey interpretation.

To reproduce all eight reports without overwriting the saved evidence:

```bash
./survex-duplicates examples/kataster/KATASTER.legs.dump --out reproduced-relative-all.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --exclude-splays --out reproduced-relative-no-splays.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --include-duplicates --out reproduced-relative-audit.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --include-duplicates --exclude-splays --out reproduced-relative-audit-no-splays.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --epsilon 0.05 --out reproduced-absolute-all.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --epsilon 0.05 --exclude-splays --out reproduced-absolute-no-splays.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --epsilon 0.05 --include-duplicates --out reproduced-absolute-audit.csv
./survex-duplicates examples/kataster/KATASTER.legs.dump --epsilon 0.05 --include-duplicates --exclude-splays --out reproduced-absolute-audit-no-splays.csv
```

In PowerShell, substitute `.\survex-duplicates.exe` for `./survex-duplicates`;
the arguments are unchanged. A source build uses the executable under `bin/`.

## Files and provenance

- [KATASTER.3d](KATASTER.3d): unchanged compiled binary snapshot, approximately 647 KiB.
- [KATASTER.legs.dump](KATASTER.legs.dump): unchanged complete dump generated
  with Survex 1.4.24 using `dump3d --legs --show-dates`; approximately 3 MiB.
- [provenance.json](provenance.json): input SHA-256 values, source context,
  generator and attribution.
- [LICENCE](LICENCE): the upstream CC BY-SA 4.0 notice.
- [results/verification.json](results/verification.json): saved report hashes,
  counts and checks of complete pairs, orientations and both endpoint errors.

The compiled model was generated locally in the source project and is preserved
by hash. It is not presented as a downloaded upstream release. Source checkout
revision `a41faea66c3c3170a96010ef7314b375875cfced` identifies the available source
context; a clean rebuild yielding the same binary is not asserted.

Binary SHA-256:
`55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077`.

Dump SHA-256:
`845c9bc863f40dd2375c1004c4d02b0293b15134abab7006de60c72addf6f298`.

Verify input hashes before relying on these expectations. From the archive or
repository root, use `shasum -a 256` on macOS:

```bash
shasum -a 256 examples/kataster/KATASTER.3d examples/kataster/KATASTER.legs.dump
```

On Linux, use `sha256sum` with the same two paths. In Windows PowerShell:

```powershell
Get-FileHash examples/kataster/KATASTER.3d, examples/kataster/KATASTER.legs.dump -Algorithm SHA256
```

Report hashes are listed individually in `results/verification.json`. Comparing
only the number of CSV rows is insufficient: orientation and both endpoint
errors must agree too.

For developers, the Go CLI regression test verifies all eight complete reports
from the saved dump without requiring Survex:

```bash
go test -count=1 -run '^TestCLIBundledKatasterReports$' .
```

This validates the preserved model/dump hashes before using expectations,
compares complete CSV bytes and checks the input hashes again afterwards.
The full Go suite also covers the independent exact oracle and real binary
integration when `dump3d` is installed. Historical Go/Python comparisons can
be replayed from the
[pinned pre-cleanup checkout](https://github.com/dlubom/survex-duplicates/blob/main/benchmarks/README.md#reproduce-the-archived-python-reference).
The saved verification file records its original run, not timings for the
machine on which you run the demo.

## Updating the demo

Keep inputs read-only during analysis. A replacement model needs its own source
attribution, hash and independent expected result; old expectations cannot be
reused for a different hash. Preserve the historical experiment as a separate
record. Regenerate the dump with a recorded Survex version and verify every
pair, orientation and both endpoint errors for both tolerances and both flag
settings before updating the expected-result table. Record new measurements
separately from preserved numerical evidence.

## License

Attribution: **Tatra Cave Registry contributors**. The model, its dump and derived
survey reports are licensed under
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/), separately from
the detector's MIT software license. The inputs are unchanged copies. The
reports are generated analyses; metadata paths are portable publication labels.
See [NOTICE.md](../../NOTICE.md) for attribution scope.
