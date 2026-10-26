# Platforms and release packages

The release configuration prepares **six native ZIP packages**, built with
**Go 1.26.3**: Linux, macOS and Windows, each for x86-64 and ARM64. The version
in `VERSION` is currently `1.1.0`; preparation of these packages does not mean
that `v1.1.0` has been published. Download published assets from
[GitHub Releases](https://github.com/dlubom/survex-duplicates/releases).

## Choose a package

Choose the operating system on which the executable will run and its processor
architecture. `amd64` means x86-64, including both Intel and AMD processors.
`arm64` means 64-bit ARM; on macOS this is Apple silicon.

| System | Processor | Target ID | ZIP filename for v1.1.0 |
| --- | --- | --- | --- |
| Linux | x86-64 | `linux_amd64` | `survex-duplicates_v1.1.0_linux_amd64.zip` |
| Linux | ARM64 | `linux_arm64` | `survex-duplicates_v1.1.0_linux_arm64.zip` |
| macOS | Intel | `darwin_amd64` | `survex-duplicates_v1.1.0_darwin_amd64.zip` |
| macOS | Apple silicon | `darwin_arm64` | `survex-duplicates_v1.1.0_darwin_arm64.zip` |
| Windows | x86-64 | `windows_amd64` | `survex-duplicates_v1.1.0_windows_amd64.zip` |
| Windows | ARM64 | `windows_arm64` | `survex-duplicates_v1.1.0_windows_arm64.zip` |

For another release, substitute its version in the filename. The exact matrix
is maintained in [release/targets.json in the source repository](https://github.com/dlubom/survex-duplicates/blob/main/release/targets.json).
The build uses the baseline `GOAMD64=v1` or `GOARM64=v8.0` setting, as defined
by the [Go 1.26.3 build documentation](https://github.com/golang/go/blob/go1.26.3/src/cmd/go/alldocs.go).

Go 1.26 supports macOS 12 Monterey or later. The upstream Windows baseline is
Windows 10 or Windows Server 2016 or later. Other operating-system and kernel
requirements follow the Go runtime; these upstream minima are not this
project's tested-version matrix. See the [Go 1.26 port notes](https://go.dev/doc/go1.26#ports)
and [Go minimum requirements](https://go.dev/wiki/MinimumRequirements).

## Extract and run

Download the ZIP and its matching `.sha256` file. Compare the archive hash with
that checksum before extracting. For example, after downloading the Apple
silicon package on macOS:

```bash
shasum -a 256 -c survex-duplicates_v1.1.0_darwin_arm64.zip.sha256
unzip survex-duplicates_v1.1.0_darwin_arm64.zip -d survex-duplicates
cd survex-duplicates
./survex-duplicates --version
./survex-duplicates examples/kataster/KATASTER.legs.dump --out duplicates.csv
```

On Linux, use the matching Linux filename and `sha256sum -c` for verification.
If an extractor does not retain the executable permission, run
`chmod +x survex-duplicates` in the extracted directory.

For Windows x86-64, use PowerShell and compare the `Get-FileHash` value with the
matching checksum file:

```powershell
Get-FileHash .\survex-duplicates_v1.1.0_windows_amd64.zip -Algorithm SHA256
Get-Content .\survex-duplicates_v1.1.0_windows_amd64.zip.sha256
Expand-Archive .\survex-duplicates_v1.1.0_windows_amd64.zip -DestinationPath .\survex-duplicates
Set-Location .\survex-duplicates
.\survex-duplicates.exe --version
.\survex-duplicates.exe examples/kataster/KATASTER.legs.dump --out duplicates.csv
```

The demo should produce **218 pairs**. No Go, Python or Survex installation is
needed to run it. Binary `.3d` input additionally requires a working Survex
`dump3d` executable on your system; it is not bundled. See the
[usage guide](usage.md#read-binary-models-or-text-dumps).

Packages do not include a Windows publisher certificate or Apple Developer ID
notarization. The operating system may request confirmation when launching a
downloaded executable; verify its source and checksum before allowing it.

## Archive contents

ZIPs have no enclosing directory; extract each into its own destination.
Each contains:

- `survex-duplicates`, or `survex-duplicates.exe` on Windows;
- a package-specific `README.md` and `manifest.json`;
- `LICENSE` for the MIT software license, `GO-LICENSE` for the Go runtime,
  and `NOTICE.md` for survey-data attribution;
- `GLOSSARY.md`, usage/platform/release guides, specification and ADRs;
- the complete `examples/kataster/` directory: binary model, complete text
  dump, provenance, data license, eight expected CSVs and their verification record.

`manifest.json` records the version, source revision, Go toolchain, target,
source fingerprints and bundled-file hashes. `--version` reports the executable's
version and source revision. Ordinary development builds report `dev (unknown)`;
release builds embed their version and commit. Snapshot packages are labelled
in the manifest, and a dirty source revision ends in `-dirty`.

The accompanying `SHA256SUMS` and `release-manifest.json` list all six archives
and their hashes. Each archive also has an individual checksum sidecar.
Checksum agreement verifies the downloaded bytes against
that release's records; it is separate from testing the executable's behavior.

## Execution coverage

The workflow is configured for native tests and extracted-archive smoke tests
on all six targets:

| Target | GitHub runner | Race detector |
| --- | --- | --- |
| `linux_amd64` | `ubuntu-24.04` | enabled |
| `linux_arm64` | `ubuntu-24.04-arm` | enabled |
| `darwin_amd64` | `macos-15-intel` | enabled |
| `darwin_arm64` | `macos-15` | enabled |
| `windows_amd64` | `windows-2025` | enabled |
| `windows_arm64` | `windows-11-arm` | unavailable in Go 1.26.3; ordinary tests run |

The Windows ARM64 race-detector limitation comes from the
[pinned Go platform implementation](https://github.com/golang/go/blob/go1.26.3/src/internal/platform/supported.go).
The Go tests verify all eight KATASTER settings; each extracted-archive smoke
test checks `--version` and complete CSV bytes for the 218-pair default and
60-pair no-SPLAY reports.
Configuring these jobs is not evidence that they have passed. At preparation
time, native local execution has been verified on macOS ARM64; the other
targets require completed remote jobs for the release's exact revision.
A successful cross-build alone does not verify runtime behavior.

## Output filesystem behavior

The detector writes, flushes and syncs a temporary CSV beside its destination
before publishing it. Processing failures leave an existing report unchanged.
Without `--force`, publication uses a hardlink to create the destination only
if it is absent, so the output filesystem must support hardlinks. With
`--force`, publication uses the operating system's rename operation.

Go does not promise that rename is atomic on every operating system, including
non-Unix systems. File syncing also does not guarantee directory-entry
durability after power loss. Network filesystems and crash behavior have not
been independently qualified. See the [Go 1.26.3 os.Rename contract](https://github.com/golang/go/blob/go1.26.3/src/os/file.go).
