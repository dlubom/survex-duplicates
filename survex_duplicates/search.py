"""Exact, ordered streaming search over an orientation-independent KDTree key."""

from collections.abc import Iterator
from time import perf_counter

import numpy as np
from scipy.spatial import KDTree

from .geometry import Tolerance, match_legs, squared_distance_cm
from .models import Pair, SurveyModel


class Search:
    """Build once; consume pairs in input leg order. Counters describe the last iteration."""

    def __init__(
        self,
        model: SurveyModel,
        tolerance: Tolerance,
        exclude_splays: bool = False,
        include_duplicates: bool = False,
    ) -> None:
        start = perf_counter()
        self.model = model
        self.tolerance = tolerance
        self.n_input = len(model.legs)
        self._indices = [
            i
            for i, leg in enumerate(model.legs)
            if (include_duplicates or "DUPLICATE" not in leg.flags)
            and (not exclude_splays or "SPLAY" not in leg.flags)
        ]
        self.n = len(self._indices)
        self.n_excluded = self.n_input - self.n
        self.candidates = 0
        self.matches = 0
        a, b = model.A_cm[self._indices], model.B_cm[self._indices]
        self._features = np.concatenate([np.minimum(a, b), np.maximum(a, b)], axis=1).astype(
            np.float64
        )
        self._tree = KDTree(self._features)
        self._radii = [tolerance.query_radius_cm(squared_distance_cm(a, b)) for a, b in zip(a, b)]
        self.build_seconds = perf_counter() - start

    def pairs(self, batch_size: int = 1) -> Iterator[Pair]:
        """All matching pairs, ordered by (leg_a, leg_b), without a global pair list."""
        if batch_size <= 0:
            raise ValueError("batch_size must be positive")
        self.candidates = self.matches = 0
        for first in range(0, self.n, batch_size):
            stop = min(first + batch_size, self.n)
            neighbors = self._tree.query_ball_point(
                self._features[first:stop],
                r=self._radii[first:stop],
                p=np.inf,
                eps=0.0,
                return_sorted=True,
            )
            for i, row in enumerate(neighbors, first):
                source_i = self._indices[i]
                for j in row:
                    if j <= i:
                        continue
                    source_j = self._indices[j]
                    self.candidates += 1
                    match = match_legs(
                        self.model.A_cm[source_i],
                        self.model.B_cm[source_i],
                        self.model.A_cm[source_j],
                        self.model.B_cm[source_j],
                        self.tolerance,
                    )
                    if match is not None:
                        self.matches += 1
                        yield Pair(
                            self.model.legs[source_i].leg_id,
                            self.model.legs[source_j].leg_id,
                            match,
                        )
