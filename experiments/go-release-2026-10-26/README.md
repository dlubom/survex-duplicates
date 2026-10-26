# Go release qualification — 2026-10-26

This is the original local verification record for the Go release preparation. It preserves
the earlier [migration measurements](../go-2026-10-26/README.md) unchanged.
The requested release matrix follows PocketTopo Exporter: Linux, macOS and
Windows, each on amd64 and arm64, distributed as six native ZIPs.

The counts, hashes and limitations below describe that qualification snapshot.
The Python verification tools were later retired from the active tree and can
be recovered from the [archived reference checkout](../../benchmarks/README.md#reproduce-the-archived-python-reference)
at immutable commit `b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1`. Subsequent native
CI evidence is recorded in [the platform guide](../../docs/platforms.md).

## Evidence

- [validation.json](validation.json): executed checks, counts, review and limits.
- [ci-b99bef2.json](ci-b99bef2.json): subsequent terminal migration CI and
  independent readback of its downloaded six-platform artifact set.
- [parity.json](parity.json): fresh 160 full CSV comparisons and 320 ordered
  exact-result comparisons, including reader metadata and both endpoint errors.
- [package-qualification.json](package-qualification.json): identities and complete
  manifests of the six qualification archives, including executable/member hashes
  and the exact source-file fingerprints used to build them.

All six targets compiled and passed archive verification. Each ZIP contained
29 files, including the full KATASTER input, eight expected CSVs, instructions
and separate software/data/Go license notices. Independent Python ZIP readback
also checked the actual ELF/Mach-O/PE architecture, all member hashes and modes,
relative Markdown links, sidecars and the complete 14-asset release set.

The macOS ARM64 package was extracted and executed with an empty `PATH`.
Its version/revision matched its manifest; both generated CSVs were identical
to the saved 218-pair and 60-pair reports. All eight combinations were separately
checked through the public Go CLI tests and the Python reference.

The final full Go race run passed 133 tests/subtests. The ordinary-suite skip
is the test-only differential probe, which was executed separately in the
320 comparisons. Python passed 246 tests with no skips. Publication tests used
a local `gh` stub to check 24 scenarios, including corruption, incomplete assets,
tag movement and API failures; they performed no external publication.

## Snapshot boundary and limits

The qualification archives were built from the uncommitted working tree based
on `1143a96292d49457ea344d46410072ecbbcec1af`, as `v1.1.0-local`, and carry
`snapshot: true` and the `-dirty` revision suffix. The manifests preserve the
actual source fingerprints; the baseline commit alone does not identify these
local changes. This evidence directory and the final documentation record were
added after qualification, avoiding a self-referential hash claim.

Final local ZIPs are regenerated in ignored `dist/release-local/` after recording
this evidence, with updated source manifests. Their executable hashes are
checked against the qualified binaries. Archive hashes may differ because
documentation and source-manifest contents changed. The local ZIPs are previews,
not published assets or tagged releases.

Only macOS ARM64 was executed locally for this snapshot. Native execution on
the other five platforms and GitHub-hosted CI for the delivered commit remained
release gates at that point. The workflow runs those checks before publishing;
configuration and successful cross-builds do not establish that remote runtime
tests have passed.

Cold Go test compiles encountered insufficient disk space. Task-created caches
and regenerable local artifacts were cleaned, and the final full race command
completed successfully. The packaging integration also found and corrected a
Homebrew layout assumption: the Go license is now a pinned repository file,
and a regression test opens every required bundle member.

Current Go packaging commands and publication requirements are in
[the release guide](../../docs/releasing.md); use the archived checkout to replay
the Python comparisons described here. No inputs or historical numerical
results were changed. Software remains MIT licensed; survey-derived records
retain the attribution and CC BY-SA terms in [NOTICE.md](../../NOTICE.md).
