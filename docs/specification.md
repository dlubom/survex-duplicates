# Survex geometric duplicate detector specification

Approved design: 2026-10-25. Publication amendment: legs carrying DUPLICATE
are excluded by default; `--include-duplicates` restores the original all-flags
audit. [ADR 0003](adr/0003-exclude-acknowledged-duplicates.md) records the owner's
subsequent decision. Historical 220/62 registry results describe that explicit audit.
Runtime amendment (2026-10-26): ADR 0004 makes Go the primary implementation
while retaining the original Python implementation as a verification reference.
This English specification replaces the original Polish document for publication.

## Purpose and scope

Accept one ordinary spatial Survex `.3d` model or its complete text dump.
Find every unordered pair of different eligible leg records whose endpoints
satisfy the exact geometric criterion below, allowing reversal. All caves,
surveys and station names participate in one coordinate system. Preserve the
input and emit an ordered CSV plus an execution summary.

Record identity is independent of geometry and metadata. Three eligible
identical records produce three pairs. DUPLICATE legs are omitted unless
explicitly included. SPLAY legs are included unless `--exclude-splays` is set.
A flag combination is excluded if either active filter excludes it; count the
union once. SURFACE alone does not exclude a leg. Read and validate the whole
input, including excluded legs, and retain every NODE for station-name lookup.

Point legs are valid. In relative mode a pair containing a point leg matches
only if both are points at exactly the same position. Absolute mode applies
its endpoint radius normally. There is no minimum relative tolerance floor.

The scope is geometry already present after adjustment. Survex can average
adjacent repeated readings before creating the model; the detector cannot
recover lost source rows. A geometric match is a candidate for review, not
proof of an error or station identity. Automatic deletion, representative
selection, clustering, cave identification, translated/rotated-model matching,
reprojection and a GUI are outside scope.

