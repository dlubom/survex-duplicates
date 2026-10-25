"""Historical tolerance experiment, not the production parser or detector.

Enumerates every pair of the complete registry without a spatial index.
Results are geometric candidates, not labelled true/false repeated readings.
Requires NumPy and dump3d in PATH. Writes a separate reproducibility output.
"""
import argparse
import csv
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import subprocess
import sys
import time
from decimal import Decimal

import numpy as np

ROOT = Path(__file__).resolve().parents[2]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("input",type=Path,nargs="?",default=ROOT/"examples/kataster/KATASTER.3d")
parser.add_argument("--out-dir",type=Path,default=ROOT/"reports/tolerance-experiment")
args = parser.parse_args()
OUT = args.out_dir.resolve()
OUT.mkdir(parents=True,exist_ok=True)
SOURCE = args.input.resolve()
source_label = SOURCE.relative_to(ROOT).as_posix() if SOURCE.is_relative_to(ROOT) else SOURCE.name
START = time.perf_counter()
source_hash = hashlib.sha256(SOURCE.read_bytes()).hexdigest()
env = dict(os.environ)
env.pop("LC_ALL", None)
env["LC_NUMERIC"] = "C"
command = ["dump3d", "--legs", "--show-dates", str(SOURCE)]
process = subprocess.run(command, env=env, capture_output=True, check=True)
dump = process.stdout.decode("utf-8")
assert dump.rstrip().endswith("STOP")
assert "EXTENDED ELEVATION" not in dump
(OUT / "kataster.legs.dump").write_bytes(process.stdout)
legs, names, anonymous = [], {}, set()

def cm(value):
    value = Decimal(value) * 100
    assert value.is_finite() and value == value.to_integral_value()
    return int(value)

for line_no, line in enumerate(dump.splitlines(), 1):
    if line.startswith("LEG "):
        fields = line.split(maxsplit=7)
        coords = [cm(x) for x in fields[1:7]]
        label, suffix = fields[7][1:].rsplit("]", 1)
        legs.append(dict(id=len(legs)+1, line=line_no, a=coords[:3],
                         b=coords[3:], survey=label, metadata=suffix.strip(),
                         flags=[x for x in suffix.split() if x in {"SPLAY", "SURFACE", "DUPLICATE"}]))
    elif line.startswith("NODE "):
        fields = line.split(maxsplit=4)
        coord = tuple(cm(x) for x in fields[1:4])
        label, suffix = fields[4][1:].rsplit("]", 1)
        if label:
            names.setdefault(coord, set()).add(label)
        else:
            anonymous.add(coord)

A = np.array([x["a"] for x in legs], dtype=np.int64)
B = np.array([x["b"] for x in legs], dtype=np.int64)
n = len(A)
spans = np.maximum(A.max(0), B.max(0)) - np.minimum(A.min(0), B.min(0))
max_squared_distance = sum(int(x)**2 for x in spans)
assert max_squared_distance < 2**63, "NumPy int64 unsafe for this input"
length_sq = np.sum((B-A)**2, axis=1)

def squared_distance(x, y):
    result = np.zeros((len(x), len(y)), dtype=np.int64)
    for axis in range(3):
        diff = x[:, None, axis] - y[None, :, axis]
        result += diff * diff
    return result

