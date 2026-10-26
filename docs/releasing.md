# Building and publishing a release

This guide describes the six-platform Go release workflow. `VERSION` contains
`1.1.0`; the corresponding release tag is `v1.1.0`. These are preparation values,
not a claim that the version has been published or its remote checks passed.
The [platform guide](platforms.md) lists the six native ZIP packages.

Run maintenance commands below from a source checkout, with Git and the pinned
**Go 1.26.3** toolchain. Application users only need their extracted executable.
The detector, tests, packaging and publication tools use Go. Historical reports
remain in the repository; the former Python implementation and lockfile are
available at the fixed pre-cleanup commit described in
[ADR 0005](adr/0005-retire-python-reference.md).

## Build a local snapshot

First run the [development checks](https://github.com/dlubom/survex-duplicates#development-and-verification).
Then inspect the validated target matrix and build one package:

```bash
go run ./cmd/release matrix
go run ./cmd/release build --target darwin_arm64 --version v1.1.0-local --out dist --snapshot
go run ./cmd/release verify --archive dist/survex-duplicates_v1.1.0-local_darwin_arm64.zip
go run ./cmd/release smoke --archive dist/survex-duplicates_v1.1.0-local_darwin_arm64.zip
```

Choose your own target for `build` and the matching filename for the following
commands. `verify` reads an archive without executing its binary, so it can
check a package for another operating system. `smoke` extracts and executes
the package and requires a compatible native host.

`build` writes a ZIP and a `<archive>.sha256` sidecar. It verifies the preserved
demo hashes before packaging. The output directory must be outside the source
tree or ignored by Git; `dist/` is the normal location. Do not edit source files
while building a package or assembling a set of packages.

`--snapshot` permits an untagged or dirty checkout and marks the manifest
accordingly. The requested version must be `v` plus `VERSION`, optionally
followed by a prerelease suffix such as `-local`. A dirty checkout records the
HEAD revision with `-dirty` appended and fingerprints its actual source files.
A snapshot is not a tagged release, even if it has the same numeric version.

## Verify the package and complete set

Archive verification checks file paths, expected contents, hashes, binary
target/build information and the bundled KATASTER snapshot. The archive
contains `manifest.json` with version, revision, toolchain, target, snapshot
status, source fingerprints and bundled-file digests. Its root README is
generated from the package template using that version and target.

The smoke test exercises the extracted executable and version reporting, then
compares complete CSV bytes for the bundled default **218-pair** report and
the **60-pair** report with SPLAY excluded. The Go tests separately verify all
eight tolerance/filter combinations. Running the extracted program requires
no Go, Python or Survex installation; the maintenance smoke command itself uses Go.

After all six packages have been built from the same source snapshot, assemble
them into one verified asset set. For example, set `release_revision` to the
exact `revision` recorded in the package manifests, including `-dirty` when
present, then run:

```bash
go run ./cmd/release assemble --dir dist --version v1.1.0-local --revision "$release_revision" --snapshot
```

In PowerShell, the same example uses a `$release_revision` variable containing
that manifest value. Keep this directory dedicated to one version and one
source snapshot. Before assembly it must contain only the six ZIPs and six
checksum sidecars; use a fresh staging directory to repeat assembly after its
metadata files have been written. Assembly rejects missing or unexpected assets, incorrect
checksums, inconsistent version/revision/snapshot metadata and differing source
fingerprints. It produces `SHA256SUMS` and `release-manifest.json` for the complete
set. These metadata files list the six archives and their hashes; the published
asset set also contains the six individual checksum sidecars.

## Tagged release requirements

A release build omits `--snapshot`:

```bash
go run ./cmd/release build --target darwin_arm64 --version v1.1.0 --out dist
```

This requires all of the following:

- `VERSION` is `1.1.0`, matching the requested `v1.1.0`;
- the working tree is clean, including nonignored untracked files;
- the existing tag `v1.1.0` resolves to HEAD;
- the pinned Go toolchain and target manifest validate;
- the bundled input, license and expected-report hashes still match provenance.

Build every target from the same reviewed revision. Use the clean commit SHA
as `--revision` during assembly, omit `--snapshot`, and retain the verified
manifests and checksums. A local package build does not create a GitHub release.

Land the reviewed workflow changes on the default branch (`main`) before
pushing its release tag. GitHub restricts release creation using `GITHUB_TOKEN`
when the target commit changes workflow files relative to that branch; see the
[release API requirements](https://docs.github.com/en/rest/releases/releases#create-a-release).

## GitHub Actions flow

The workflow separates verification from publication. Check jobs and package
jobs may run in parallel; assembly waits for both groups to finish successfully:

1. Run Go checks on native Linux, macOS and Windows runners for both amd64 and
   arm64. Use the race detector where the pinned Go toolchain supports it.
   Run the independent Go oracle, all eight KATASTER CSV comparisons and real
   `dump3d` integration on Linux. Validate workflow syntax with pinned actionlint.
2. Build all six ZIP packages and verify their contents. Transfer them as
   workflow artifacts, then extract and execute each package on its matching
   native runner.
3. Assemble and verify the exact complete asset set from one version and
   source revision. Missing, extra or inconsistent packages fail the run.
4. For an eligible version tag only, create a draft GitHub release and upload
   the verified assets. Download those assets again and check the exact set
   and checksums before making the release public.

Ordinary branch pushes, pull requests and manual snapshot runs produce workflow
artifacts only. They do not publish a release. Version-tag publication must wait
for all required checks, builds, archive verifications and native smoke tests.
The configuration itself is not a passing CI result: inspect the terminal
statuses and source SHA of the actual run before treating a version as ready.

If upload or independent readback fails, the draft must remain unpublished.
Do not substitute locally rebuilt files into a partially verified release set;
repair the failure and verify a complete set with consistent provenance.

The publication job invokes `go run ./cmd/publish` with the GitHub CLI (`gh`)
available. It receives `GH_TOKEN`, `GH_REPO`, `RELEASE_VERSION`,
`RELEASE_REVISION` and `RUN_URL` from the workflow environment; do not put tokens
in files or command arguments. It saves publication/readback evidence to
`build/release-publication.json`. Its tests replace GitHub commands with an
offline stub and cover failure before publication and uncertain publication
responses. No Python interpreter is needed for this workflow.

Evidence schema 2 records `phase`, version, revision, workflow URL,
`draft_created`, `publication_attempted` and `publication_state` even on failure.
The `published` field is deliberately nullable:

| publication_state | published | Meaning |
| --- | --- | --- |
| `not_attempted` | `false` | The public-transition command was not invoked. |
| `unknown` | `null` | It was invoked, but its response did not confirm success. |
| `confirmed` | `true` | The public-transition command confirmed success. |

Only `phase: verified` confirms that subsequent release and tag checks also
passed. For `unknown`, inspect the current remote release before retrying; a
lost response may follow a successful publication. A failed postpublication
check leaves `published: true` with the failing phase, not a false assertion
that the release is still private. Consumers of schema 1 must account for the
new nullable field and explicit state.

Local packaging also preserves existing output files. If writing a ZIP's
checksum sidecar fails, the newly created ZIP is removed. If assembly cannot
write its second metadata file, it removes the newly created `SHA256SUMS`.
These are error-path cleanups; two-file publication is not crash-atomic.

## Updating a version or platform

Change `VERSION` deliberately and keep the release tag consistent. An ordinary
`go build` identifies itself as `dev (unknown)`; the release builder injects
the intended version and source revision for `--version`.

The supported package matrix is the six entries in
[release/targets.json](https://github.com/dlubom/survex-duplicates/blob/main/release/targets.json).
When changing toolchain, targets, archive content or publication logic, update
the associated tests and documentation and repeat archive readback plus native
execution for the affected packages. Preserve the full KATASTER example, its
attribution and its independent expected reports. New measurements belong in
separate verification records; historical evidence is not overwritten.