References: [Survex programs](https://survex.com/docs/manual/cmdline.htm),
[repeated readings](https://survex.com/docs/manual/svxhowto.htm#repeated-readings),
[3D format](https://survex.com/docs/3dformat.htm),
[coordinate systems](https://survex.com/docs/manual/datafile.htm#cs), and
[flag semantics](https://survex.com/docs/manual/datafile.htm#flags).

## Runtime and CLI

Go 1.26 module, standard library only; release and CI builds use the pinned
Go 1.26.3 toolchain. Keep reader, exact geometry, streaming search,
report and CLI responsibilities separate. Use math/big for exact integer/rational
arithmetic and preserve the reference decimal formatting. Record tested versions.
The preserved Python 3.12+/NumPy/SciPy implementation, pytest and independent
Python-integer oracle are development verification tools, not runtime dependencies.
Survex is an external dependency only for binary input; reading a saved dump
must work without Survex installed.

```bash
go run . model.3d --out duplicates.csv
go run . model.3d --relative-percent 5 --exclude-splays --out without-splays.csv
go run . model.dump --epsilon 0.05 --out absolute.csv
go run . model.3d --include-duplicates --out full-audit.csv
```

| Argument | Contract |
| --- | --- |
| INPUT | One existing file. |
| --relative-percent | Positive finite decimal percentage of the shorter leg; default 5 if neither tolerance argument is supplied. |
| --epsilon | Positive finite decimal endpoint tolerance in metres; explicitly selects absolute mode. |
| --include-duplicates | Include every DUPLICATE leg; disabled by default. |
| --exclude-splays | Exclude every SPLAY leg before indexing; disabled by default. |
| --out | Required CSV path. |
| --input-format | auto, 3d or dump; default auto. |
| --dump3d | Executable path; default lookup of dump3d in PATH. |
| --batch-size | Positive integer; default 1. |
| --force | Allow replacing an existing result after successful processing. |
| --help | Explain both endpoint tolerances, units, examples, flags and adjusted-model limitations. |
| --version | Print executable version and source revision without requiring input or output. |

Explicit tolerance arguments are mutually exclusive. Accept decimal scientific
notation exactly. Reject zero, negatives, NaN and Infinity. Default 5% must not
conflict with explicit epsilon, and relative mode has no hidden centimetre cap.
Auto mode detects the `Survex 3D Image File` signature; otherwise the input must
validate as a full dump. The filename extension is not evidence of its format.
Exit 0 means success, including no matches; 2 means invalid arguments; 1 means
input, subprocess, computation or write failure. Diagnostics and summary use stderr.
Help and version output use stdout. Ordinary unversioned source builds report
`dev (unknown)`; release packaging embeds the intended version and source revision.
All detector-authored documentation and diagnostics are in English. Errors
from external tools retain their original diagnostic text.

## Reader contract

### Binary model

Invoke `dump3d --legs --show-dates INPUT` as a list without a shell. Paths with
spaces must work. Remove inherited LC_ALL and set LC_NUMERIC=C while preserving
other locale settings. Stream stdout and drain stderr without a pipe deadlock.
Check the final return code: valid partial stdout plus nonzero exit invalidates
the entire read. Missing executable and missing required options need clear errors.
Reject extended elevation with a request for an ordinary spatial model.

The selected, tested generator is Survex 1.4.24. `--legs` exposes both endpoints
in each LEG record and is recommended for scripts by the
[dump3d documentation](https://survex.com/docs/manual/cmdline.htm#dump3d).

### Full text dump

Support two geometry forms: LEG contains both endpoints; MOVE sets the current
position and LINE emits a leg to its endpoint, then advances the position.
LINE before MOVE is an error. Consecutive MOVE records do not emit legs.
Reject a mixture of LEG and LINE. NODE records between MOVE and LINE do not
change the current position. Empty `[]` names and missing STYLE are legal.

Preserve survey, sorted distinct flags, optional style and the leg's original
survey date or date range. Header creation time is not every shot's date.
Survey is not a station name. Missing metadata is legal.

Require a valid ordered header and STOP; reject truncation, repeated rewind
streams and data after STOP. A valid model without legs is legal. The tested
header is TITLE, DATE, DATE_NUMERIC, optional CS, VERSION, optional standalone
EXTENDED ELEVATION, SEPARATOR and `--`. Missing CS is legal. The parser accepts
versions 1–8; real fixtures verify versions 3 and 8. Other historical versions
have not been independently tested.

After LEG coordinates, parse `[survey]`, optional STYLE, flags and a bare date.
Default `--show-dates` uses `YYYY.MM.DD` or `YYYY.MM.DD-YYYY.MM.DD`; validate
calendar dates and range order. No date is legal. Alternative date formats
need real fixtures before being supported; `dump3d -D` is currently unsupported.
Names can contain `[`, `]`, commas, quotes and hierarchy separators without
escaping: survey `p],"q[` appears as `[p],"q[]`. The first closing bracket is
not the end of the name. Apply the same rule to NODE and XSECT names.

NODE is retained for reporting; it is not the binary record type LABEL.
XSECT, optionally dated, XSECT_END and ERROR_INFO are explicitly validated
and emit no legs. Unknown records, malformed geometry, `<<< REWIND <<<`
and CODE_0x… are errors, never silently discarded. Errors identify dump line numbers.
The [published NODE/MOVE/LINE example](https://trac.survex.com/ticket/112)
is additional format context.

Before closing parser development, generate and preserve real binary, LEG
and MOVE/LINE fixtures with the actual supported Survex version. Record
commands, hashes and generator version. Confirm headers, field order, flags,
styles, date suffixes and termination from those files. Parser tests consume
saved dumps without requiring Survex; real binary integration is separate.

## Data model, identity and station names

Store each endpoint as a three-element int64 point after checking each
coordinate is a valid signed int32 centimetre value. The Python reference
retains int64 NumPy arrays `A_cm` and `B_cm` of shape `(n, 3)`. Parse metres
exactly; multiplying by 100 must produce an integer. Reject off-grid values
instead of silently rounding. The supported spatial axes are East, North, Up.

Assign `leg_id` from 1 in the order of all input legs before filtering. IDs may
have gaps in the analysed subset and must never be renumbered. Retain one-based
`source_line` in the full text dump, including a dump generated from binary;
it is not an original SVX line number. Keep `survey`, alphabetically ordered
unique flag names, optional `style` and original `survey_date` per record.

After reading the entire dump, attach names only by exact NODE centimetre
coordinates. Retain all distinct nonempty full names at each endpoint in
lexicographic order. Do not select a nearby station, one arbitrary alias or a
name inferred from survey. Several labels at one position do not prove which
a source reading used. Retain coexistence of named and anonymous NODE records.
An empty name or ANON flag means the anonymous indicator is true, even if names
also exist. Name availability is independent of both selection filters.

For human interpretation, explicitly distinguish an unnamed station from a
missing station record, retaining coordinates, survey, identity and the other
endpoint's available names. Neither status means invalid geometry. Identifying
an exact original SVX/SRV row is outside the available input contract.

## Exact predicate

For leg endpoints A_i, B_i and A_j, B_j in metres:

```text
D(i,j) = min(max(norm(A_i-A_j), norm(B_i-B_j)),
             max(norm(A_i-B_j), norm(B_i-A_j)))
L_i = norm(B_i-A_i)
```

Relative mode accepts exactly the eligible i<j pairs with
`D <= (relative_percent/100) * min(L_i,L_j)`. Absolute mode accepts exactly
those with `D <= epsilon`. Both matched endpoints must satisfy the limit.
A midpoint, intersection, one endpoint or mean error is insufficient.

Parse the tolerance as an exact rational decimal. Qualify in squared cm:

```text
length_i_sq = squared_distance_cm(A_i,B_i)
length_j_sq = squared_distance_cm(A_j,B_j)
relative: threshold_sq = floor(rho² * min(length_i_sq,length_j_sq))
absolute: threshold_sq = floor((100*epsilon)²)
s_AA = squared_distance_cm(A_i,A_j)
s_BB = squared_distance_cm(B_i,B_j)
s_AB = squared_distance_cm(A_i,B_j)
s_BA = squared_distance_cm(B_i,A_j)
direct_sq = max(s_AA,s_BB)
reverse_sq = max(s_AB,s_BA)
match = min(direct_sq,reverse_sq) <= threshold_sq
reversed = reverse_sq < direct_sq
```

Ties are direct. Retain the two separate squared endpoint errors in the chosen
orientation. At 5%, equivalently test
`400 * min(direct_sq,reverse_sq) <= min(length_i_sq,length_j_sq)`.
Do not use the longer/mean length or a floor radius for point legs.

The predicate must not overflow. Subtract validated int32 coordinates in int64,
then square and sum with math/big integers; int64 cannot hold every squared-distance
sum across the full int32 range. The independent Python oracle converts
coordinates to Python int before subtraction and squaring. A vector optimisation needs a proven safe range plus an
exact fallback. No arbitrary numeric epsilon or rounded CSV value may decide
a match. Floating roots are only for accepted-result formatting; integer isqrt
is allowed for index radii.

## KDTree search and streaming

Use the orientation-independent bounding key in centimetres. This reference
notation defines the key; the Go implementation stores it as six int64 values:

```python
F_cm = np.concatenate([np.minimum(A_cm, B_cm), np.maximum(A_cm, B_cm)], axis=1)
tree = KDTree(F_cm.astype(np.float64))
```

The six coordinates are component-wise minima followed by maxima. Lexicographic
sorting of the real endpoints is not an equivalent key. A matching pair has
coordinate differences within tolerance in F; use the maximum norm `p=np.inf`.
The default 6D Euclidean norm could miss valid endpoint matches.

Query i at `ceil(rho * sqrt(length_i_sq))` centimetres in relative mode, or
`ceil(100*epsilon)` in absolute mode. The pair predicate still uses the shorter
length; per-query radius is valid regardless of leg order. Round without float:
for exact rational radius square q, return `2**32` if `q >= (2**32)**2`;
otherwise set `k=isqrt(floor(q))` and increment if `k*k < q` exactly.
For q=0 return 0. Go queries use this integer directly; only the Python/SciPy reference converts
the final integer to float for KDTree.
This is `min(ceil(sqrt(q)),2**32)` and covers every possible int32 coordinate
difference. The outward rounding may add candidates, rejected by the predicate.

```python
neighbors = tree.query_ball_point(F_cm[i].astype(np.float64),
    r=query_radius_cm[i], p=np.inf, eps=0.0, return_sorted=True)
```

The Go KDTree performs exact integer maximum-norm queries. In the Python
reference, SciPy eps=0.0 disables approximate searching; it is not user tolerance.
The Go median-partition implementation sorts partitions during construction,
which costs O(n log² n); this is not a guarantee about query complexity.
Process selected legs in original ID order, preserve an explicit mapping
between compact tree rows and original records, retain only neighbours with
higher IDs, and evaluate the exact predicate. Emit pairs ordered by `(leg_a,leg_b)`.
Batching uses the appropriate radius for every query leg and must return every
neighbour. There is no k-nearest-neighbour cap or hidden output truncation.

No n-by-n matrix, global candidate-pair list, full output list or production
`query_pairs`. Stream accepted rows to the temporary CSV. One query may return
n neighbours: memory is O(n + NODE storage) at batch 1, with potential O(B*n)
neighbour-list storage for batch B. K can reach n(n-1)/2. Do not claim an
O(n+K) guarantee for this KDTree; measure actual performance.

Summary fields: n_input, union count n_excluded, n, unordered candidate count C
after the larger-ID condition, K, exact tolerance mode/value/unit, both filter
settings, batch size, detected format and read/build/search-with-write times.

## CSV report

UTF-8, comma delimiter, a header even for zero matches, minimal CSV quoting
and CRLF record endings compatible with the preserved Python csv.writer. Empty metadata is an empty field; reversed and boolean indicators are
0/1. Column order:

```text
leg_a,leg_b,survey_a,survey_b,source_line_a,source_line_b,distance_m,endpoint_1_error_m,endpoint_2_error_m,reversed,flags_a,flags_b,style_a,style_b,date_a,date_b,a_x1,a_y1,a_z1,a_x2,a_y2,a_z2,b_x1,b_y1,b_z1,b_x2,b_y2,b_z2,distance_cm,length_a_m,length_b_m,relative_error_percent,tolerance_mode,tolerance_value,tolerance_m,a1_station_names,a2_station_names,b1_station_names,b2_station_names,a1_station_status,a2_station_status,b1_station_status,b2_station_status,a1_has_anonymous_node,a2_has_anonymous_node,b1_has_anonymous_node,b2_has_anonymous_node
```

- `leg_a < leg_b`, rows increasing by that pair.
- `distance_m=sqrt(min(direct_sq,reverse_sq))/100`; distance_cm is the same
  error in centimetres. Endpoint errors use the chosen orientation and the
  first and second endpoints of leg_a respectively.
- Lengths are in metres. Relative error is `100*D/min(L_a,L_b)`; leave it
  empty when the shorter length is zero, including when D=0. No NaN/Infinity.
- `tolerance_mode` is `relative_percent` or `absolute_m`. `tolerance_value`
  is the exact decimal parameter (default `5`). `tolerance_m` is the actual
  pair tolerance before squared-integer truncation, not the rounded tree radius.
  Allow scientific notation and use exact decimal formatting with sufficient range so a large
  valid parameter does not become float Infinity or a formatting error.
- Preserve original endpoint order in all coordinates and a1/a2/b1/b2 names;
  reversal affects matching, not these columns. Coordinates are metres with
  two decimals. Format distances with enough precision to reconstruct float64,
  for example 17 significant digits.
- Join alphabetically sorted flags using `|`. Dates are shot dates, not model
  creation dates. source_line refers to the dump, not original SVX.
- Names are JSON arrays of all distinct nonempty, lexicographically sorted full
  names, with Unicode unescaped. Standard CSV quoting handles the outer layer.
  Do not join names using a character that might be part of a name.
- Missing names are `[]`, never a blank cell. Status is `named` if names exist,
  `unnamed` if NODE exists without an available name, or `missing_node` if no
  NODE exists. Explain the latter two in human documentation.
- `*_has_anonymous_node` is 1 when a node has an empty name or ANON flag.
  It may coexist with names and named status and does not select a source alias.

Write a temporary file in the destination directory. Publish only after all
processing succeeds, preserving an existing report and removing the temporary
file on failure. Without --force, an existing output is an error, including a
file created while processing runs. Output must not refer to input through
its direct path, symlink or hardlink.

The Go writer flushes and syncs the temporary file before publication. Normal
publication requires filesystem hardlink support; `--force` uses the operating
system's rename operation. Atomic replacement is not guaranteed on every
platform, and file sync does not guarantee directory-entry durability after a
crash. See [platform filesystem behavior](platforms.md#output-filesystem-behavior).

## Acceptance tests

### Absolute mode

All coordinates below are centimetres and use explicit epsilon=0.05 m.
Each row is independent; no SPLAY filtering is implied.

| Case | First leg | Second leg | Result |
| --- | --- | --- | --- |
| Identical | (0,0,0)→(100,0,0) | (0,0,0)→(100,0,0) | one pair, errors 0/0, direct |
| Reversed | (0,0,0)→(100,0,0) | (100,0,0)→(0,0,0) | one pair, errors 0/0, reversed |
| 3–4–5 boundary | (0,0,0)→(100,0,0) | (3,4,0)→(103,4,0) | one pair, errors 5/5 cm |
| False candidate | (0,0,0)→(100,0,0) | (5,1,0)→(100,0,0) | no pair; sqrt(26)>5 cm |
| Same min/max key | (0,0,0)→(100,100,0) | (0,100,0)→(100,0,0) | no pair |
| Z coordinate | (0,0,0)→(100,0,0) | (0,0,5)→(100,0,5) | pair; shift by 6 cm does not match |
| Better reversed orientation | (0,0,0)→(2,0,0) | (2,1,0)→(0,1,0) | reversed, errors 1/1 cm |
| Orientation tie | (0,0,0)→(1,0,0) | (5,0,0)→(3,4,0) | direct, errors 5/sqrt(20) cm |
| Point legs | (0,0,0)→(0,0,0) | (3,4,0)→(3,4,0) | direct, errors 5/5 cm |

Also verify three eligible identical records give exactly (1,2),(1,3),(2,3).
Parallel legs shifted transversely by 0/4/8 cm give only (1,2),(2,3), without
transitive closure. Changing surveys does not affect geometry; flags affect
only explicit selection. Check negative coordinates and zero crossings,
translation by (100000000,100000000,100000000) cm, and different int32-extreme
square diagonals with the same F key (rejected without overflow).
Epsilon 0.0099 m permits only zero error on the cm grid; 0.01 m permits 1 cm.
An input coordinate of 0.001 m is an error. Empty and single-leg inputs return
zero pairs. Every allowed batch size produces identical CSV bytes.
Regression: (-2,0,0)→(2,100,0) and (2,0,0)→(-2,100,0) match despite reversal
of lexicographic endpoint order.

### Relative mode

Verify these cases with the predicate and full search:

1. At 5%, (0,0,0)→(100,0,0) and (3,4,0)→(103,4,0) match at the boundary;
   a shift (0,0,6) does not.
2. 1000 cm legs shifted by (30,40,0) match at 5%, but not at absolute 5 cm.
3. A 4 cm leg and its reversal match at 5%; shifting both endpoints by
   (0,1,0) does not, although absolute 5 cm would accept it.
4. At 10%, (0,0,0)→(100,0,0) and (-2,3,0)→(110,3,0) do not match in
   either record order: sqrt(109) exceeds the shorter-length limit of 10 cm.
5. (0,0,0)→(100,100,0) and (5,5,0)→(105,105,0) match exactly at 5% for
   an irrational length; 4.9999% rejects them without float qualification.
6. Coincident point legs match; point legs 1 cm apart and a point/nonzero
   pair do not. Reports have no divide-by-zero or non-finite relative errors.
7. Preserve reversal, tie rules, record identity, non-transitivity and translation
   behaviour, including full int32 extremes.
8. Exercise query-radius rounding below/on/above an integer square, zero and
   the 2**32 cap. No oracle match may disappear.
9. Query longer legs before shorter ones and the reverse, with mixed radii in
   batches; complete results and ordering remain identical.

### Selection and CLI

For identical records with IDs 1 (unflagged), 2 (SPLAY), 3 (DUPLICATE), the
normal report has (1,2); with --exclude-splays it has no pairs. With explicit
--include-duplicates it has all three pairs; combining that with --exclude-splays
leaves (1,3). IDs and source lines always retain their input values.

Test combined SPLAY/SURFACE/DUPLICATE flags, filter union counts, unflagged
unnamed legs, nodes associated with excluded legs, and malformed excluded
records that invalidate the full input. When every leg is excluded, emit a
header-only CSV with correct n_input/n_excluded/n/K and status 0.
Default tolerance equals explicit --relative-percent 5 for each flag setting;
--epsilon selects only absolute mode. Invalid numbers, nonpositive parameters,
invalid batches and both tolerance arguments produce status 2.
Both tolerances, both switches and multiple batches must agree with the oracle
and produce deterministic user-visible output.

### Reader, process and output safety

Use a real binary and both of its dump forms; geometry, metadata and matches
must agree, allowing different dump line numbers. Test NODE between MOVE/LINE,
consecutive MOVE, empty names, missing STYLE, and model creation time not leaking
into shot dates. Unknown records, wrong coordinate counts, malformed numbers,
missing STOP, rewind and extended elevation must fail with useful diagnostics.

Test a missing dump3d executable, unsupported options and failure after valid
partial stdout. Test large stderr without blocking; preserve locale except the
specified numeric settings. No failed process may publish a report.
Use paths with spaces/Unicode and names containing brackets, commas, quotes
and hierarchy separators. Test NODE before/after geometry, several aliases,
named-plus-anonymous, anonymous-only and missing NODE positions.
Existing reports without --force stay unchanged; injected write/publication or
processing failures leave no complete-looking new report or temporary file.
A real dump3d integration can skip without Survex, but must be executed at least
once before binary input is claimed as verified. Distinguish executed and skipped checks.

## Independent correctness and benchmarks

A test-only exhaustive i<j oracle computes all four squared endpoint distances,
orientation, both squared errors, both lengths and the exact mode threshold
independently. It uses neither the production predicate nor KDTree. It selects
legs using the requested flags and retains original IDs. Compare complete
ordered tuples, not counts, on fixed-seed small sets with duplicates, reversals,
cm-grid data and boundaries, plus the 512-leg registry sample.

Provide reproducible 50,000- and 100,000-leg sparse and deliberately repeated
cases, plus a smaller dense case. Include varied lengths, both tolerance modes,
both DUPLICATE policies and explicit SPLAY state. Record generator, seed,
parameters, n_input, n_excluded, n, C, K, batch size, hardware, system, library
versions, per-stage times and process peak memory. A repeated set's expected
result comes from construction and smaller independent-oracle checks. Never
invent a time limit or extrapolate random-data performance into a general guarantee.

The bundled registry has binary SHA-256
`55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077`.
At 5%, normal selection has n_input=18584, n_excluded=2, n=18582 and K=218;
with --exclude-splays: n_excluded=6433, n=12151, K=60. Explicitly including
DUPLICATE restores K=220, or K=62 after excluding 6431 SPLAY legs. Absolute
5 cm gives 103/50 normally and 104/51 with DUPLICATE included.
Check the hash before expectations and compare full pairs/orientations/errors
with the preserved experiment. Different hashes require new independently
established evidence without overwriting the historical record.

## Module responsibilities and completion

| File | Responsibility |
| --- | --- |
| go.mod / main.go | Go module and executable entry point. |
| version.go / VERSION | Development identity and intended release version; release builds embed version and revision. |
| cli_args.go / cli.go | Arguments, coordination, diagnostics and exit status. |
| reader.go | dump3d process, full-stream parsing and validation. |
| models.go | Leg metadata, input model and match/pair records. |
| geometry.go | Exact tolerance, predicate, orientation and endpoint errors. |
| search.go | F key, KDTree and ordered streaming callback. |
| report.go | CSV formatting and atomic publication. |
| *_test.go | Go tests at the approved seams; test-only differential probe. |
| survex_duplicates/ / pyproject.toml / uv.lock | Preserved Python verification reference and locked dependencies. |
| tests/ | Python reference tests, independent oracle and real fixtures. |
| benchmarks/ | Reproducible generators, measurement and full verification. |
| examples/kataster/ | Complete portable registry demo with provenance and license. |
| README.md / AGENTS.md | Human and agent entry points. |

The reader returns ordered records and metadata; the engine streams matches
through a callback; reporting consumes them. Predicate and engine are usable without
Survex processes or input files. Shared private lexical syntax may be factored
without widening the public interfaces.

Completion requires both input forms, every required acceptance behaviour,
exact agreement with the oracle, deterministic complete CSV, actual benchmark
measurements, real fixture provenance, tested dependency versions and clear
instructions/limitations. The published tree and its reachable history must
contain no contributor-specific home-directory paths. Preserve local development
history separately from the clean publication root.
