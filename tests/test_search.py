import random

import numpy as np
import pytest

from survex_duplicates.geometry import Tolerance
from survex_duplicates.models import Leg, SurveyModel
from survex_duplicates.search import Search
from tests.oracle import all_pairs


def model_of(legs, flags=None):
    return SurveyModel(
        np.array([x[0] for x in legs], dtype=np.int64).reshape(-1, 3),
        np.array([x[1] for x in legs], dtype=np.int64).reshape(-1, 3),
        tuple(
            Leg(i + 1, i + 8, f"survey-{i}", flags[i] if flags else ()) for i in range(len(legs))
        ),
    )


def signatures(search, batch_size=1):
    return [
        (p.leg_a, p.leg_b, p.match.reversed, p.match.endpoint_1_sq_cm, p.match.endpoint_2_sq_cm)
        for p in search.pairs(batch_size=batch_size)
    ]


def test_three_records_keep_distinct_identity_and_ordered_pairs():
    model = model_of([((0, 0, 0), (100, 0, 0)), ((100, 0, 0), (0, 0, 0)), ((0, 0, 0), (100, 0, 0))])
    search = Search(model, Tolerance.absolute(".05"))
    assert signatures(search) == [(1, 2, True, 0, 0), (1, 3, False, 0, 0), (2, 3, True, 0, 0)]
    assert (search.n, search.candidates, search.matches) == (3, 3, 3)


def test_acknowledged_duplicate_legs_are_excluded_by_default_without_renumbering():
    model = model_of(
        [((0, 0, 0), (100, 0, 0))] * 4, [(), ("DUPLICATE",), ("SURFACE",), ("DUPLICATE", "SPLAY")]
    )
    search = Search(model, Tolerance.relative())
    assert signatures(search) == [(1, 3, False, 0, 0)]
    assert (search.n_input, search.n_excluded, search.n) == (4, 2, 2)


@pytest.mark.parametrize("seed", [7, 20261025, 91])
@pytest.mark.parametrize(
    "tolerance",
    [
        Tolerance.relative(),
        Tolerance.relative("17.3"),
        Tolerance.absolute(".05"),
        Tolerance.absolute(".0099"),
    ],
)
@pytest.mark.parametrize("exclude", [False, True])
@pytest.mark.parametrize("include", [False, True])
def test_search_matches_independent_oracle_with_filter_variable_radii_and_batches(
    seed, tolerance, exclude, include
):
    rng = random.Random(seed)
    legs = []
    for i in range(45):
        a = tuple(rng.randrange(-200, 201) for _ in range(3))
        b = tuple(a[k] + rng.randrange(-1500, 1501) for k in range(3))
        legs.append((a, b))
        if i % 3 == 0:
            legs.append((b, a))
        if i % 5 == 0:
            delta = (3, 4, 0)
            legs.append(
                (tuple(a[k] + delta[k] for k in range(3)), tuple(b[k] + delta[k] for k in range(3)))
            )
    legs += [
        ((0, 0, 0), (100, 0, 0)),
        ((3, 4, 0), (103, 4, 0)),
        ((0, 0, 0), (0, 0, 0)),
        ((0, 0, 0), (0, 0, 0)),
        ((-2, 0, 0), (2, 100, 0)),
        ((2, 0, 0), (-2, 100, 0)),
        ((0, 0, 0), (1000, 0, 0)),
        ((30, 40, 0), (1030, 40, 0)),
    ]
    flags = [("SPLAY", "SURFACE") if i % 4 == 0 else ("DUPLICATE",) for i in range(len(legs))]
    model = model_of(legs, flags)
    expected = all_pairs(model, tolerance.mode, str(tolerance.value), exclude, include)
    for batch in (1, 2, 17, 1000):
        search = Search(model, tolerance, exclude_splays=exclude, include_duplicates=include)
        assert signatures(search, batch) == expected
        assert search.n_input == len(legs)
        assert search.n_excluded == sum(
            (exclude and "SPLAY" in f) or (not include and "DUPLICATE" in f) for f in flags
        )
        assert search.matches == len(expected)


