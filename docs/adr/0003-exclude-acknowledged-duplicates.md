# Exclude acknowledged duplicate legs by default

Decision: 2026-10-25, requested by the repository owner before publication.
This amends the initial all-flags default without changing the geometry predicate.

Survex uses `DUPLICATE` for deliberately repeated surveys along the same
passage, for example to tie surveys into a permanent station. The detector's
normal report should prioritise repetition that has not already been acknowledged.
[Survex flag documentation](https://survex.com/docs/manual/datafile.htm#flags).

Exclude every leg carrying `DUPLICATE` before building the index. A pair
containing such a leg is absent from the default report. The explicit
`--include-duplicates` option restores the full geometric audit, including
the original experimental results. `--exclude-splays` remains independent;
both filters remove their union and an overlapping leg is counted once.
IDs are assigned before filtering and the reader still validates every record
and retains all station names.

On the preserved full registry snapshot, two legs have `DUPLICATE`:
the default 5% report has 218 pairs, or 60 with `--exclude-splays`.
With `--include-duplicates` these counts remain 220 and 62. The two acknowledged
Mietusia repeat-survey pairs are excluded by default; the six Wysoka pairs remain.

The flag expresses author intent. It does not prove that every geometric
match is a redundant observation, and this detector does not edit survey data.
