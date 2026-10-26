# Python retirement implementation plan

**Goal:** Retire the Python runtime and active tooling after the complete
six-platform migration CI passed, preserving independent Go verification and
all historical input/result bytes.

**Architecture:** The Go detector and rational exhaustive oracle remain the
active implementation and verification seams. A standard-library Go publisher
replaces the Python publication script with the same draft/readback gates.
Historical comparison code is reproducible at commit
`b99bef2f7cda6ccc97432fd32f70ded9ea0a2de1`.

**Tech stack:** Go 1.26.3 standard library; Survex for binary-input integration;
GitHub CLI only for the publication maintenance command.

**Specification:** docs/specification.md and the retirement decision in ADR 0005.

## Constraints

- Preserve the six native ZIP targets and every publication failure gate.
- Preserve all fixture, registry, CSV and numerical historical JSON bytes.
- Keep the exhaustive oracle independent of production geometry and KDTree.
- No release tag or public release is part of repository cleanup.

## Tasks

- [x] Verify terminal CI for the exact pushed source revision, including six
  native tests, six downloaded package smoke runs and complete-set assembly.
- [x] Add focused Go regression tests for the 512-leg independent oracle,
  fixture hashes, cross-format reports and failed CLI publication.
- [x] Implement and test `cmd/publish` and `internal/releasepublish` against all
  prior offline publication failure scenarios; keep `gh` injectable in tests.
- [x] Remove the obsolete Python program, tests, dependencies and active scripts;
  replace workflow checks and retain immutable reproduction instructions.
- [x] Update current docs/agent instructions and record the successful migration CI.
- [x] Run native Go tests/race/vet/format, actionlint and archive verification;
  review the cleanup and ensure snapshot hashes remain unchanged.

## Review focus

Loss of independent checks; historical reproduction links after file deletion;
binary input silently losing coverage; publisher making a release public before
readback; generated caches or local output accidentally included in delivery.
