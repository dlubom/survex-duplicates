# Go release preparation plan

**Goal:** Complete verification and documentation for the Go runtime, retain the
full KATASTER example, and let GitHub Actions build and publish reviewed tagged
versions for Windows, macOS and Linux on x64 and ARM64.

**Scope:** The user authorized the Go transition, comprehensive checks, updated
documentation and a release workflow. Prepare and verify locally before any
publication decision. Preserve all existing working-tree migration changes,
survey inputs, historical measurements and the Python reference.

**Architecture:** A checked target manifest, standard-library Go packaging tool,
reusable checks and one workflow orchestrating checks, cross-builds, archive
readback, native smoke tests and publication. Tags publish; branch/PR/manual
snapshot runs produce artifacts only. No third-party runtime dependency in the
native detector. The six native ZIP packages follow the current
PocketTopo Exporter release matrix requested by the user.

## Choices

Use one archive per target, with a self-contained quick start, documentation,
licenses and complete KATASTER model/dump/reports. This makes each download
directly usable. A separate example download would save repeated data but make
the requested demonstration harder to run. A small Go release tool keeps build
and verification logic testable without adding a release-framework dependency.

The manifest contains exactly six targets: Linux, Windows and macOS, each on
amd64 and arm64. Every package uses ZIP and includes the full example. Each
GitHub Actions runner builds and executes its own target. The pinned Go
compiler must support every selected target; other compiler ports are outside
this release scope.

## Tasks

- [x] Validate the six selected targets against the pinned toolchain.
- [x] Add version reporting and full registry CSV tests in Go.
- [x] Implement deterministic archives, metadata/checksums and independent archive
  verification; include the entire bundled KATASTER demo and license notices.
- [x] Add native checks for Linux/macOS/Windows on amd64 and arm64, accounting
  for race-detector support; keep real dump3d/Python oracle verification.
- [x] Add gated build/release workflow, including extraction and execution of
  downloaded archives, complete artifact-set verification and tag/revision checks.
- [x] Review documentation: download/build/run instructions, platform selection,
  explained real CSV examples, filtering, tolerance, reversed matches and limitations.
- [x] Run local tests, vet, formatting, fixture hashes, differential verification,
  release-tool tests, archive readback and the full cross-build matrix sequentially.
- [x] Independently review code, workflow, docs and delivered artifacts; record
  executed checks and unexecuted remote/runtime checks without overclaiming.

## Review focus

Wrong-architecture archives; missing data or notices; inconsistent version/SHA;
partial/duplicate release assets; unsupported runtime vs merely successful build;
source changes caused by line-ending conversion; paths escaping an archive;
publishing before all checks finish; commands that fail after unpacking.
