# Real Survex fixtures

The purpose-built `survey.svx` and `legacy.svx` sources were compiled with real
**cavern 1.4.24**. Dump files were generated with **dump3d 1.4.24**;
`extended.3d` was produced by **extend 1.4.24**. Commands, versions and SHA-256
values are preserved in `provenance.json`.

Ordinary Go parser tests verify snapshot hashes and use the saved dumps without
Survex. Binary integration tests additionally read the real models when `dump3d`
is installed. The Polish title and Unicode test labels are intentional input
data and are preserved verbatim.

To investigate regeneration, run the recorded Survex commands in a **fresh
candidate directory**, from the repository root. The final directory must not
already exist; these commands leave the saved fixtures and provenance unchanged.

```bash
(
set -e
mkdir -p reports
mkdir reports/fixture-candidate
cp tests/fixtures/survey.svx tests/fixtures/legacy.svx reports/fixture-candidate/
cd reports/fixture-candidate
cavern --version
dump3d --version
extend --version
cavern --no-auxiliary-files --output=survey.3d survey.svx
cavern --no-auxiliary-files --3d-version=3 --output=legacy.3d legacy.svx
cavern --output=closure.3d legacy.svx
extend survey.3d extended.3d
dump3d --legs --show-dates survey.3d > survey.legs.dump
dump3d --show-dates survey.3d > survey.lines.dump
dump3d --legs --show-dates legacy.3d > legacy.legs.dump
dump3d --legs --show-dates closure.3d > closure.legs.dump
dump3d --legs --show-dates extended.3d > extended.legs.dump
shasum -a 256 *.svx *.3d *.dump
)
```

On Linux, `sha256sum *.svx *.3d *.dump` can replace `shasum`. Creation timestamps
and hashes will change. Compare all records and parser results before proposing
any fixture replacement; record commands, tool versions and new hashes in its
provenance separately. Never refresh saved hashes merely to accept a changed
result. The original Python generator is available in the
[archived reference checkout](../../benchmarks/README.md#reproduce-the-archived-python-reference).

- `survey`: v8 LEG and MOVE/LINE variants of the same model, no CS, CARTESIAN
  style, a single date and date range, SURFACE/DUPLICATE/SPLAY, anonymous NODE,
  dated XSECT and XSECT_END. Survey `p],"q[` demonstrates unescaped brackets,
  commas and quotes in survey and full station names.
- `legacy`: v3, empty root survey `[]`, no STYLE or shot dates, text creation
  date rather than `@timestamp`.
- `closure`: v8 from the same source, NORMAL style and real ERROR_INFO.
  Omitting `--no-auxiliary-files` is intentional: that option also omits this
  information from the model. The archived generation omitted the auxiliary
  `.err` from the committed snapshot; retain it in the candidate directory for
  review.
- `extended`: a real extended-elevation model, rejected by the detector.

Supported survey dates are the default `--show-dates` forms `YYYY.MM.DD` and
`YYYY.MM.DD-YYYY.MM.DD`; alternative `dump3d -D` dates are unsupported.
The ordered header contains TITLE, DATE, DATE_NUMERIC, optional CS, VERSION,
SEPARATOR and `--`; STOP terminates the stream. NODE is retained; XSECT,
XSECT_END and ERROR_INFO are validated without emitting legs. Unknown records,
CODE_0x… and rewind streams are errors. The parser accepts versions 1–8;
real fixtures and integration cover v3 and v8, not every historical version.

References: [dump3d](https://survex.com/docs/manual/cmdline.htm#dump3d),
[3D v8](https://survex.com/docs/3dformat.htm),
[NODE/MOVE/LINE example](https://trac.survex.com/ticket/112).
