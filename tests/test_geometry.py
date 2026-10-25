import numpy as np
import pytest

from survex_duplicates.geometry import Tolerance, match_legs


def test_identical_records_match_at_both_endpoints():
    result = match_legs((0, 0, 0), (100, 0, 0), (0, 0, 0), (100, 0, 0), Tolerance.absolute("0.05"))
    assert result is not None
    assert (result.reversed, result.endpoint_1_sq_cm, result.endpoint_2_sq_cm) == (False, 0, 0)


@pytest.mark.parametrize(
    "a1,a2,b1,b2,expected",
    [
        ((0, 0, 0), (100, 0, 0), (100, 0, 0), (0, 0, 0), (True, 0, 0)),
        ((0, 0, 0), (100, 0, 0), (3, 4, 0), (103, 4, 0), (False, 25, 25)),
        ((0, 0, 0), (100, 0, 0), (5, 1, 0), (100, 0, 0), None),
        ((0, 0, 0), (100, 100, 0), (0, 100, 0), (100, 0, 0), None),
        ((0, 0, 0), (100, 0, 0), (0, 0, 5), (100, 0, 5), (False, 25, 25)),
        ((0, 0, 0), (100, 0, 0), (0, 0, 6), (100, 0, 6), None),
        ((0, 0, 0), (2, 0, 0), (2, 1, 0), (0, 1, 0), (True, 1, 1)),
        ((0, 0, 0), (1, 0, 0), (5, 0, 0), (3, 4, 0), (False, 25, 20)),
        ((0, 0, 0), (0, 0, 0), (3, 4, 0), (3, 4, 0), (False, 25, 25)),
        ((-2, 0, 0), (2, 100, 0), (2, 0, 0), (-2, 100, 0), (False, 16, 16)),
    ],
)
@pytest.mark.parametrize("translation", [0, -100, 100000000])
def test_absolute_acceptance_geometry_and_orientation(a1, a2, b1, b2, expected, translation):
    points = [np.array(p, dtype=np.int64) + translation for p in (a1, a2, b1, b2)]
    result = match_legs(*points, Tolerance.absolute("5e-2"))
    actual = (
        None
        if result is None
        else (result.reversed, result.endpoint_1_sq_cm, result.endpoint_2_sq_cm)
    )
    assert actual == expected


@pytest.mark.parametrize(
    "a1,a2,b1,b2,percent,expected",
    [
        ((0, 0, 0), (100, 0, 0), (3, 4, 0), (103, 4, 0), "5", (False, 25, 25)),
        ((0, 0, 0), (100, 0, 0), (0, 0, 6), (100, 0, 6), "5", None),
        ((0, 0, 0), (1000, 0, 0), (30, 40, 0), (1030, 40, 0), "5", (False, 2500, 2500)),
        ((0, 0, 0), (4, 0, 0), (4, 0, 0), (0, 0, 0), "5", (True, 0, 0)),
        ((0, 0, 0), (4, 0, 0), (0, 1, 0), (4, 1, 0), "5", None),
        ((0, 0, 0), (100, 0, 0), (-2, 3, 0), (110, 3, 0), "10", None),
        ((-2, 3, 0), (110, 3, 0), (0, 0, 0), (100, 0, 0), "10", None),
        ((0, 0, 0), (100, 100, 0), (5, 5, 0), (105, 105, 0), "5", (False, 50, 50)),
        ((0, 0, 0), (100, 100, 0), (5, 5, 0), (105, 105, 0), "4.9999", None),
        ((0, 0, 0), (0, 0, 0), (0, 0, 0), (0, 0, 0), "5", (False, 0, 0)),
        ((0, 0, 0), (0, 0, 0), (0, 1, 0), (0, 1, 0), "5", None),
        ((0, 0, 0), (0, 0, 0), (0, 0, 0), (0, 1, 0), "5", None),
    ],
)
def test_relative_shorter_length_exact_boundaries_and_point_legs(a1, a2, b1, b2, percent, expected):
    result = match_legs(a1, a2, b1, b2, Tolerance.relative(percent))
    actual = (
        None
        if result is None
        else (result.reversed, result.endpoint_1_sq_cm, result.endpoint_2_sq_cm)
    )
    assert actual == expected


@pytest.mark.parametrize("factory", [Tolerance.relative, Tolerance.absolute])
@pytest.mark.parametrize(
    "value", ["0", "-1", "NaN", "Infinity", "-Infinity", "1/2", "oops", "", "1_0"]
)
def test_tolerances_require_positive_finite_decimal(factory, value):
    with pytest.raises(ValueError):
        factory(value)


@pytest.mark.parametrize(
    "tolerance,expected",
    [(Tolerance.absolute("0.0099"), False), (Tolerance.absolute("0.01"), True)],
)
def test_subcentimetre_tolerances_are_exact(tolerance, expected):
    assert (
        match_legs((0, 0, 0), (100, 0, 0), (0, 0, 1), (100, 0, 1), tolerance) is not None
    ) == expected


@pytest.mark.parametrize("tolerance", [Tolerance.absolute(".05"), Tolerance.relative()])
def test_extreme_int32_diagonals_do_not_overflow(tolerance):
    lo, hi = -(2**31), 2**31 - 1
    points = [
        np.array(p, dtype=np.int64)
        for p in [(lo, lo, lo), (hi, hi, hi), (lo, hi, lo), (hi, lo, hi)]
    ]
    assert match_legs(*points, tolerance) is None
    match = match_legs(points[0], points[1], points[1], points[0], tolerance)
    assert match is not None and match.reversed and match.distance_sq_cm == 0
