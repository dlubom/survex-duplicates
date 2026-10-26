"""Differential verification and alternating fresh-process Go/Python measurements.

Run from the repository root. The Python implementation is a frozen reference;
the independent oracle never calls either production predicate or spatial index.
"""

import argparse
import csv
import hashlib
import importlib.metadata
import itertools
import json
import os
import platform
import random
import shutil
import statistics
import subprocess
import sys
import time
from decimal import Decimal
from pathlib import Path

from benchmarks.datasets import expected_pairs, make_dump
from survex_duplicates.geometry import Tolerance
from survex_duplicates.reader import read_model
from survex_duplicates.report import write_report
from survex_duplicates.search import Search
from tests.oracle import all_pairs

ROOT = Path(__file__).resolve().parents[1]
HEADER = 'TITLE "Differential"\nDATE "@0"\nDATE_NUMERIC 0\nVERSION 8\nSEPARATOR \'.\'\n--\n'
MODES = [("relative_percent", "5"), ("absolute_m", "0.05")]


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify_hashes():
    result = {}
    for directory in (ROOT / "examples/kataster", ROOT / "tests/fixtures"):
        provenance = json.loads((directory / "provenance.json").read_text())
        for name, expected in provenance["sha256"].items():
            path = directory / name
            actual = digest(path)
            assert actual == expected, f"Changed snapshot: {path}"
            result[str(path.relative_to(ROOT))] = actual
    return result


def metres(point):
    return " ".join(f"{'-' if v < 0 else ''}{abs(v) // 100}.{abs(v) % 100:02}" for v in point)


def generated(path, seed):
    rng = random.Random(seed)
    legs = []
    for i in range(45):
        a = tuple(rng.randrange(-200, 201) for _ in range(3))
        b = tuple(a[k] + rng.randrange(-1500, 1501) for k in range(3))
        legs.append((a, b))
        if i % 3 == 0:
            legs.append((b, a))
        if i % 5 == 0:
            legs.append(
                (
                    tuple(v + d for v, d in zip(a, (3, 4, 0))),
                    tuple(v + d for v, d in zip(b, (3, 4, 0))),
                )
            )
    lo, hi = -(2**31), 2**31 - 1
    legs += [((0, 0, 0), (0, 0, 0))] * 2 + [
        ((1, 0, 0), (1, 0, 0)),
        ((0, 0, 0), (100, 100, 0)),
        ((5, 5, 0), (105, 105, 0)),
        ((-2, 0, 0), (2, 100, 0)),
        ((2, 0, 0), (-2, 100, 0)),
        ((lo, lo, lo), (hi, hi, hi)),
        ((hi, hi, hi), (lo, lo, lo)),
        ((lo, hi, lo), (hi, lo, hi)),
    ]
    with path.open("w", encoding="utf-8") as stream:
        stream.write(HEADER)
        for i, (a, b) in enumerate(legs):
            flags = ("", " SPLAY", " DUPLICATE", " DUPLICATE SPLAY SURFACE")[i % 4]
            stream.write(
                f'LEG {metres(a)} {metres(b)} [żółć],"q[{i}] STYLE=NORMAL{flags} 2024.02.29\n'
            )
        for i, (a, _) in enumerate(legs):
            stream.write(f'NODE {metres(a)} [  alias],"ż{i}] UNDERGROUND\n')
            stream.write(f"NODE {metres(a)} [] ANON\n")
        stream.write("STOP\n")


def flags(mode, value, exclude, include, batch=1):
    result = [
        "--relative-percent" if mode == "relative_percent" else "--epsilon",
        value,
        "--batch-size",
        str(batch),
    ]
    if exclude:
        result.append("--exclude-splays")
    if include:
        result.append("--include-duplicates")
    return result


