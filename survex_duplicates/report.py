"""Stream report rows. Float roots appear only after exact qualification."""

import csv
import json
import os
import tempfile
from collections.abc import Iterable
from decimal import MAX_EMAX, MIN_EMIN, Decimal, localcontext
from math import sqrt
from pathlib import Path

from .geometry import Tolerance, squared_distance_cm
from .models import Pair, SurveyModel

HEADER = "leg_a,leg_b,survey_a,survey_b,source_line_a,source_line_b,distance_m,endpoint_1_error_m,endpoint_2_error_m,reversed,flags_a,flags_b,style_a,style_b,date_a,date_b,a_x1,a_y1,a_z1,a_x2,a_y2,a_z2,b_x1,b_y1,b_z1,b_x2,b_y2,b_z2,distance_cm,length_a_m,length_b_m,relative_error_percent,tolerance_mode,tolerance_value,tolerance_m,a1_station_names,a2_station_names,b1_station_names,b2_station_names,a1_station_status,a2_station_status,b1_station_status,b2_station_status,a1_has_anonymous_node,a2_has_anonymous_node,b1_has_anonymous_node,b2_has_anonymous_node".split(
    ","
)


def _float(value: float) -> str:
    return format(value, ".17g")


def _metres(cm: int) -> str:
    whole, fraction = divmod(abs(int(cm)), 100)
    return f"{'-' if cm < 0 else ''}{whole}.{fraction:02d}"


def _row(model: SurveyModel, pair: Pair, tolerance: Tolerance) -> list[object]:
    i, j = pair.leg_a - 1, pair.leg_b - 1
    a, b = model.legs[i], model.legs[j]
    points = (model.A_cm[i], model.B_cm[i], model.A_cm[j], model.B_cm[j])
    lengths = squared_distance_cm(points[0], points[1]), squared_distance_cm(points[2], points[3])
    shortest = min(lengths)
    match = pair.match
    distance = sqrt(match.distance_sq_cm)
    stations = [model.station_at((int(p[0]), int(p[1]), int(p[2]))) for p in points]
    with localcontext() as context:
        context.prec = max(40, len(tolerance.value.as_tuple().digits) + 20)
        context.Emax, context.Emin = MAX_EMAX, MIN_EMIN
        allowed = tolerance.value
        if tolerance.mode == "relative_percent":
            allowed = allowed * Decimal(shortest).sqrt() / Decimal(10000)
        tolerance_m = str(allowed)
    return [
        a.leg_id,
        b.leg_id,
        a.survey,
        b.survey,
        a.source_line,
        b.source_line,
        _float(distance / 100),
        _float(sqrt(match.endpoint_1_sq_cm) / 100),
        _float(sqrt(match.endpoint_2_sq_cm) / 100),
        int(match.reversed),
        "|".join(a.flags),
        "|".join(b.flags),
        a.style,
        b.style,
        a.survey_date,
        b.survey_date,
        *[_metres(int(coord)) for point in points for coord in point],
        _float(distance),
        _float(sqrt(lengths[0]) / 100),
        _float(sqrt(lengths[1]) / 100),
        _float(100 * sqrt(match.distance_sq_cm / shortest)) if shortest else "",
        tolerance.mode,
        str(tolerance.value),
        tolerance_m,
        *[json.dumps(s.names, ensure_ascii=False) for s in stations],
        *[s.status for s in stations],
        *[int(s.has_anonymous_node) for s in stations],
    ]


def write_report(
    path: Path, model: SurveyModel, pairs: Iterable[Pair], tolerance: Tolerance, force: bool = False
) -> None:
    """Publish a complete CSV atomically; failures leave any previous report intact."""
    temporary: Path | None = None
    try:
        with tempfile.NamedTemporaryFile(
            mode="w",
            encoding="utf-8",
            newline="",
            dir=path.parent,
            prefix=f".{path.name}.",
            suffix=".tmp",
            delete=False,
        ) as stream:
            temporary = Path(stream.name)
            writer = csv.writer(stream)
            writer.writerow(HEADER)
            for pair in pairs:
                writer.writerow(_row(model, pair, tolerance))
            stream.flush()
            os.fsync(stream.fileno())
        if force:
            os.replace(temporary, path)
        else:
            # Atomic create-if-absent, including a destination created mid-search.
            os.link(temporary, path)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def validate_destination(source: Path, destination: Path, force: bool) -> None:
    if destination.resolve() == source.resolve() or (
        destination.exists() and source.samefile(destination)
    ):
        raise ValueError(
            "output must not refer to the input file, including through a symlink or hardlink"
        )
    if (destination.exists() or destination.is_symlink()) and not force:
        raise ValueError("output already exists; use --force to replace it")
