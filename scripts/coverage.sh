#!/usr/bin/env bash
# Measure statement coverage through tests and the real release CLI.
set -euo pipefail

cd "$(dirname "$0")/.."
coverage_tmp=$(mktemp -d "${TMPDIR:-/tmp}/survex-coverage.XXXXXX")
trap 'rm -rf "$coverage_tmp"' EXIT
mkdir -p "$coverage_tmp/counters"

go test -count=1 -covermode=set -coverprofile="$coverage_tmp/unit.out" ./... | tee "$coverage_tmp/unit-summary.txt"
coverage_exe="$coverage_tmp/release"
if [ "$(go env GOOS)" = windows ]; then coverage_exe="$coverage_exe.exe"; fi
go build -cover -o "$coverage_exe" ./cmd/release
coverage_target="$(go env GOOS)_$(go env GOARCH)"
coverage_version="v$(cat VERSION)-coverage"
coverage_archive="$coverage_tmp/packages/survex-duplicates_${coverage_version}_${coverage_target}.zip"
export GOCOVERDIR="$coverage_tmp/counters"
"$coverage_exe" matrix
"$coverage_exe" build --target "$coverage_target" --version "$coverage_version" --out "$coverage_tmp/packages" --snapshot
"$coverage_exe" verify --archive "$coverage_archive"
"$coverage_exe" smoke --archive "$coverage_archive"
go tool covdata textfmt -i="$coverage_tmp/counters" -o="$coverage_tmp/integration.out"

# Both profiles use the same source. Union their covered statement blocks;
# percentages cannot be added. Reject incompatible block sizes or modes.
awk '
  /^mode:/ { if ($2 != "set") exit 1; next }
  NF != 3 { exit 1 }
  { if ($1 in statements && statements[$1] != $2) exit 1
    statements[$1] = $2; covered[$1] = covered[$1] || ($3 > 0) }
  END { for (block in statements) print block, statements[block], covered[block] + 0 }
' "$coverage_tmp/unit.out" "$coverage_tmp/integration.out" | LC_ALL=C sort > "$coverage_tmp/blocks.out"
{ echo 'mode: set'; cat "$coverage_tmp/blocks.out"; } > "$coverage_tmp/combined.out"

# Write ignored artifacts only after source-fingerprint checks have completed.
mkdir -p reports/coverage
cp "$coverage_tmp/unit.out" reports/coverage/unit.out
cp "$coverage_tmp/integration.out" reports/coverage/integration.out
cp "$coverage_tmp/combined.out" reports/coverage/combined.out
cp "$coverage_tmp/unit-summary.txt" reports/coverage/unit-summary.txt
awk 'NR > 1 { package = $1; sub(/\/[^\/]+$/, "", package)
  total[package] += $2; if ($3 > 0) hit[package] += $2 }
  END { for (package in total)
    printf "%s\t%.1f%%\t%d/%d statements\n", package, 100 * hit[package] / total[package], hit[package], total[package] }
' reports/coverage/combined.out | LC_ALL=C sort > reports/coverage/packages.txt
go tool cover -func=reports/coverage/combined.out | tee reports/coverage/functions.txt
go tool cover -html=reports/coverage/combined.out -o reports/coverage/index.html