@pytest.mark.parametrize("tolerance", [Tolerance.relative(), Tolerance.absolute(".05")])
def test_empty_single_dense_and_nontransitive_sets_match_oracle(tolerance):
    examples = [
        [],
        [((0, 0, 0), (100, 0, 0))],
        [((0, 0, 0), (100, 0, 0))] * 80,
        [((0, y, 0), (100, y, 0)) for y in (0, 4, 8)],
        [((-(2**31),) * 3, (2**31 - 1,) * 3), (((2**31 - 1),) * 3, (-(2**31),) * 3)],
    ]
    for legs in examples:
        model = model_of(legs)
        assert signatures(Search(model, tolerance)) == all_pairs(
            model, tolerance.mode, str(tolerance.value)
        )


@pytest.mark.parametrize(
    "epsilon,separation,candidates,matched",
    [
        (".0499999999999999999999", 5, 1, False),
        (".05", 5, 1, True),
        (".0500000000000000000001", 6, 1, False),
        (".0099", 1, 1, False),
        (".01", 1, 1, True),
    ],
)
def test_outward_radius_below_on_and_above_square_boundaries(
    epsilon, separation, candidates, matched
):
    model = model_of([((0, 0, 0), (100, 0, 0)), ((0, separation, 0), (100, separation, 0))])
    search = Search(model, Tolerance.absolute(epsilon))
    found = signatures(search)
    assert bool(found) == matched
    assert search.candidates == candidates


@pytest.mark.parametrize(
    "tolerance",
    [Tolerance.relative("1e1000"), Tolerance.absolute("1e1000"), Tolerance.absolute("42949672.96")],
)
def test_capped_radius_never_loses_extreme_coordinate_pairs(tolerance):
    lo, hi = -(2**31), 2**31 - 1
    model = model_of(
        [((lo, lo, lo), (hi, hi, hi)), ((hi, hi, hi), (lo, lo, lo)), ((lo, hi, lo), (hi, lo, hi))]
    )
    search = Search(model, tolerance)
    assert signatures(search) == all_pairs(model, tolerance.mode, str(tolerance.value))
    assert search.candidates == 3


def test_zero_relative_query_radius_and_different_length_orderings():
    for legs in (
        [((0, 0, 0), (0, 0, 0)), ((1, 0, 0), (1, 0, 0)), ((0, 0, 0), (0, 0, 0))],
        [((0, 0, 0), (100, 0, 0)), ((-2, 3, 0), (110, 3, 0))],
        [((-2, 3, 0), (110, 3, 0)), ((0, 0, 0), (100, 0, 0))],
    ):
        model = model_of(legs)
        for batch in (1, 2, 99):
            assert signatures(Search(model, Tolerance.relative("10")), batch) == all_pairs(
                model, value="10"
            )


@pytest.mark.parametrize(
    "percent,legs,expected",
    [
        ("5", [((0, 0, 0), (100, 100, 0)), ((5, 5, 0), (105, 105, 0))], True),
        ("4.9999", [((0, 0, 0), (100, 100, 0)), ((5, 5, 0), (105, 105, 0))], False),
        ("5", [((0, 0, 0), (4, 0, 0)), ((4, 0, 0), (0, 0, 0))], True),
        ("5", [((0, 0, 0), (4, 0, 0)), ((0, 1, 0), (4, 1, 0))], False),
        ("5", [((0, 0, 0), (1000, 0, 0)), ((30, 40, 0), (1030, 40, 0))], True),
        ("5", [((0, 0, 0), (100, 0, 0)), ((0, 0, 6), (100, 0, 6))], False),
    ],
)
def test_relative_acceptance_examples_through_full_search(percent, legs, expected):
    assert bool(signatures(Search(model_of(legs), Tolerance.relative(percent)))) == expected


@pytest.mark.parametrize("kind", ["sparse", "repeated", "dense"])
@pytest.mark.parametrize("mode,value", [("relative_percent", "5"), ("absolute_m", ".05")])
@pytest.mark.parametrize("exclude", [False, True])
@pytest.mark.parametrize("include", [False, True])
def test_small_benchmark_construction_is_checked_by_independent_oracle(
    tmp_path, kind, mode, value, exclude, include
):
    from benchmarks.datasets import expected_pairs, make_dump
    from survex_duplicates.reader import read_model

    path = tmp_path / "bench.dump"
    make_dump(path, 80, kind, 20261025)
    model = read_model(path)
    tolerance = (
        Tolerance.relative(value) if mode == "relative_percent" else Tolerance.absolute(value)
    )
    expected = all_pairs(model, mode, value, exclude, include)
    assert signatures(Search(model, tolerance, exclude, include)) == expected
    assert [(r[0], r[1], r[2]) for r in expected] == list(
        expected_pairs(80, kind, mode, exclude, include)
    )
