"""Command-line coordination and user-visible diagnostics."""

import argparse
import json
import sys
from pathlib import Path
from time import perf_counter

from .geometry import Tolerance
from .reader import read_model
from .report import validate_destination, write_report
from .search import Search


def _positive_int(text: str) -> int:
    value = int(text)
    if value <= 0:
        raise ValueError("expected a positive integer")
    return value


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Find geometric duplicate candidates in Survex models: both endpoints, including reversed legs. "
        "The default is 5% of the shorter leg, with no fixed centimetre cap. "
        "After network adjustment, repeated readings may already have been averaged; "
        "each match requires human review. Legs already marked DUPLICATE are excluded by default.",
        epilog="Examples:\n  python -m survex_duplicates model.3d --out pairs.csv\n"
        "  python -m survex_duplicates model.3d --relative-percent 5 --exclude-splays --out without-splays.csv\n"
        "  python -m survex_duplicates model.dump --epsilon 0.05 --out absolute.csv\n"
        "Text dumps work without Survex. Binary .3d input requires dump3d with --legs and --show-dates.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("input", type=Path)
    parser.add_argument("--out", type=Path, required=True)
    group = parser.add_mutually_exclusive_group()
    group.add_argument(
        "--relative-percent",
        dest="tolerance",
        type=Tolerance.relative,
        help="percentage of the shorter leg length (default: 5)",
    )
    group.add_argument(
        "--epsilon",
        dest="tolerance",
        type=Tolerance.absolute,
        help="absolute tolerance for each endpoint, in metres",
    )
    parser.add_argument(
        "--exclude-splays", action="store_true", help="exclude all legs carrying the SPLAY flag"
    )
    parser.add_argument(
        "--include-duplicates",
        action="store_true",
        help="include legs already acknowledged with the DUPLICATE flag",
    )
    parser.add_argument("--batch-size", type=_positive_int, default=1)
    parser.add_argument("--input-format", choices=("auto", "3d", "dump"), default="auto")
    parser.add_argument("--dump3d", default="dump3d", help="path to the Survex dump3d executable")
    parser.add_argument(
        "--force",
        action="store_true",
        help="replace an existing report only after successful completion",
    )
    try:
        args = parser.parse_args(argv)
    except SystemExit as exc:
        return int(exc.code or 0)
    try:
        return _run(args)
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


def _run(args: argparse.Namespace) -> int:
    if not args.input.is_file():
        raise ValueError(f"input must be an existing file: {args.input}")
    validate_destination(args.input, args.out, args.force)
    start = perf_counter()
    model = read_model(args.input, args.input_format, args.dump3d)
    read_seconds = perf_counter() - start
    tolerance = args.tolerance or Tolerance.relative()
    search = Search(model, tolerance, args.exclude_splays, args.include_duplicates)
    start = perf_counter()
    write_report(args.out, model, search.pairs(args.batch_size), tolerance, args.force)
    print(
        json.dumps(
            {
                "n_input": search.n_input,
                "n_excluded": search.n_excluded,
                "n": search.n,
                "C": search.candidates,
                "K": search.matches,
                "tolerance_mode": tolerance.mode,
                "tolerance_value": str(tolerance.value),
                "tolerance_unit": "percent" if tolerance.mode == "relative_percent" else "m",
                "exclude_splays": args.exclude_splays,
                "include_duplicates": args.include_duplicates,
                "batch_size": args.batch_size,
                "input_format": model.input_format,
                "read_seconds": read_seconds,
                "build_seconds": search.build_seconds,
                "search_write_seconds": perf_counter() - start,
            }
        ),
        file=sys.stderr,
    )
    return 0
