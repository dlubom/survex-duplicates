# Real Survex fixtures

The purpose-built `survey.svx` and `legacy.svx` sources were compiled with real
**cavern 1.4.24**. Dump files were generated with **dump3d 1.4.24**;
`extended.3d` was produced by **extend 1.4.24**. Commands, versions and SHA-256
values are preserved in `provenance.json`.

```bash
python tests/fixtures/regenerate.py
```

Regeneration changes creation timestamps and hashes. Ordinary parser tests use
the saved dumps without Survex; tests marked integration additionally read
real binary files. The Polish title and Unicode test labels are intentional
input data and are preserved verbatim.

- `survey`: v8 LEG and MOVE/LINE variants of the same model, no CS, CARTESIAN
  style, a single date and date range, SURFACE/DUPLICATE/SPLAY, anonymous NODE,
  dated XSECT and XSECT_END. Survey `p],"q[` demonstrates unescaped brackets,
  commas and quotes in survey and full station names.
- `legacy`: v3, empty root survey `[]`, no STYLE or shot dates, text creation
  date rather than `@timestamp`.
- `closure`: v8 from the same source, NORMAL style and real ERROR_INFO.
  Omitting `--no-auxiliary-files` is intentional: that option also omits this
  information from the model. The auxiliary `.err` is removed after generation.
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
