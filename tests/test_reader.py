import shutil
import sys
from pathlib import Path

import pytest

from survex_duplicates.reader import ReaderError, parse_dump, read_model

FIXTURES = Path(__file__).parent / "fixtures"


def full_dump(body):
    header = (FIXTURES / "survey.legs.dump").read_text().split("--\n", 1)[0]
    return (header + "--\n" + body + "\nSTOP\n").splitlines()


def test_real_legs_dump_returns_ordered_geometry_and_shot_metadata():
    model = read_model(FIXTURES / "survey.legs.dump")
    assert model.A_cm.tolist() == [
        [0, 0, 0],
        [100, 0, 0],
        [100, 101, 0],
        [0, 0, 0],
        [100, 0, 0],
        [0, 0, 0],
    ]
    assert model.B_cm[0].tolist() == [100, 0, 0]
    assert model.A_cm.dtype.name == model.B_cm.dtype.name == "int64"
    assert [leg.leg_id for leg in model.legs] == [1, 2, 3, 4, 5, 6]
    leg = model.legs[3]
    assert (leg.source_line, leg.survey, leg.style, leg.flags, leg.survey_date) == (
        10,
        'p],"q[',
        "CARTESIAN",
        ("DUPLICATE",),
        "2024.02.13",
    )


def test_real_move_line_dump_has_the_same_legs_without_conflating_moves():
    model = read_model(FIXTURES / "survey.lines.dump")
    leg_model = read_model(FIXTURES / "survey.legs.dump")
    assert model.A_cm.tolist() == leg_model.A_cm.tolist()
    assert model.B_cm.tolist() == leg_model.B_cm.tolist()
    assert [(x.leg_id, x.survey, x.flags, x.style, x.survey_date) for x in model.legs] == [
        (x.leg_id, x.survey, x.flags, x.style, x.survey_date) for x in leg_model.legs
    ]


def test_nodes_preserve_all_exact_names_and_anonymity_without_moving_line_start():
    model = parse_dump(
        full_dump("""NODE 0 0 0 [Żółta:alias]
MOVE 0 0 0
MOVE 1 0 0
NODE 0 0 0 [a],"b[] UNDERGROUND
NODE 0 0 0 [] ANON
NODE 0 0 0 [Żółta:alias]
LINE 0 0 0 []
NODE 2 0 0 []""")
    )
    assert model.A_cm.tolist() == [[100, 0, 0]]
    station = model.station_at((0, 0, 0))
    assert (station.names, station.status, station.has_anonymous_node) == (
        ('a],"b[', "Żółta:alias"),
        "named",
        True,
    )
    assert model.station_at((200, 0, 0)).status == "unnamed"
    assert model.station_at((100, 0, 0)).status == "missing_node"
    assert model.legs[0].survey_date == model.legs[0].style == ""


@pytest.mark.parametrize(
    "bad",
    [
        "",
        "STOP\n",
        "TITLE broken\n--\nSTOP\n",
        "\n".join(full_dump("LEG 0 0 0 1 0 0 []")[:-1]),
        "\n".join(full_dump("")) + "\nLEG 0 0 0 1 0 0 []",
        "\n".join(full_dump("")) + "\n<<< REWIND <<<",
        "\n".join(full_dump("")) + "\n" + "\n".join(full_dump("")),
        "\n".join(full_dump("CODE_0x42 1")),
        "\n".join(full_dump("LINE 1 2 3 []")),
        "\n".join(full_dump("LEG 0 0 0 1 0 []")),
        "\n".join(full_dump("LEG 0 0 bad 1 0 0 []")),
        "\n".join(full_dump("LEG 0 0 0 1 0 0 []\nMOVE 0 0 0\nLINE 1 0 0 []")),
        "\n".join(full_dump("LEG 0.001 0 0 1 0 0 []")),
        "\n".join(full_dump("LEG 21474836.48 0 0 1 0 0 []")),
        "\n".join(full_dump("LEG 0 0 0 1 0 0 [] 2024.02.30")),
        "\n".join(full_dump("LEG 0 0 0 1 0 0 [] UNKNOWN")),
        "\n".join(full_dump("NODE 0 0 0 [x] UNKNOWN")),
        "\n".join(full_dump("XSECT broken")),
        "\n".join(full_dump("XSECT_END broken")),
        "\n".join(full_dump("ERROR_INFO broken")),
        "\n".join(full_dump("STOP extra")),
        "\n".join(full_dump("")).replace("VERSION 8", "VERSION 8\nEXTENDED ELEVATION"),
    ],
)
def test_reader_rejects_invalid_or_incomplete_stream_with_line_diagnostic(bad):
    with pytest.raises(ReaderError, match="line [0-9]+"):
        parse_dump(bad.splitlines())


