"""Exact endpoint predicates; no files, index, or floating-point qualification."""

import re
from dataclasses import dataclass, field
from decimal import Decimal, InvalidOperation
from fractions import Fraction
from math import isqrt
from typing import Sequence

import numpy as np
from numpy.typing import NDArray

from ._decimal_syntax import DECIMAL_NUMBER_PATTERN
from .models import Match

Coordinates = Sequence[int] | NDArray[np.int64]


@dataclass(frozen=True)
class Tolerance:
    mode: str
    value: Decimal
    _scale_sq: Fraction = field(init=False, repr=False)

    def __post_init__(self) -> None:
        if self.mode not in {"absolute_m", "relative_percent"}:
            raise ValueError("unknown tolerance mode")
        if not self.value.is_finite() or self.value <= 0:
            raise ValueError("tolerance must be a positive, finite decimal number")
        scale = Fraction(self.value) * (100 if self.mode == "absolute_m" else Fraction(1, 100))
        object.__setattr__(self, "_scale_sq", scale * scale)

    @classmethod
    def absolute(cls, text: str) -> "Tolerance":
        return cls("absolute_m", _decimal(text))

    @classmethod
    def relative(cls, text: str = "5") -> "Tolerance":
        return cls("relative_percent", _decimal(text))

    def threshold_sq_cm(self, length_a_sq: int, length_b_sq: int) -> int:
        multiplier = min(length_a_sq, length_b_sq) if self.mode == "relative_percent" else 1
        return self._scale_sq.numerator * multiplier // self._scale_sq.denominator

    def query_radius_cm(self, length_sq: int) -> int:
        """Exact outward-rounded radius, capped at the int32 coordinate diameter."""
        numerator = self._scale_sq.numerator * (length_sq if self.mode == "relative_percent" else 1)
        denominator = self._scale_sq.denominator
        cap = 2**32
        if numerator >= cap * cap * denominator:
            return cap
        radius = isqrt(numerator // denominator)
        return radius + (radius * radius * denominator < numerator)


def _decimal(text: str) -> Decimal:
    if not re.fullmatch(DECIMAL_NUMBER_PATTERN, text):
        raise ValueError("tolerance must be a decimal number")
    try:
        return Decimal(text)
    except InvalidOperation as exc:
        raise ValueError("tolerance must be a decimal number") from exc


def squared_distance_cm(a: Coordinates, b: Coordinates) -> int:
    """Python ints prevent overflow across the complete int32 input range."""
    return sum((int(a[k]) - int(b[k])) ** 2 for k in range(3))


def match_legs(
    a1: Coordinates, a2: Coordinates, b1: Coordinates, b2: Coordinates, tolerance: Tolerance
) -> Match | None:
    """Return exact squared errors for a matching pair, otherwise None."""
    errors = squared_distance_cm(a1, b1), squared_distance_cm(a2, b2)
    reverse_errors = squared_distance_cm(a1, b2), squared_distance_cm(a2, b1)
    reversed_ = max(reverse_errors) < max(errors)
    if reversed_:
        errors = reverse_errors
    threshold = tolerance.threshold_sq_cm(squared_distance_cm(a1, a2), squared_distance_cm(b1, b2))
    return Match(reversed_, *errors) if max(errors) <= threshold else None
