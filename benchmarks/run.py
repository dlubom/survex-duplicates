"""Run all required cases sequentially with fresh-process peak RSS measurements."""

import argparse
import json
import os
import platform
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

import numpy
import scipy


def environment():
    info = {
        "python": sys.version,
        "numpy": numpy.__version__,
        "scipy": scipy.__version__,
        "platform": platform.platform(),
        "machine": platform.machine(),
        "cpu_count": os.cpu_count(),
    }
    if sys.platform == "darwin":
        for key in ("hw.model", "hw.memsize", "machdep.cpu.brand_string"):
            value = subprocess.run(["sysctl", "-n", key], text=True, capture_output=True)
            info[key] = (
                value.stdout.strip()
                if value.returncode == 0
                else "unavailable: " + value.stderr.strip()
            )
    return info


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path("benchmarks/publication-results.json"))
    parser.add_argument("--batch-size", type=int, default=1)
    parser.add_argument("--include-duplicates", action="store_true")
    args = parser.parse_args()
    cases = [
        (n, kind, mode, False)
        for n in (50000, 100000)
        for kind in ("sparse", "repeated")
        for mode in ("relative_percent", "absolute_m")
    ]
    cases += [(400, "dense", mode, False) for mode in ("relative_percent", "absolute_m")]
    cases += [(100000, "repeated", mode, True) for mode in ("relative_percent", "absolute_m")]
    results = []
    record = {
        "recorded_at": datetime.now(timezone.utc).isoformat(),
        "environment": environment(),
        "generator": "benchmarks.datasets.make_dump; seed 20261025",
        "memory_measurement": "ru_maxrss in each fresh worker; whole process high-water mark",
        "complete": False,
        "runs": results,
    }
    for n, kind, mode, exclude in cases:
        command = [
            sys.executable,
            "-m",
            "benchmarks.worker",
            "--n",
            str(n),
            "--kind",
            kind,
            "--mode",
            mode,
            "--batch-size",
            str(args.batch_size),
        ]
        if exclude:
            command.append("--exclude-splays")
        if args.include_duplicates:
            command.append("--include-duplicates")
        result = json.loads(subprocess.check_output(command, text=True))
        results.append(result)
        record["complete"] = len(results) == len(cases)
        args.out.write_text(
            json.dumps(record, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        print(
            f"{kind} n={n} {mode} exclude={exclude}: C={result['C']} K={result['K']} "
            f"read={result['read_seconds']:.3f}s build={result['build_seconds']:.3f}s "
            f"search+CSV={result['search_write_seconds']:.3f}s RSS={result['peak_rss_bytes'] / 2**20:.1f} MiB",
            flush=True,
        )


if __name__ == "__main__":
    main()