rows = []
for start in range(0, n, 64):
    stop = min(start+64, n)
    left = np.arange(start, stop)
    right = np.arange(start+1, n)
    if not len(right):
        break
    aa = squared_distance(A[left], A[right])
    bb = squared_distance(B[left], B[right])
    direct = np.maximum(aa, bb)
    del aa, bb
    ab = squared_distance(A[left], B[right])
    ba = squared_distance(B[left], A[right])
    reverse = np.maximum(ab, ba)
    del ab, ba
    distance_sq = np.minimum(direct, reverse)
    shorter_sq = np.minimum(length_sq[left, None], length_sq[right][None, :])
    # Envelope includes every tested rule: abs <= 0.5 m or relative <= 10%.
    envelope = (distance_sq <= np.maximum(2500, shorter_sq // 100))
    envelope &= left[:, None] < right[None, :]
    ii, jj = np.nonzero(envelope)
    for row, col in zip(ii.tolist(), jj.tolist()):
        i, j = int(left[row]), int(right[col])
        l1, l2 = int(length_sq[i]), int(length_sq[j])
        d2, lmin2 = int(distance_sq[row,col]), min(l1,l2)
        reversed_match = bool(reverse[row,col] < direct[row,col])
        ai, bi, aj, bj = legs[i]["a"], legs[i]["b"], legs[j]["a"], legs[j]["b"]
        # Independent arbitrary-size Python integer check for every retained pair.
        sq = lambda a,b: sum((int(x)-int(y))**2 for x,y in zip(a,b))
        pa, pb, pc, pd = sq(ai,aj), sq(bi,bj), sq(ai,bj), sq(bi,aj)
        assert d2 == min(max(pa,pb), max(pc,pd))
        assert reversed_match == (max(pc,pd) < max(pa,pb))
        vec1, vec2 = B[i]-A[i], B[j]-A[j]
        angle = None
        if l1 and l2:
            cosine = abs(sum(int(x)*int(y) for x,y in zip(vec1,vec2))) / math.sqrt(l1*l2)
            angle = math.degrees(math.acos(min(1.0, cosine)))
        rules = {
            "exact": d2 == 0,
            "abs_1cm": d2 <= 1,
            "abs_5cm": d2 <= 25,
            "abs_10cm": d2 <= 100,
            "abs_50cm": d2 <= 2500,
            "rel_1pct": d2 <= lmin2 // 10000,
            "rel_2pct": d2 <= lmin2 // 2500,
            "rel_5pct": d2 <= lmin2 // 400,
            "rel_10pct": d2 <= lmin2 // 100,
            "min_5cm_5pct": d2 <= min(25, lmin2 // 400),
            "max_5cm_5pct": d2 <= max(25, lmin2 // 400),
        }
        rows.append(dict(leg_a=i+1, leg_b=j+1, distance_sq_cm=d2,
                         distance_m=math.sqrt(d2)/100,
                         length_a_m=math.sqrt(l1)/100, length_b_m=math.sqrt(l2)/100,
                         relative_error_percent=100*math.sqrt(d2/lmin2) if lmin2 else None,
                         angle_degrees=angle, reversed=reversed_match,
                         shared_endpoint=any(x==y for x in (ai,bi) for y in (aj,bj)),
                         both_splay="SPLAY" in legs[i]["flags"] and "SPLAY" in legs[j]["flags"],
                         any_splay="SPLAY" in legs[i]["flags"] or "SPLAY" in legs[j]["flags"],
                         same_survey=legs[i]["survey"]==legs[j]["survey"],
                         any_zero=not lmin2, rules=rules))

rows.sort(key=lambda r:(r["leg_a"],r["leg_b"]))
rules = list(rows[0]["rules"]) if rows else []
summary = {}
for rule in rules:
    selected = [r for r in rows if r["rules"][rule]]
    summary[rule] = {"pairs":len(selected),
                     **{key:sum(bool(r[key]) for r in selected)
                        for key in ("both_splay","any_splay","shared_endpoint","same_survey","any_zero")},
                     "length_bins":{name:sum(lo <= min(r["length_a_m"],r["length_b_m"]) < hi for r in selected)
                                    for name,lo,hi in [("0-0.1m",0,.1),("0.1-1m",.1,1),("1-5m",1,5),
                                                       ("5-20m",5,20),("20m+",20,math.inf)]}}

def enrich(leg):
    return dict(leg, names_a=sorted(names.get(tuple(leg["a"]), [])),
                names_b=sorted(names.get(tuple(leg["b"]), [])),
                anonymous_a=tuple(leg["a"]) in anonymous,
                anonymous_b=tuple(leg["b"]) in anonymous)

selected_ids = sorted({r[key] for r in rows for key in ("leg_a","leg_b")})
output = dict(source=source_label, source_sha256=source_hash,
              command=["dump3d", "--legs", "--show-dates", source_label],
              dump_sha256=hashlib.sha256(process.stdout).hexdigest(),
              dump3d_version=subprocess.check_output([command[0],"--version"],text=True).strip(),
              python=sys.version, numpy=np.__version__, platform=platform.platform(),
              leg_count=n, pair_count=n*(n-1)//2, max_squared_distance_bound_cm=max_squared_distance,
              length_quantiles_m={str(p):float(np.quantile(np.sqrt(length_sq)/100,p)) for p in (0,.1,.5,.9,.99,1)},
              zero_length_count=int(np.sum(length_sq==0)),
              elapsed_seconds=time.perf_counter()-START, counts=summary,
              legs={i:enrich(legs[i-1]) for i in selected_ids}, pairs=rows)
assert source_hash == hashlib.sha256(SOURCE.read_bytes()).hexdigest()
(OUT / "results.json").write_text(json.dumps(output,ensure_ascii=False,indent=2)+"\n")
with (OUT / "pairs.csv").open("w",newline="",encoding="utf-8") as handle:
    keys=[key for key in rows[0] if key!="rules"]
    names_keys=[f"{side}_{end}_names" for side in ("a","b") for end in ("a","b")]
    writer=csv.DictWriter(handle,fieldnames=keys+names_keys+["survey_a","survey_b"]+rules)
    writer.writeheader()
    for row in rows:
        named={f"{side}_{end}_names":json.dumps(output["legs"][row[f"leg_{side}"]][f"names_{end}"],ensure_ascii=False)
               for side in ("a","b") for end in ("a","b")}
        surveys={f"survey_{side}":legs[row[f"leg_{side}"]-1]["survey"] for side in ("a","b")}
        writer.writerow({**{k:row[k] for k in keys},**named,**surveys,**row["rules"]})
print(json.dumps({key:output[key] for key in ("leg_count","pair_count","zero_length_count","elapsed_seconds","counts")},ensure_ascii=False,indent=2))