def compare(go, probe, out):
    inputs = [(ROOT / "examples/kataster/KATASTER.legs.dump", False)]
    inputs += [
        (ROOT / "tests/fixtures" / name, True)
        for name in (
            "survey.legs.dump",
            "survey.lines.dump",
            "legacy.legs.dump",
            "closure.legs.dump",
        )
    ]
    if shutil.which("dump3d"):
        inputs += [
            (ROOT / "examples/kataster/KATASTER.3d", False),
            (ROOT / "tests/fixtures/survey.3d", True),
        ]
    for seed in (7, 20261025, 91):
        path = out / f"seed-{seed}.dump"
        generated(path, seed)
        inputs.append((path, True))
    # Preserve a reproducible registry sample as an independent-oracle input.
    registry = read_model(inputs[0][0])
    selected = {2835, 14208, 2836, 14227, 1447, 7947, 1448, 7948}
    rng = random.Random(20261025)
    while len(selected) < 512:
        selected.add(rng.randrange(len(registry.legs)))
    sample = out / "registry-512.dump"
    with sample.open("w") as stream:
        stream.write(HEADER)
        for i in sorted(selected):
            stream.write(
                f"LEG {metres(registry.A_cm[i])} {metres(registry.B_cm[i])} [] {' '.join(registry.legs[i].flags)}\n"
            )
        stream.write("STOP\n")
    inputs.append((sample, True))
    requests, expected, checks = [], [], []
    for path, oracle in inputs:
        model = read_model(path)
        modes = list(MODES)
        if path.name.startswith("seed-"):
            modes += [("relative_percent", v) for v in ("4.9999", "17.3", "1e1000")]
            modes += [("absolute_m", v) for v in (".0099", ".01", "1e1000")]
        first = True
        for (mode, value), exclude, include in itertools.product(
            modes, (False, True), (False, True)
        ):
            tolerance = Tolerance(mode, Decimal(value))
            search = Search(model, tolerance, exclude, include)
            tuples = [
                (
                    p.leg_a,
                    p.leg_b,
                    p.match.reversed,
                    p.match.endpoint_1_sq_cm,
                    p.match.endpoint_2_sq_cm,
                )
                for p in search.pairs()
            ]
            if oracle:
                assert tuples == all_pairs(model, mode, value, exclude, include)
            for batch in (1, 19):
                requests.append(
                    dict(
                        Input=str(path),
                        Mode=mode,
                        Value=value,
                        Exclude=exclude,
                        Include=include,
                        Batch=batch,
                        Inspect=first,
                    )
                )
                expected.append((model, tuples, search.candidates, search.n, search.n_excluded))
                first = False
            reference = out / "reference.csv"
            actual = out / "go.csv"
            write_report(reference, model, search.pairs(), tolerance, force=True)
            completed = subprocess.run(
                [
                    str(go),
                    str(path),
                    "--out",
                    str(actual),
                    "--force",
                    *flags(mode, value, exclude, include),
                ],
                capture_output=True,
                text=True,
            )
            assert completed.returncode == 0 and not completed.stdout, completed.stderr
            ref_bytes, go_bytes = reference.read_bytes(), actual.read_bytes()
            if ref_bytes != go_bytes:
                left, right = (
                    list(csv.DictReader(ref_bytes.decode().splitlines())),
                    list(csv.DictReader(go_bytes.decode().splitlines())),
                )
                difference = next(
                    (
                        (i, {k: (a[k], b[k]) for k in a if a[k] != b[k]})
                        for i, (a, b) in enumerate(zip(left, right))
                        if a != b
                    ),
                    ("length/encoding", (len(left), len(right))),
                )
                raise AssertionError(
                    f"CSV differs: {path.name} {mode} {value} {exclude} {include}: {difference}"
                )
            checks.append(
                dict(
                    input=os.path.relpath(path, ROOT),
                    sha256=digest(path),
                    mode=mode,
                    value=value,
                    exclude_splays=exclude,
                    include_duplicates=include,
                    pairs=len(tuples),
                    csv_sha256=hashlib.sha256(ref_bytes).hexdigest(),
                    independent_oracle=oracle,
                )
            )
    request_path, result_path = out / "probe-request.json", out / "probe-results.jsonl"
    request_path.write_text(json.dumps(requests))
    env = dict(
        os.environ, SURVEX_PROBE_REQUEST=str(request_path), SURVEX_PROBE_OUTPUT=str(result_path)
    )
    subprocess.run(
        [str(probe), "-test.run=^TestDifferentialProbe$"], env=env, check=True, capture_output=True
    )
    with result_path.open() as stream:
        for req, (model, pairs, candidates, n, excluded), line in zip(
            requests, expected, stream, strict=True
        ):
            actual = json.loads(line)
            assert actual["pairs"] == [list(p) for p in pairs], req
            assert (actual["C"], actual["n"], actual["n_excluded"]) == (candidates, n, excluded), (
                req
            )
            assert actual["input_format"] == model.input_format
            if req["Inspect"]:
                expected_legs = [
                    dict(
                        ID=leg.leg_id,
                        SourceLine=leg.source_line,
                        A=[int(v) for v in model.A_cm[i]],
                        B=[int(v) for v in model.B_cm[i]],
                        Survey=leg.survey,
                        Flags=list(leg.flags),
                        Style=leg.style,
                        Date=leg.survey_date,
                    )
                    for i, leg in enumerate(model.legs)
                ]
                for leg in actual["legs"] or []:
                    leg["Flags"] = leg["Flags"] or []
                assert (actual["legs"] or []) == expected_legs, req
                expected_stations = [
                    dict(
                        Point=list(p),
                        Station=dict(
                            Names=list(s.names), Anonymous=s.has_anonymous_node, Status=s.status
                        ),
                    )
                    for p, s in sorted(model.stations.items())
                ]
                for station in actual["stations"]:
                    station["Station"]["Names"] = station["Station"]["Names"] or []
                assert actual["stations"] == expected_stations, req
                assert actual["header"] == model.header, req
    request_path.unlink()
    result_path.unlink()
    return dict(
        csv_checks=checks,
        exact_tuple_checks=len(requests),
        binary_available=bool(shutil.which("dump3d")),
    )


