"""Independent exhaustive oracle: no geometry predicate or spatial index imports."""

from fractions import Fraction
from itertools import combinations


def all_pairs(
    model, mode="relative_percent", value="5", exclude_splays=False, include_duplicates=False
):
    selected = [
        i
        for i, leg in enumerate(model.legs)
        if (include_duplicates or "DUPLICATE" not in leg.flags)
        and (not exclude_splays or "SPLAY" not in leg.flags)
    ]
    result = []
    parameter = Fraction(value)
    for i, j in combinations(selected, 2):
        a, b, c, d = [
            tuple(int(v) for v in point)
            for point in (model.A_cm[i], model.B_cm[i], model.A_cm[j], model.B_cm[j])
        ]
        aa = sum((x - y) ** 2 for x, y in zip(a, c))
        bb = sum((x - y) ** 2 for x, y in zip(b, d))
        ab = sum((x - y) ** 2 for x, y in zip(a, d))
        ba = sum((x - y) ** 2 for x, y in zip(b, c))
        reverse = max(ab, ba) < max(aa, bb)
        first, second = (ab, ba) if reverse else (aa, bb)
        if mode == "relative_percent":
            length_a = sum((x - y) ** 2 for x, y in zip(a, b))
            length_b = sum((x - y) ** 2 for x, y in zip(c, d))
            bound = parameter**2 * min(length_a, length_b) / 10000
        else:
            bound = parameter**2 * 10000
        if max(first, second) <= bound:
            result.append((model.legs[i].leg_id, model.legs[j].leg_id, reverse, first, second))
    return result
