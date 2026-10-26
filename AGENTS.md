# Repository instructions

## Requirements and terminology

Before changing parsing, geometry, filtering or CSV behaviour, read
[docs/specification.md](docs/specification.md) and the relevant decisions in
[docs/adr/](docs/adr/). Use [GLOSSARY.md](GLOSSARY.md) for domain terms.
ADR 0003 supersedes the initial all-flags policy: `DUPLICATE` legs are excluded
by default and `--include-duplicates` explicitly enables the full audit.

## Implementation and tests

Keep the existing module responsibilities and small public interfaces.
The agreed TDD seams are:

1. Reader → ordered geometry and metadata: formats, validation, dates, flags and names.
2. Public geometry predicate → match: exact tolerances, orientation and squared errors.
3. Search engine → ordered pair iterator: completeness against the independent oracle.
4. CLI → status, stderr and saved CSV: arguments, filters, identity and atomic publication.

Write tests through these interfaces. Keep the oracle independent of the
production predicate and KDTree. Qualification uses exact centimetre arithmetic:
use arbitrary-precision squares in Go and convert NumPy coordinates to Python
integers in the independent oracle. Streaming production code retains only the
current query batch, not all candidates or results.

## Data and portability

Treat registry and fixture inputs as read-only snapshots. Verify their hashes
before using saved expectations. Use the bundled example for integration;
`KATASTER_PATH` is an optional override, not a required local checkout.
Preserve numerical historical evidence. Record metadata redactions and new
measurements separately, as described in the experiment's provenance.

Use repository-relative paths in documentation and saved artifacts. Keep
instructions, comments and diagnostics in English; preserve proper names,
original survey labels and intentional Unicode test data.

## Validation and packaging

Run the development checks in [README.md](README.md#development-and-verification)
after behaviour changes. Check complete pairs, orientation and both endpoint
errors when altering search or selection. Report executed checks and skips.
For data updates, follow [examples/kataster/README.md](examples/kataster/README.md).
Keep the MIT software license and CC BY-SA survey-data notices distinct.
For release changes, follow [docs/releasing.md](docs/releasing.md); verify all
six archive contents and execute the packaged demo on each native CI target.