def measured(command, out):
    with (out / "stderr.tmp").open("w+b") as stderr, (out / "stdout.tmp").open("w+b") as stdout:
        start = time.perf_counter()
        process = subprocess.Popen(command, stdout=stdout, stderr=stderr)
        _, status, usage = os.wait4(process.pid, 0)
        elapsed = time.perf_counter() - start
        process.returncode = os.waitstatus_to_exitcode(status)
        stderr.seek(0)
        diagnostic = stderr.read().decode()
        stdout.seek(0)
        assert process.returncode == 0 and not stdout.read(), diagnostic
    summary = json.loads(diagnostic)
    summary.update(
        wall_seconds=elapsed,
        peak_rss_bytes=usage.ru_maxrss * (1 if sys.platform == "darwin" else 1024),
    )
    return summary


def benchmark(go, out, repeats):
    cases = []
    for mode, value in MODES:
        for exclude, include in itertools.product((False, True), repeat=2):
            cases.append(
                (
                    "registry",
                    ROOT / "examples/kataster/KATASTER.legs.dump",
                    mode,
                    value,
                    exclude,
                    include,
                )
            )
    for kind, n in (
        ("sparse", 50000),
        ("sparse", 100000),
        ("repeated", 50000),
        ("repeated", 100000),
        ("dense", 400),
    ):
        path = out / f"{kind}-{n}.dump"
        for mode, value in MODES:
            for include in (False, True) if kind == "repeated" else (False,):
                cases.append((f"{kind}-{n}", path, mode, value, False, include))
    if shutil.which("dump3d"):
        cases.append(
            (
                "registry-binary",
                ROOT / "examples/kataster/KATASTER.3d",
                "relative_percent",
                "5",
                False,
                False,
            )
        )
    results = []
    generated_input = None
    for name, path, mode, value, exclude, include in cases:
        synthetic = name.startswith(("sparse-", "repeated-", "dense-"))
        if synthetic and path != generated_input:
            if generated_input is not None:
                generated_input.unlink()
            kind, count = name.rsplit("-", 1)
            make_dump(path, int(count), kind, 20261025)
            generated_input = path
        runs = {"python": [], "go": []}
        hashes = {}
        for iteration in range(repeats + 1):
            order = ("python", "go") if iteration % 2 == 0 else ("go", "python")
            for language in order:
                output = out / "timed.csv"
                output.unlink(missing_ok=True)
                command = (
                    [str(go)] if language == "go" else [sys.executable, "-m", "survex_duplicates"]
                )
                record = measured(
                    [
                        *command,
                        str(path),
                        "--out",
                        str(output),
                        "--force",
                        *flags(mode, value, exclude, include),
                    ],
                    out,
                )
                hashes[language] = digest(output)
                if synthetic and iteration == repeats and language == "go":
                    kind, count = name.rsplit("-", 1)
                    with output.open(newline="") as stream:
                        rows = [
                            (int(r["leg_a"]), int(r["leg_b"]), bool(int(r["reversed"])))
                            for r in csv.DictReader(stream)
                        ]
                    assert rows == list(expected_pairs(int(count), kind, mode, exclude, include))
                output.unlink()
                if iteration:
                    runs[language].append(record)
            assert hashes["python"] == hashes["go"], (name, mode, exclude, include)
        medians = {
            language: {
                key: statistics.median(r[key] for r in records)
                for key in (
                    "wall_seconds",
                    "read_seconds",
                    "build_seconds",
                    "search_write_seconds",
                    "peak_rss_bytes",
                )
            }
            for language, records in runs.items()
        }
        result = dict(
            name=name,
            input=os.path.relpath(path, ROOT),
            input_sha256=digest(path),
            mode=mode,
            value=value,
            exclude_splays=exclude,
            include_duplicates=include,
            output_sha256=hashes["go"],
            runs=runs,
            median=medians,
            wall_speedup=medians["python"]["wall_seconds"] / medians["go"]["wall_seconds"],
        )
        results.append(result)
        (out / "benchmark-progress.json").write_text(json.dumps(results, indent=2) + "\n")
        print(
            f"{name} {mode} exclude={exclude} include={include}: {result['wall_speedup']:.2f}x",
            flush=True,
        )
    if generated_input is not None:
        generated_input.unlink()
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go-bin", type=Path, required=True)
    parser.add_argument("--probe-bin", type=Path, required=True)
    parser.add_argument("--out-dir", type=Path, default=Path("reports/go-comparison"))
    parser.add_argument("--repeats", type=int, default=5)
    parser.add_argument("--skip-benchmark", action="store_true")
    parser.add_argument("--benchmark-only", action="store_true")
    args = parser.parse_args()
    if args.repeats < 1:
        parser.error("--repeats must be positive")
    if not args.skip_benchmark and not hasattr(os, "wait4"):
        parser.error("timing with peak RSS requires Unix; use --skip-benchmark for parity checks")
    out = args.out_dir.resolve()
    out.mkdir(parents=True, exist_ok=True)
    before = verify_hashes()
    result = dict(
        environment=dict(
            python=sys.version,
            platform=platform.platform(),
            machine=platform.machine(),
            go=subprocess.check_output(["go", "version"], text=True).strip(),
            cpu_count=os.cpu_count(),
            numpy=importlib.metadata.version("numpy"),
            scipy=importlib.metadata.version("scipy"),
        ),
        input_hashes=before,
        repeats=args.repeats,
        warmups=1,
        order="alternating fresh processes; generation/readback excluded",
        go_binary_sha256=digest(args.go_bin.resolve()),
        reference_commit="1143a96292d49457ea344d46410072ecbbcec1af",
        go_source_sha256={p.name: digest(p) for p in sorted(ROOT.glob("*.go"))},
        go_mod_sha256=digest(ROOT / "go.mod"),
        harness_sha256=digest(Path(__file__)),
    )
    if not args.benchmark_only:
        result["verification"] = compare(args.go_bin.resolve(), args.probe_bin.resolve(), out)
        (out / "verification.json").write_text(json.dumps(result, indent=2) + "\n")
        print("Differential verification passed", flush=True)
    if not args.skip_benchmark:
        result["benchmarks"] = benchmark(args.go_bin.resolve(), out, args.repeats)
    assert verify_hashes() == before
    result["input_hashes_unchanged"] = True
    (out / "results.json").write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
