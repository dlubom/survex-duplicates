# Complete Tatra Cave Registry demo

This is the full **18,584-leg** spatial model, not a cropped toy dataset.
It was preserved from the Tatra Cave Registry on 2026-10-25. The original
upstream repository is
[Jaskiniowy-Kataster-Tatr-Zachodnich](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich).

## Files and provenance

- `KATASTER.3d`: unchanged compiled binary snapshot, approximately 647 KiB.
- `KATASTER.legs.dump`: unchanged complete dump generated with Survex 1.4.24,
  using `dump3d --legs --show-dates`; approximately 3 MiB.
- `provenance.json`: SHA-256 values, source context, generator and attribution.
- `LICENCE`: the upstream CC BY-SA 4.0 notice.
- `results/`: generated reports and their complete verification record.

The compiled model was generated locally in the source project and is preserved
by hash. It is not presented as a downloaded upstream release. Source checkout
revision `a41faea66c3c3170a96010ef7314b375875cfced` identifies the available source
context; a clean rebuild yielding the same binary is not asserted.

Binary SHA-256:
`55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077`.

Dump SHA-256:
`845c9bc863f40dd2375c1004c4d02b0293b15134abab7006de60c72addf6f298`.

## Run from the repository root

```bash
python -m survex_duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
python -m survex_duplicates examples/kataster/KATASTER.legs.dump --exclude-splays --out duplicates-without-splays.csv
python -m survex_duplicates examples/kataster/KATASTER.legs.dump --include-duplicates --out duplicates-audit.csv
python -m benchmarks.verify_kataster
```

The dump commands require no Survex installation. The verification command
also reads the binary through real `dump3d`, checks its hash before loading
historical expectations, and checks that the input hash is unchanged afterwards.
It compares full ordered pairs, orientations and both endpoint errors for all
eight combinations of the two tolerances, SPLAY and DUPLICATE settings.

| Mode | Include acknowledged duplicates | Exclude SPLAY | Excluded legs | Analysed legs | Pairs |
| --- | --- | --- | ---: | ---: | ---: |
| 5% | no | no | 2 | 18582 | 218 |
| 5% | no | yes | 6433 | 12151 | 60 |
| 5% | yes | no | 0 | 18584 | 220 |
| 5% | yes | yes | 6431 | 12153 | 62 |
| 0.05 m | no | no | 2 | 18582 | 103 |
| 0.05 m | no | yes | 6433 | 12151 | 50 |
| 0.05 m | yes | no | 0 | 18584 | 104 |
| 0.05 m | yes | yes | 6431 | 12153 | 51 |

The known Mietusia examples are intentionally marked DUPLICATE, so they appear
only in the explicit audit. The six Wysoka repeat-sequence candidates remain
in the normal report. Matches still require interpretation of the underlying surveys.

## Updating the demo

Keep inputs read-only during analysis. A replacement model needs its own source
attribution, hash and independent expected result; old expectations cannot be
reused for a different hash. Preserve the historical experiment as a separate
record. Regenerate the dump with a recorded Survex version and verify every
pair and orientation for both tolerances and both flag settings before updating
the expected-result table.

## License

Attribution: **Tatra Cave Registry contributors**. The model, its dump and derived
survey reports are licensed under
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/), separately from
the detector's MIT software license. The inputs are unchanged copies. The
reports are generated analyses; metadata paths are portable publication labels.
See [NOTICE.md](../../NOTICE.md) for attribution scope.
