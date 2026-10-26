# Tolerance experiment — 2026-10-25

This is preserved historical evidence. Its Python scripts were retired from the
active tree after the Go migration and remain available at immutable commit
[`b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1`](https://github.com/dlubom/survex-duplicates/tree/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1).
Follow the [archived reproduction guide](../../benchmarks/README.md#reproduce-the-archived-python-reference)
before running the commands below; current development uses Go.

## Conclusion and scope

An absolute 5 cm limit misses examples worth reviewing. Relative 5% of the
shorter leg is a reasonable **initial candidate-search threshold**, not an
empirically established separator of every real repeat from other geometry.
The owner approved that default, a separate absolute mode and optional SPLAY
exclusion on 2026-10-25.

This historical experiment included **all flags**, including acknowledged
DUPLICATE legs. The later publication policy excludes DUPLICATE by default;
use `--include-duplicates` to reproduce its 220/62 results. Current normal
reports have 218/60 pairs. This policy change does not alter the saved geometry
or experimental matches. The [publication provenance record](publication-provenance.json)
records path-only redactions, hash-reference updates and explicit policy labels.

The selected behaviour compares both endpoints and reversal, retains full names
and flags, and reports errors in metres, centimetres and percent of the shorter
length. Relative mode has no hard 5 cm cap. No data is deleted and a match is not
automatically considered a survey error.

At 5%, excluding SPLAY in the original all-flags experiment removes 6431 legs,
leaving 12153 legs and 62 pairs. Both Mietusia examples and all six Wysoka pairs
remain in this audit. The current default additionally excludes the two
acknowledged DUPLICATE legs and their two Mietusia pairs.

There is no basis for calling 5% optimal: a fully labelled set of genuine and
non-genuine repeats is unavailable, so precision and recall were not estimated.
Evidence for tens-of-metres or hundred-metre legs is also limited: each tested
relative threshold has only one matching pair with shorter length at least 20 m,
and that pair is exactly coincident.

## Inputs and provenance

- Preserved input: [examples/kataster/KATASTER.3d](../../examples/kataster/KATASTER.3d).
- Binary SHA-256: `55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077`.
- 18584 legs; **172673236 unordered pairs**. All surveys and flags are combined.
- Survex 1.4.24, model v8, EPSG:32634.
- Original analysis: Python 3.12.14, NumPy 2.3.5; full environment in `results.json`.
- Lengths: minimum 0.02236 m, median 3.48951 m, p90 9.52544 m, p99 20.31230 m,
  maximum 107 m. No point legs occur in this model.
- `kataster.legs.dump` is a generated model representation, not source readings.
  `results.json` and `pairs.csv` contain analysis outputs. The input and registry
  sources were not modified. Data attribution and CC BY-SA 4.0 terms are in
  [NOTICE.md](../../NOTICE.md).

## Method

[The archived compare.py](https://github.com/dlubom/survex-duplicates/blob/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1/experiments/tolerance-2026-10-25/compare.py)
is an experimental script for this real dump, not the production
parser or detector. It enumerates every pair in NumPy blocks **without KDTree**,
computes four squared endpoint distances in centimetres and picks the smaller
orientation-specific maximum. It proves int64 safety for the actual input range.
Qualification uses exact integers; roots are used only to describe accepted pairs.

For fractional threshold p, the relative rule is `D² <= p² * min(L_a²,L_b²)`.
At 5% this is exactly `400*D² <= min(L_a²,L_b²)`. A point-leg pair would therefore
need identical geometry; it does not occur here and is covered by production
synthetic tests.

The saved result is the union of pairs meeting any tested rule, with no output
truncation. Every saved pair's errors and orientation were independently checked
again using Python integers. NODE lookup retains every exact-position name
and anonymous status.

The original full enumeration took approximately 3.01 seconds. It was **not a
production benchmark**: peak memory and 50k/100k inputs were not measured then.
Current measured performance is documented separately under `benchmarks/`.

## Historical results

“No SPLAY” means neither member has the SPLAY flag, not a classification of
real repeated observations. All rows below include DUPLICATE legs.

| Rule | All pairs | Pairs without SPLAY |
| --- | ---: | ---: |
| Exact geometry | 56 | 49 |
| Absolute 1 cm | 59 | 49 |
| Absolute 5 cm | 104 | 51 |
| Absolute 10 cm | 172 | 54 |
| Absolute 50 cm | 2393 | 65 |
| Relative 1% | 88 | 53 |
| Relative 2% | 123 | 56 |
| Relative 5% | 220 | 62 |
| Relative 10% | 462 | 68 |
| min(5 cm, 5%) | 98 | 51 |
| max(5 cm, 5%) | 226 | 62 |

Relative 5% adds 122 pairs and omits 6 compared with absolute 5 cm, a net gain
of 116. Of the additional pairs, 11 have no SPLAY and 111 have at least one.
All six omitted pairs concern two short splays. These counts alone do not
establish which pairs are physically repeated observations.

## Examples independently supported by source surveys

### Mietusia Wyżnia: deliberately acknowledged repeat surveys

| Model legs | Shorter length | Maximum endpoint error | Relative error |
| --- | ---: | ---: | ---: |
| 2836 ↔ 14209 | 3.15636 m | 1.73205 cm | 0.54875% |
| 2837 ↔ 14228 | 4.56503 m | 10 cm | 2.19057% |

The second pair is `MietusiaWyznia:mr_23→mr_22` and
`MietusiaWyznia:pe_2→pe_1`. Their first endpoints coincide and second endpoints
differ by 10 cm. Absolute 5 cm and relative 1%/2% miss it; 5% includes it.
It now appears only in the explicit acknowledged-duplicate audit.

Upstream source evidence, pinned to the available source revision:

- [perystaltyka.svx, line 25](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Mietusia/M_Swistowka/Mietusia_Wyznia/_RAW/01/perystaltyka.svx#L25): explicit
  `*flags duplicate`, two readings dated 2024-12-07.
- [mylna_rura.svx, line 63](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Mietusia/M_Swistowka/Mietusia_Wyznia/_RAW/01/mylna_rura.svx#L63) and
  [line 94](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Mietusia/M_Swistowka/Mietusia_Wyznia/_RAW/01/mylna_rura.svx#L94): earlier readings dated 2024-02-13,
  using a different instrument and team.
- [mietusia_wyznia.svx, line 88](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Mietusia/M_Swistowka/Mietusia_Wyznia/_RAW/01/mietusia_wyznia.svx#L88): connecting
  the middle station.
- Active equivalents: [MWYZN_PE.SRV, line 26](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Mietusia/M_Swistowka/Mietusia_Wyznia/MWYZN_PE.SRV#L26) and
  [MWYZN_MR.SRV, line 62](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Mietusia/M_Swistowka/Mietusia_Wyznia/MWYZN_MR.SRV#L62).

These are preserved independent measurements, not automatic data errors.
The outer stations were not equated; evidence comes from flags, traverse
structure and source readings.

### Wysoka–Za Siedmiu Progami: matching six-leg sequences

The active [7PROGOW.SRV](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Koscieliska/Wys_Grzbiet/Wysoka-7_Progow/7PROGOW.SRV#L124) contains two runs with identical
length/bearing/inclination triples and no intervening unit or calibration change:

| First sequence | Second sequence | Length / bearing / inclination |
| --- | --- | --- |
| 2→3, line 124 | 2→28, line 153 | 14.3 / 242 / 80 |
| 3→4, line 125 | 28→29, line 154 | 9.1 / 246 / 62 |
| 4→41, line 168 | 29→30, line 155 | 12.5 / 218 / 66 |
| 41→42, line 169 | 30→31, line 156 | 14 / 228 / 82 |
| 42→43, line 170 | 31→32, line 157 | 5.5 / 268 / 40 |
| 43→44, line 171 | 32→33, line 158 | 5.2 / 0 / 90 |

Full name prefix: `Wysoka7Progow:W-*/2/`. Model pairs are (1448,7948),
(1449,7949), (7950,7961), (7951,7962), (7952,7963), (7953,7964).
The first pair has maximum error 7.071 cm, the others 13.601 cm. Relative errors
range from 0.495% to 2.616%. All pass 5%; none pass absolute 5 cm.
All six remain in the normal publication report.

This is a strong candidate for auditing a repeated traverse. The
[source metadata](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/Poligony/D_Koscieliska/Wys_Grzbiet/Wysoka-7_Progow/_RAW/01/README.md#L3) says the original is unavailable
and its origin unknown. Physical station identity and the cause of repetition
are not established by this evidence.

### Unconfirmed cases

The 5% threshold also includes Lodowa w Ciemniaku pair 8617↔8722: lengths about
12.57/12.65 m, error 62.37 cm (4.96%). One is a SPLAY and the other leads to a
named station. It requires review and is not a confirmed false positive.

## Geometric reasoning and limits

- Length-relative endpoint tolerance permits larger positional errors for long legs
  while treating short legs more strictly.
- At 5%, the triangle inequality bounds length difference by 10% of the shorter
  leg and the angle between matched directions by about 5.73 degrees. Extra
  angle/length thresholds are not needed to prevent arbitrarily different legs.
- This is not an accuracy model: network adjustment or displacement of a whole
  network portion need not scale with one leg's length.
- Parallel 100 m legs four metres apart pass 5%, without proving a repeated
  observation. Ten-centimetre legs shifted by 1 cm fail it. One threshold cannot
  guarantee complete semantic classification.
- On the centimetre grid, 5% permits only exact geometry for lengths below 20 cm.
  This follows from the threshold, not coordinate rounding.

## Independent verification and reproduction

[The archived verification.py](https://github.com/dlubom/survex-duplicates/blob/b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1/experiments/tolerance-2026-10-25/verification.py)
uses MOVE/LINE, Fraction parsing and Python integers only,
without importing compare.py or NumPy. A deterministic 512-leg sample (seed
20261025, including both Mietusia examples) checked all 130816 pairs. Pair sets,
rules, squared errors and orientations agreed, as did 67 available metadata
records. There were zero discrepancies. This is a sample check, not a second
full enumeration of 172.7 million pairs.

From the root of the archived `b99bef2` checkout, use a separate output directory
so preserved evidence is not overwritten:

```bash
uv run --locked python experiments/tolerance-2026-10-25/compare.py examples/kataster/KATASTER.3d --out-dir reports/tolerance-experiment
uv run --locked python experiments/tolerance-2026-10-25/verification.py --results reports/tolerance-experiment/results.json --out reports/tolerance-experiment/verification.json
```

Both find dump3d in PATH. The comparison requires NumPy; the independent
verification uses the standard library. The original comparison ran on Python
3.12.14/NumPy 2.3.5, and the original sample verification on Python 3.9.6.
Production requirements and tests were completed later and are recorded in the
main README and verification record.
