"""Read-only kataster verification through the CLI against preserved exhaustive evidence."""

import argparse
import csv
import hashlib
import json
import subprocess
import sys
from math import sqrt
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HISTORICAL_SHA256 = "55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077"


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify(source, destination):
    source = source.resolve()
    digest = sha256(source)
    if digest != HISTORICAL_SHA256:
        raise ValueError(
            f"Changed input SHA-256 {digest}; historical expectations cannot be used. "
            "Preserve historical evidence and establish new exhaustive expectations before comparison."
        )
    reference_path = ROOT / "experiments/tolerance-2026-10-25/results.json"
    reference = json.loads(reference_path.read_text())
    assert reference["source_sha256"] == digest
    destination.mkdir(parents=True, exist_ok=True)
    runs = []
    for mode, option, value, rule in (
        ("relative_percent", "--relative-percent", "5", "rel_5pct"),
        ("absolute_m", "--epsilon", "0.05", "abs_5cm"),
    ):
        for include in (False, True):
            for exclude in (False, True):
                output = (
                    destination
                    / f"kataster-{mode}-{'no-splays' if exclude else 'all'}{'-include-duplicates' if include else ''}.csv"
                )
                command = [
                    sys.executable,
                    "-m",
                    "survex_duplicates",
                    str(source),
                    "--out",
                    str(output),
                    "--force",
                    option,
                    value,
                ]
                if exclude:
                    command.append("--exclude-splays")
                if include:
                    command.append("--include-duplicates")
                process = subprocess.run(
                    command, capture_output=True, text=True, check=True, cwd=ROOT
                )
                summary = json.loads(process.stderr)
                expected = [
                    (p["leg_a"], p["leg_b"], p["reversed"])
                    for p in reference["pairs"]
                    if p["rules"][rule]
                    and (not exclude or not p["any_splay"])
                    and (
                        include
                        or all(
                            "DUPLICATE" not in reference["legs"][str(p[key])]["flags"]
                            for key in ("leg_a", "leg_b")
                        )
                    )
                ]
                actual = []
                with output.open(encoding="utf-8", newline="") as stream:
                    for row in csv.DictReader(stream):
                        i, j = int(row["leg_a"]), int(row["leg_b"])
                        reversed_ = bool(int(row["reversed"]))
                        actual.append((i, j, reversed_))
                        # Independently recover both endpoint errors from historic cm coordinates.
                        a, b = reference["legs"][str(i)], reference["legs"][str(j)]
                        first, second = (b["b"], b["a"]) if reversed_ else (b["a"], b["b"])
                        e1 = sum((int(x) - int(y)) ** 2 for x, y in zip(a["a"], first))
                        e2 = sum((int(x) - int(y)) ** 2 for x, y in zip(a["b"], second))
                        assert float(row["endpoint_1_error_m"]) == sqrt(e1) / 100
                        assert float(row["endpoint_2_error_m"]) == sqrt(e2) / 100
                        assert float(row["distance_cm"]) == sqrt(max(e1, e2))
                assert actual == expected, (
                    f"full pair/orientation mismatch: {mode}, exclude={exclude}"
                )
                runs.append(
                    {
                        **summary,
                        "csv": output.name,
                        "csv_sha256": sha256(output),
                        "full_pairs_and_orientations": "passed",
                        "both_endpoint_errors": "passed",
                    }
                )
    final_digest = sha256(source)
    assert final_digest == digest
    record = {
        "input": source.relative_to(ROOT).as_posix()
        if source.is_relative_to(ROOT)
        else source.name,
        "sha256_before": digest,
        "sha256_after": final_digest,
        "reference": str(reference_path.relative_to(ROOT)),
        "reference_sha256": sha256(reference_path),
        "runs": runs,
    }
    (destination / "verification.json").write_text(
        json.dumps(record, ensure_ascii=False, indent=2) + "\n"
    )
    return record


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "input", type=Path, nargs="?", default=ROOT / "examples/kataster/KATASTER.3d"
    )
    parser.add_argument("--out-dir", type=Path, default=ROOT / "examples/kataster/results")
    args = parser.parse_args()
    result = verify(args.input, args.out_dir)
    for run in result["runs"]:
        print(
            f"{run['tolerance_mode']} exclude_splays={run['exclude_splays']} include_duplicates={run['include_duplicates']}: "
            f"n={run['n']} C={run['C']} K={run['K']} — full comparison passed"
        )
