import hashlib
import json
import os
import random
import shutil
from pathlib import Path

import pytest

from survex_duplicates.geometry import Tolerance
from survex_duplicates.models import SurveyModel
from survex_duplicates.reader import read_model
from survex_duplicates.search import Search
from tests.oracle import all_pairs
from tests.test_search import signatures

ROOT = Path(__file__).resolve().parents[1]
KATASTER = Path(
    os.environ.get(
        "KATASTER_PATH",
        str(ROOT / "examples/kataster/KATASTER.3d"),
    )
)
SHA256 = "55c9a785bd4494cd4c8e41dbcdfd106eedfd4e415558e7d4cee856a789821077"


@pytest.fixture(scope="module")
def kataster():
    if not KATASTER.is_file() or shutil.which("dump3d") is None:
        pytest.skip("KATASTER_PATH or Survex unavailable")
    with KATASTER.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    assert digest == SHA256, f"Changed input {digest}; historical expectations must not be reused"
    # Only load historical expectations after validating the actual input hash.
    reference = json.loads((ROOT / "experiments/tolerance-2026-10-25/results.json").read_text())
    assert reference["source_sha256"] == digest
    return read_model(KATASTER), reference


@pytest.mark.integration
@pytest.mark.parametrize("exclude", [False, True])
@pytest.mark.parametrize("include", [False, True])
@pytest.mark.parametrize(
    "tolerance,rule", [(Tolerance.relative(), "rel_5pct"), (Tolerance.absolute(".05"), "abs_5cm")]
)
def test_full_kataster_pairs_and_orientations_equal_historical_exhaustive_result(
    kataster, tolerance, rule, exclude, include
):
    model, reference = kataster
    expected = [
        (p["leg_a"], p["leg_b"], p["reversed"], p["distance_sq_cm"])
        for p in reference["pairs"]
        if p["rules"][rule]
        and (not exclude or not p["any_splay"])
        and (
            include
            or all(
                "DUPLICATE" not in reference["legs"][str(p[key])]["flags"]
                for key in ("leg_a", "leg_b")
            )
        )
    ]
    for batch in (1, 19):
        search = Search(model, tolerance, exclude, include)
        actual = [
            (p.leg_a, p.leg_b, p.match.reversed, p.match.distance_sq_cm)
            for p in search.pairs(batch)
        ]
        assert actual == expected
        assert (search.n_input, search.n_excluded, search.n) == (
            18584,
            (6431 if exclude else 0) if include else (6433 if exclude else 2),
            (12153 if exclude else 18584) if include else (12151 if exclude else 18582),
        )
        if rule == "rel_5pct":
            assert search.matches == (
                (62 if exclude else 220) if include else (60 if exclude else 218)
            )
            pairs = {(p[0], p[1]) for p in actual}
            assert {
                (1448, 7948),
                (1449, 7949),
                (7950, 7961),
                (7951, 7962),
                (7952, 7963),
                (7953, 7964),
            } <= pairs
            if include:
                assert {(2836, 14209), (2837, 14228)} <= pairs
            else:
                assert not ({(2836, 14209), (2837, 14228)} & pairs)


@pytest.mark.integration
@pytest.mark.parametrize("tolerance", [Tolerance.relative(), Tolerance.absolute(".05")])
@pytest.mark.parametrize("exclude", [False, True])
@pytest.mark.parametrize("include", [False, True])
def test_kataster_seeded_512_leg_sample_matches_independent_python_integer_oracle(
    kataster, tolerance, exclude, include
):
    model, _ = kataster
    selected = {2835, 14208, 2836, 14227, 1447, 7947, 1448, 7948}
    rng = random.Random(20261025)
    while len(selected) < 512:
        selected.add(rng.randrange(len(model.legs)))
    indices = sorted(selected)
    subset = SurveyModel(
        model.A_cm[indices], model.B_cm[indices], tuple(model.legs[i] for i in indices)
    )
    expected = all_pairs(subset, tolerance.mode, str(tolerance.value), exclude, include)
    assert signatures(Search(subset, tolerance, exclude, include), 17) == expected


@pytest.mark.integration
def test_real_binary_and_both_dump_variants_produce_same_matches():
    if shutil.which("dump3d") is None:
        pytest.skip("Survex unavailable")
    models = [
        read_model(ROOT / "tests/fixtures" / name)
        for name in ("survey.3d", "survey.legs.dump", "survey.lines.dump")
    ]
    for tolerance in (Tolerance.relative(), Tolerance.absolute(".05")):
        for exclude in (False, True):
            results = [signatures(Search(model, tolerance, exclude), 2) for model in models]
            assert results[0] == results[1] == results[2]