@pytest.mark.integration
@pytest.mark.skipif(shutil.which("dump3d") is None, reason="Survex unavailable")
def test_real_binary_reader_matches_recorded_dump(tmp_path):
    path = tmp_path / "model ze spacją Żółć.3d"
    shutil.copyfile(FIXTURES / "survey.3d", path)
    model = read_model(path)
    expected = read_model(FIXTURES / "survey.legs.dump")
    assert model.A_cm.tolist() == expected.A_cm.tolist()
    assert model.B_cm.tolist() == expected.B_cm.tolist()
    assert model.legs == expected.legs
    assert model.stations == expected.stations
    assert model.input_format == "3d"


@pytest.mark.parametrize("partial", [False, True])
def test_dump3d_failure_reports_process_error_even_after_valid_stdout(tmp_path, partial):
    program = tmp_path / "fake dump3d"
    data = (FIXTURES / "survey.legs.dump").read_text() if partial else ""
    program.write_text(
        f"#!{sys.executable}\nimport sys\nsys.stdout.write({data!r})\n"
        "sys.stderr.write('unrecognized option --legs')\nsys.exit(9)\n"
    )
    program.chmod(0o755)
    with pytest.raises(ReaderError, match="dump3d.*9.*--legs"):
        read_model(FIXTURES / "survey.3d", dump3d=str(program))


def test_legacy_empty_survey_missing_style_and_date_and_closure_metadata():
    legacy = read_model(FIXTURES / "legacy.legs.dump")
    modern = read_model(FIXTURES / "closure.legs.dump")
    assert legacy.A_cm.tolist() == modern.A_cm.tolist()
    assert legacy.B_cm.tolist() == modern.B_cm.tolist()
    assert len(legacy.legs) == len(modern.legs) == 3
    assert all(x.survey == x.style == x.survey_date == "" for x in legacy.legs)
    assert all(x.style == "NORMAL" and x.survey_date == "" for x in modern.legs)
    assert read_model(FIXTURES / "survey.legs.dump").station_at((0, 100, 0)).has_anonymous_node


def test_real_extended_dump_is_rejected_with_useful_diagnostic():
    with pytest.raises(ReaderError, match="ordinary spatial model"):
        read_model(FIXTURES / "extended.legs.dump")


def test_exact_int32_boundaries_and_scientific_decimal_coordinates():
    model = parse_dump(full_dump("LEG -21474836.48 0 1e-2 21474836.47 0 -1e-2 []"))
    assert model.A_cm.tolist() == [[-(2**31), 0, 1]]
    assert model.B_cm.tolist() == [[2**31 - 1, 0, -1]]


def test_child_locale_and_large_stderr_do_not_deadlock(tmp_path, monkeypatch):
    program = tmp_path / "dump program"
    data = (FIXTURES / "survey.legs.dump").read_text()
    program.write_text(
        f"#!{sys.executable}\nimport os,sys\n"
        "assert 'LC_ALL' not in os.environ\nassert os.environ['LC_NUMERIC']=='C'\n"
        "assert os.environ['LC_TIME']=='pl_PL.UTF-8'\n"
        "assert sys.argv[1:3]==['--legs','--show-dates']\n"
        "sys.stderr.write('x'*2000000)\n" + f"sys.stdout.write({data!r})\n"
    )
    program.chmod(0o755)
    monkeypatch.setenv("LC_ALL", "pl_PL.UTF-8")
    monkeypatch.setenv("LC_TIME", "pl_PL.UTF-8")
    assert len(read_model(FIXTURES / "survey.3d", dump3d=str(program)).legs) == 6


@pytest.mark.integration
@pytest.mark.skipif(shutil.which("dump3d") is None, reason="Survex unavailable")
def test_real_extended_binary_is_rejected():
    with pytest.raises(ReaderError, match="ordinary spatial model"):
        read_model(FIXTURES / "extended.3d")
