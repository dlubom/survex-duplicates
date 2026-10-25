"""Separated cells give a known result without relying on the detector."""

import random
from itertools import combinations
from pathlib import Path


def _metres(point):
    return " ".join(f"{v / 100:.2f}" for v in point)


def make_dump(path: Path, n: int, kind: str, seed: int):
    if kind not in {"sparse", "repeated", "dense"} or n < 0 or (kind == "repeated" and n % 2):
        raise ValueError("invalid benchmark construction")
    rng = random.Random(seed)
    with path.open("w", encoding="utf-8") as stream:
        stream.write(
            'TITLE "Benchmark"\nDATE "@0"\nDATE_NUMERIC 0\nVERSION 8\nSEPARATOR \'.\'\n--\n'
        )
        for group in range(n // 2 if kind == "repeated" else n):
            origin = ((group % 1000) * 10000, (group // 1000) * 10000, rng.randrange(-3000, 3001))
            length = 3000 if group % 4 == 0 else rng.choice((4, 20, 100, 1000, 3000))
            direction = rng.choice(((1, 0, 0), (0, 1, 0), (0, 0, 1), (1, 1, 0)))
            end = tuple(origin[k] + length * direction[k] for k in range(3))
            if kind == "dense":
                origin, end = (0, 0, 0), (1000, 0, 0)
                if group % 2:
                    origin, end = end, origin
            flags = " SPLAY" if kind != "repeated" and group % 10 == 0 else ""
            stream.write(
                f"LEG {_metres(origin)} {_metres(end)} [bench:{group}] STYLE=NORMAL{flags}\n"
            )
            if kind == "repeated":
                shift = (30, 40, 0) if group % 4 == 0 else (0, 0, 0)
                other_start = tuple(end[k] + shift[k] for k in range(3))
                other_end = tuple(origin[k] + shift[k] for k in range(3))
                flags = " SPLAY" if group % 10 == 0 else " DUPLICATE"
                stream.write(
                    f"LEG {_metres(other_start)} {_metres(other_end)} [copy:{group}] STYLE=NORMAL{flags}\n"
                )
        stream.write("STOP\n")


def expected_pairs(n, kind, mode, exclude_splays, include_duplicates=False):
    """Full ordered pair IDs and orientations implied by cell construction."""
    if kind == "repeated":
        for group in range(n // 2):
            if not include_duplicates and group % 10 != 0:
                continue
            if exclude_splays and group % 10 == 0:
                continue
            if mode == "absolute_m" and group % 4 == 0:
                continue
            yield 2 * group + 1, 2 * group + 2, True
    elif kind == "dense":
        selected = (i for i in range(n) if not exclude_splays or i % 10 != 0)
        for i, j in combinations(selected, 2):
            yield i + 1, j + 1, bool(i % 2 != j % 2)
