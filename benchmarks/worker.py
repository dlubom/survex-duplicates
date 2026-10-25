"""One isolated process per benchmark, including reading and CSV publication."""

import argparse
import csv
import json
import resource
import sys
import tempfile
from itertools import zip_longest
from pathlib import Path
from time import perf_counter

from survex_duplicates.geometry import Tolerance
from survex_duplicates.reader import read_model
from survex_duplicates.report import write_report
from survex_duplicates.search import Search

from .datasets import expected_pairs, make_dump


def run(args):
    total_start = perf_counter()
    with tempfile.TemporaryDirectory(prefix="survex-benchmark-") as folder:
        source, output = Path(folder) / "model.dump", Path(folder) / "report.csv"
        start = perf_counter()
        make_dump(source, args.n, args.kind, args.seed)
        generation_seconds = perf_counter() - start
        start = perf_counter()
        model = read_model(source)
        read_seconds = perf_counter() - start
        tolerance = (
            Tolerance.relative() if args.mode == "relative_percent" else Tolerance.absolute(".05")
        )
        search = Search(model, tolerance, args.exclude_splays, args.include_duplicates)
        start = perf_counter()
        write_report(output, model, search.pairs(args.batch_size), tolerance)
        search_write_seconds = perf_counter() - start
        expected = expected_pairs(
            args.n, args.kind, args.mode, args.exclude_splays, args.include_duplicates
        )
        count = 0
        with output.open(encoding="utf-8", newline="") as stream:
            for row, wanted in zip_longest(csv.DictReader(stream), expected):
                if row is None or wanted is None:
                    raise AssertionError("benchmark result has missing/extra pairs")
                actual = int(row["leg_a"]), int(row["leg_b"]), bool(int(row["reversed"]))
                if actual != wanted:
                    raise AssertionError((actual, wanted))
                count += 1
        assert count == search.matches
        output_bytes = output.stat().st_size
    peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    return {
        "kind": args.kind,
        "seed": args.seed,
        "n_input": search.n_input,
        "n_excluded": search.n_excluded,
        "n": search.n,
        "C": search.candidates,
        "K": search.matches,
        "mode": tolerance.mode,
        "value": str(tolerance.value),
        "exclude_splays": args.exclude_splays,
        "include_duplicates": args.include_duplicates,
        "input_format": "dump",
        "batch_size": args.batch_size,
        "generation_seconds": generation_seconds,
        "read_seconds": read_seconds,
        "build_seconds": search.build_seconds,
        "search_write_seconds": search_write_seconds,
        "total_seconds": perf_counter() - total_start,
        "peak_rss_bytes": peak if sys.platform == "darwin" else peak * 1024,
        "output_bytes": output_bytes,
        "full_pair_orientation_check": "passed",
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--n", type=int, required=True)
    parser.add_argument("--kind", choices=("sparse", "repeated", "dense"), required=True)
    parser.add_argument("--mode", choices=("relative_percent", "absolute_m"), required=True)
    parser.add_argument("--seed", type=int, default=20261025)
    parser.add_argument("--batch-size", type=int, default=1)
    parser.add_argument("--exclude-splays", action="store_true")
    parser.add_argument("--include-duplicates", action="store_true")
    print(json.dumps(run(parser.parse_args())))


if __name__ == "__main__":
    main()
