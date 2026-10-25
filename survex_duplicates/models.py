"""Shared records; coordinates are exact signed int32 centimetres in int64 arrays."""

from dataclasses import dataclass, field
from typing import TypeAlias

import numpy as np
from numpy.typing import NDArray

Point: TypeAlias = tuple[int, int, int]


@dataclass(frozen=True)
class Leg:
    leg_id: int
    source_line: int
    survey: str = ""
    flags: tuple[str, ...] = ()
    style: str = ""
    survey_date: str = ""


@dataclass(frozen=True)
class Match:
    reversed: bool
    endpoint_1_sq_cm: int
    endpoint_2_sq_cm: int

    @property
    def distance_sq_cm(self) -> int:
        return max(self.endpoint_1_sq_cm, self.endpoint_2_sq_cm)


@dataclass(frozen=True)
class Pair:
    leg_a: int
    leg_b: int
    match: Match


@dataclass
class StationNames:
    names: tuple[str, ...] = ()
    has_anonymous_node: bool = False
    status: str = "missing_node"


@dataclass
class SurveyModel:
    A_cm: NDArray[np.int64]
    B_cm: NDArray[np.int64]
    legs: tuple[Leg, ...]
    header: dict[str, str] = field(default_factory=dict)
    input_format: str = "dump"
    stations: dict[Point, StationNames] = field(default_factory=dict)

    def station_at(self, point: Point) -> StationNames:
        """All exact-position names, including anonymous-node coexistence."""
        return self.stations.get(point, StationNames())
