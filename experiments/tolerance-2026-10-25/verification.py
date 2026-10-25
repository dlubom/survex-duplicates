"""Independent small-sample oracle for compare.py's saved experiment.

Uses dump3d MOVE/LINE/NODE, Fraction parsing and Python integer arithmetic.
Does not import or execute compare.py and does not require NumPy.
Run: python3 experiments/tolerance-2026-10-25/verification.py
"""

import argparse
import hashlib
import itertools
import json
import math
import os
from pathlib import Path
import random
import re
import subprocess
import sys
import time
from fractions import Fraction


def centimetres(text):
    value = Fraction(text) * 100
    if value.denominator != 1:
        raise ValueError(f"Off-grid coordinate: {text}")
    return value.numerator


def metadata(text):
    match = re.fullmatch(r"\[(.*)\](?:\s+(.*))?", text)
    if match is None:
        raise ValueError(f"Bad metadata: {text}")
    return match[1], (match[2] or "").split()


def read_model(text):
    current = None
    legs, names, anonymous = [], {}, set()
    stopped = False
    for number, line in enumerate(text.splitlines(), 1):
        if stopped:
            raise ValueError("Data after STOP")
        kind = line.partition(" ")[0]
        if kind == "MOVE":
            fields = line.split()
            assert len(fields) == 4
            current = tuple(centimetres(x) for x in fields[1:])
        elif kind == "LINE":
            assert current is not None
            fields = line.split(maxsplit=4)
            end = tuple(centimetres(x) for x in fields[1:4])
            survey, suffix = metadata(fields[4])
            legs.append({"a": current, "b": end, "survey": survey,
                         "suffix": suffix, "move_line_source_line": number})
            current = end
        elif kind == "NODE":
            fields = line.split(maxsplit=4)
            position = tuple(centimetres(x) for x in fields[1:4])
            name, flags = metadata(fields[4])
            if name:
                names.setdefault(position, set()).add(name)
            if "ANON" in flags:
                anonymous.add(position)
        elif kind == "STOP":
            stopped = True
        elif kind not in {"TITLE", "DATE", "DATE_NUMERIC", "CS", "VERSION",
                          "SEPARATOR", "--", "ERROR_INFO"}:
            raise ValueError(f"Unexpected record at {number}: {line}")
    assert stopped
    return legs, names, anonymous


def squared(p, q):
    return sum((p[k] - q[k]) ** 2 for k in range(3))


def compare_pair(first, second):
    aa = squared(first["a"], second["a"])
    bb = squared(first["b"], second["b"])
    ab = squared(first["a"], second["b"])
    ba = squared(first["b"], second["a"])
    direct, reverse = max(aa, bb), max(ab, ba)
    reverse_wins = reverse < direct
    error = reverse if reverse_wins else direct
    length_a = squared(first["a"], first["b"])
    length_b = squared(second["a"], second["b"])
    shorter = min(length_a, length_b)
    # Cross multiplication expresses the rules without floor division.
    rules = {"exact": error == 0,
             "abs_1cm": error <= 1, "abs_5cm": error <= 25,
             "abs_10cm": error <= 100, "abs_50cm": error <= 2500,
             "rel_1pct": 10000 * error <= shorter,
             "rel_2pct": 2500 * error <= shorter,
             "rel_5pct": 400 * error <= shorter,
             "rel_10pct": 100 * error <= shorter}
    rules["min_5cm_5pct"] = rules["abs_5cm"] and rules["rel_5pct"]
    rules["max_5cm_5pct"] = rules["abs_5cm"] or rules["rel_5pct"]
    return {"distance_sq_cm": error, "reversed": reverse_wins,
            "endpoint_errors_sq_cm": [ab, ba] if reverse_wins else [aa, bb],
            "length_a_sq_cm": length_a, "length_b_sq_cm": length_b,
            "relative_error_percent": 100 * math.sqrt(Fraction(error, shorter))
            if shorter else None, "rules": rules}


def main():
    start = time.perf_counter()
    directory = Path(__file__).resolve().parent
    root = directory.parents[1]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input",type=Path,default=root/"examples/kataster/KATASTER.3d")
    parser.add_argument("--results",type=Path,default=directory/"results.json")
    parser.add_argument("--out",type=Path,default=root/"reports/tolerance-experiment/verification.json")
    args=parser.parse_args()
    raw_results = args.results.read_bytes()
    expected = json.loads(raw_results)
    source = args.input.resolve()
    source_label = source.relative_to(root).as_posix() if source.is_relative_to(root) else source.name
    source_hash = hashlib.sha256(source.read_bytes()).hexdigest()
    assert source_hash == expected["source_sha256"]
    command = ["dump3d", "--show-dates", str(source)]
    environment = dict(os.environ)
    environment.pop("LC_ALL", None)
    environment["LC_NUMERIC"] = "C"
    output = subprocess.run(command, env=environment, capture_output=True, check=True)
    legs, names, anonymous = read_model(output.stdout.decode("utf-8"))
    assert len(legs) == expected["leg_count"]
    forced = {2836, 2837, 14209, 14228}
    seed = 20261025
    population = [i for i in range(1, len(legs) + 1) if i not in forced]
    sample = sorted(forced | set(random.Random(seed).sample(population, 508)))
    assert len(sample) == 512
    reference = {(r["leg_a"], r["leg_b"]): r for r in expected["pairs"]
                 if r["leg_a"] in sample and r["leg_b"] in sample}
    actual = {}
    counts = {rule: 0 for rule in expected["counts"]}
    for i, j in itertools.combinations(sample, 2):
        result = compare_pair(legs[i - 1], legs[j - 1])
        for rule, matches in result["rules"].items():
            counts[rule] += matches
        if any(result["rules"].values()):
            actual[i, j] = result
    mismatches = []
    for pair in sorted(actual.keys() | reference.keys()):
        if pair not in actual or pair not in reference:
            mismatches.append({"pair": pair, "reason": "Pair set differs"})
            continue
        for key in ("distance_sq_cm", "reversed", "rules"):
            if actual[pair][key] != reference[pair][key]:
                mismatches.append({"pair": pair, "field": key,
                                   "actual": actual[pair][key],
                                   "expected": reference[pair][key]})
        relative = actual[pair]["relative_error_percent"]
        ref_relative = reference[pair]["relative_error_percent"]
        if relative is not None and not math.isclose(relative, ref_relative,
                                                      rel_tol=1e-12, abs_tol=1e-12):
            mismatches.append({"pair": pair, "field": "relative_error_percent"})
    checked_metadata = 0
    for identifier in sample:
        saved = expected["legs"].get(str(identifier))
        if saved is None:
            continue
        leg = legs[identifier - 1]
        fields = {"a": list(leg["a"]), "b": list(leg["b"]),
                  "survey": leg["survey"], "metadata": " ".join(leg["suffix"]),
                  "names_a": sorted(names.get(leg["a"], set())),
                  "names_b": sorted(names.get(leg["b"], set())),
                  "anonymous_a": leg["a"] in anonymous,
                  "anonymous_b": leg["b"] in anonymous}
        checked_metadata += 1
        for key, value in fields.items():
            if saved[key] != value:
                mismatches.append({"leg": identifier, "field": key,
                                   "actual": value, "expected": saved[key]})
    known = []
    for pair in ((2836, 14209), (2837, 14228)):
        record = {"leg_a": pair[0], "leg_b": pair[1], **actual[pair]}
        record["distance_m"] = math.sqrt(record["distance_sq_cm"]) / 100
        record["length_a_m"] = math.sqrt(record["length_a_sq_cm"]) / 100
        record["length_b_m"] = math.sqrt(record["length_b_sq_cm"]) / 100
        known.append(record)
    report = {"status": "passed" if not mismatches else "failed",
              "method": "MOVE/LINE + NODE; Fraction parsing; all selected pairs; Python int; no NumPy or compare.py imports",
              "source": source_label, "source_sha256": source_hash,
              "results_sha256": hashlib.sha256(raw_results).hexdigest(),
              "command": ["dump3d", "--show-dates", source_label], "dump_sha256": hashlib.sha256(output.stdout).hexdigest(),
              "dump3d_version": subprocess.check_output([command[0], "--version"], text=True).strip(),
              "python": sys.version, "seed": seed, "sample_size": len(sample),
              "forced_leg_ids": sorted(forced), "sample_leg_ids": sample,
              "pairs_checked": len(sample) * (len(sample) - 1) // 2,
              "matching_pairs": len(actual), "reference_matching_pairs": len(reference),
              "sample_counts": counts, "metadata_records_checked": checked_metadata,
              "known_reference_pairs": known, "mismatches": mismatches,
              "elapsed_seconds": time.perf_counter() - start,
              "scope_limit": "Independent verification of 512 legs, not a second full-data enumeration"}
    args.out.parent.mkdir(parents=True,exist_ok=True)
    args.out.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ("status", "pairs_checked", "matching_pairs", "sample_counts", "mismatches")}, indent=2))
    return int(bool(mismatches))


if __name__ == "__main__":
    raise SystemExit(main())
