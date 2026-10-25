import csv
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from survex_duplicates.cli import main
from survex_duplicates.reader import read_model
from tests.oracle import all_pairs
from tests.test_reader import full_dump

HEADER = "leg_a,leg_b,survey_a,survey_b,source_line_a,source_line_b,distance_m,endpoint_1_error_m,endpoint_2_error_m,reversed,flags_a,flags_b,style_a,style_b,date_a,date_b,a_x1,a_y1,a_z1,a_x2,a_y2,a_z2,b_x1,b_y1,b_z1,b_x2,b_y2,b_z2,distance_cm,length_a_m,length_b_m,relative_error_percent,tolerance_mode,tolerance_value,tolerance_m,a1_station_names,a2_station_names,b1_station_names,b2_station_names,a1_station_status,a2_station_status,b1_station_status,b2_station_status,a1_has_anonymous_node,a2_has_anonymous_node,b1_has_anonymous_node,b2_has_anonymous_node".split(
    ","
)


def write_dump(tmp_path, body):
    path = tmp_path / "wejście Żółć.dump"
    path.write_text("\n".join(full_dump(body)), encoding="utf-8")
    return path


def read_csv(path):
    with path.open(encoding="utf-8", newline="") as stream:
        reader = csv.DictReader(stream)
        assert reader.fieldnames == HEADER
        return list(reader)


def test_cli_excludes_acknowledged_duplicates_and_can_include_them_for_audit(tmp_path, capsys):
    path = write_dump(
        tmp_path, "LEG 0 0 0 1 0 0 []\nLEG 0 0 0 1 0 0 [] DUPLICATE\nLEG 0 0 0 1 0 0 []"
    )
    out = tmp_path / "filtered.csv"
    assert main([str(path), "--out", str(out)]) == 0
    assert [(r["leg_a"], r["leg_b"]) for r in read_csv(out)] == [("1", "3")]
    summary = json.loads(capsys.readouterr().err)
    assert (
        summary["n_input"],
        summary["n_excluded"],
        summary["n"],
        summary["include_duplicates"],
    ) == (3, 1, 2, False)
    assert main([str(path), "--out", str(out), "--force", "--include-duplicates"]) == 0
    assert [(r["leg_a"], r["leg_b"]) for r in read_csv(out)] == [("1", "2"), ("1", "3"), ("2", "3")]


@pytest.mark.parametrize("flags", ["DUPLICATE", "DUPLICATE SPLAY SURFACE"])
def test_all_acknowledged_duplicates_produce_header_only_and_union_counts(tmp_path, capsys, flags):
    path = write_dump(tmp_path, f"LEG 0 0 0 1 0 0 [] {flags}\nLEG 0 0 0 1 0 0 [] {flags}")
    out = tmp_path / "empty.csv"
    assert main([str(path), "--out", str(out), "--exclude-splays"]) == 0
    assert read_csv(out) == []
    summary = json.loads(capsys.readouterr().err)
    assert (summary["n_input"], summary["n_excluded"], summary["n"], summary["K"]) == (2, 2, 0, 0)


def test_cli_writes_complete_csv_and_summary_using_default_relative_five_percent(tmp_path, capsys):
    path = write_dump(
        tmp_path,
        """LEG 0 0 0 1 0 0 [pierwsza] STYLE=NORMAL 2024.02.13
LEG 1 0 0 0 0 0 [druga] DUPLICATE
NODE 0 0 0 [Żółć],"a[]
NODE 0 0 0 [] ANON
NODE 1 0 0 []""",
    )
    out = tmp_path / "wynik ze spacją.csv"
    assert main([str(path), "--out", str(out), "--include-duplicates"]) == 0
    rows = read_csv(out)
    assert len(rows) == 1
    row = rows[0]
    assert (row["leg_a"], row["leg_b"], row["reversed"]) == ("1", "2", "1")
    assert row["source_line_a"] == "7"
    assert row["survey_a"] == "pierwsza"
    assert row["date_a"] == "2024.02.13" and row["date_b"] == ""
    assert row["distance_m"] == row["distance_cm"] == "0"
    assert row["tolerance_mode"] == "relative_percent" and row["tolerance_value"] == "5"
    assert float(row["tolerance_m"]) == 0.05
    assert row["a_x1"] == "0.00" and row["a_x2"] == "1.00"
    assert json.loads(row["a1_station_names"]) == ['Żółć],"a[']
    assert row["a1_station_status"] == "named" and row["a1_has_anonymous_node"] == "1"
    assert row["a2_station_status"] == "unnamed" and row["b1_station_status"] == "unnamed"
    output = capsys.readouterr()
    assert output.out == ""
    summary = json.loads(output.err)
    assert (
        summary["n_input"],
        summary["n_excluded"],
        summary["n"],
        summary["C"],
        summary["K"],
    ) == (2, 0, 2, 1, 1)


@pytest.mark.parametrize("option,value", [("--relative-percent", "5"), ("--epsilon", "5e-2")])
@pytest.mark.parametrize("exclude", [False, True])
@pytest.mark.parametrize("include", [False, True])
def test_cli_tolerance_filter_and_batches_preserve_original_ids_and_csv(
    tmp_path, capsys, option, value, exclude, include
):
    path = write_dump(
        tmp_path,
        """LEG 0 0 0 1 0 0 [a]
LEG 0 0 0 1 0 0 [b] SPLAY
LEG 0 0 0 1 0 0 [c] DUPLICATE
LEG 0 0 0 1 0 0 [d] SURFACE SPLAY DUPLICATE
LEG 0 0 0 1 0 0 [e] SURFACE
NODE 0 0 0 [z]""",
    )
    mode = "relative_percent" if option == "--relative-percent" else "absolute_m"
    expected = all_pairs(read_model(path), mode, value, exclude, include)
    contents = []
    for batch in [1, 2, 100]:
        out = tmp_path / f"out{batch}.csv"
        args = [str(path), "--out", str(out), option, value, "--batch-size", str(batch)]
        if exclude:
            args.append("--exclude-splays")
        if include:
            args.append("--include-duplicates")
        assert main(args) == 0
        rows = read_csv(out)
        actual = [(int(r["leg_a"]), int(r["leg_b"]), bool(int(r["reversed"]))) for r in rows]
        assert actual == [r[:3] for r in expected]
        assert [r["source_line_a"] for r in rows] == [str(6 + r[0]) for r in expected]
        contents.append(out.read_bytes())
        summary = json.loads(capsys.readouterr().err)
        expected_excluded = (2 if exclude else 0) if include else (3 if exclude else 2)
        assert summary["n_excluded"] == expected_excluded
        assert summary["exclude_splays"] == exclude
    assert contents[0] == contents[1] == contents[2]


@pytest.mark.parametrize(
    "args",
    [
        [],
        ["x"],
        ["x", "--out", "y", "--epsilon", "0"],
        ["x", "--out", "y", "--relative-percent", "NaN"],
        ["x", "--out", "y", "--epsilon", "Infinity"],
        ["x", "--out", "y", "--relative-percent", "-5"],
        ["x", "--out", "y", "--epsilon", "1", "--relative-percent", "5"],
        ["x", "--out", "y", "--batch-size", "0"],
        ["x", "--out", "y", "--batch-size", "1.5"],
        ["x", "--out", "y", "--input-format", "bad"],
    ],
)
def test_argument_errors_return_two_with_diagnostic(args, capsys):
    assert main(args) == 2
    output = capsys.readouterr()
    assert output.out == "" and output.err


def test_existing_output_requires_force_and_force_replaces_only_after_success(tmp_path, capsys):
    path = write_dump(tmp_path, "LEG 0 0 0 1 0 0 []")
    out = tmp_path / "out.csv"
    out.write_bytes(b"poprzedni wynik")
    assert main([str(path), "--out", str(out)]) == 1
    assert out.read_bytes() == b"poprzedni wynik"
    assert main([str(path), "--out", str(out), "--force"]) == 0
    assert read_csv(out) == []
    good = out.read_bytes()
    path.write_text("urwany dump")
    assert main([str(path), "--out", str(out), "--force"]) == 1
    assert out.read_bytes() == good
    assert sorted(p.name for p in tmp_path.iterdir()) == sorted([path.name, out.name])


@pytest.mark.parametrize("alias", ["same", "symlink", "hardlink"])
def test_output_never_overwrites_input_including_links(tmp_path, capsys, alias):
    path = write_dump(tmp_path, "LEG 0 0 0 1 0 0 []")
    original = path.read_bytes()
    out = tmp_path / "alias.csv"
    if alias == "same":
        out = path
    elif alias == "symlink":
        out.symlink_to(path)
    else:
        os.link(path, out)
    assert main([str(path), "--out", str(out), "--force"]) == 1
    assert path.read_bytes() == original
    assert "input" in capsys.readouterr().err


def test_help_explains_units_examples_and_aligned_geometry(capsys):
    assert main(["--help"]) == 0
    text = capsys.readouterr().out
    assert "5%" in text and "adjustment" in text
    assert "python -m survex_duplicates" in text
    assert "metres" in text and "--exclude-splays" in text


@pytest.mark.parametrize("failure", ["fsync", "replace", "link"])
def test_failed_save_preserves_old_report_and_removes_temporary_file(
    tmp_path, monkeypatch, capsys, failure
):
    path = write_dump(tmp_path, "LEG 0 0 0 1 0 0 []\nLEG 0 0 0 1 0 0 []")
    out = tmp_path / "out.csv"
    args = [str(path), "--out", str(out)]
    if failure != "link":
        out.write_bytes(b"previous")
        args.append("--force")

    def fail(*args):
        raise OSError("injected disk failure")

    monkeypatch.setattr(os, failure, fail)
    assert main(args) == 1
    assert "injected disk failure" in capsys.readouterr().err
    if failure != "link":
        assert out.read_bytes() == b"previous"
    else:
        assert not out.exists()
    assert not list(tmp_path.glob(".*.tmp"))


def test_default_equals_explicit_five_and_all_splays_or_empty_produce_header(tmp_path, capsys):
    path = write_dump(tmp_path, "LEG 0 0 0 1 0 0 [] SPLAY\nLEG 0 0 0 1 0 0 [] SPLAY")
    default, explicit = tmp_path / "default.csv", tmp_path / "explicit.csv"
    assert main([str(path), "--out", str(default)]) == 0
    assert main([str(path), "--out", str(explicit), "--relative-percent", "5"]) == 0
    assert default.read_bytes() == explicit.read_bytes()
    capsys.readouterr()
    assert main([str(path), "--out", str(explicit), "--force", "--exclude-splays"]) == 0
    assert read_csv(explicit) == []
    summary = json.loads(capsys.readouterr().err)
    assert (summary["n_input"], summary["n_excluded"], summary["n"], summary["K"]) == (2, 2, 0, 0)
    path.write_text("\n".join(full_dump("")))
    assert main([str(path), "--out", str(explicit), "--force"]) == 0
    assert read_csv(explicit) == []


@pytest.mark.parametrize("option", ["--relative-percent", "--epsilon"])
def test_huge_tolerance_is_finite_and_point_relative_error_is_undefined(tmp_path, capsys, option):
    path = write_dump(tmp_path, "LEG 0 0 0 0 0 0 []\nLEG 0 0 0 0 0 0 []\nLEG 1 0 0 2 0 0 []")
    out = tmp_path / "large.csv"
    assert main([str(path), "--out", str(out), option, "1e1000"]) == 0
    rows = read_csv(out)
    assert rows and all(r["relative_error_percent"] == "" for r in rows)
    assert rows[0]["a1_station_names"] == "[]" and rows[0]["a1_station_status"] == "missing_node"
    assert all("Infinity" not in r["tolerance_m"] for r in rows)
    assert len(rows) == (1 if option == "--relative-percent" else 3)


@pytest.mark.parametrize(
    "body",
    [
        "LEG 0.001 0 0 1 0 0 [] SPLAY",
        "LEG 0 0 bad 1 0 0 [] SPLAY",
        "LEG 0.001 0 0 1 0 0 [] DUPLICATE",
        "LEG 0 0 bad 1 0 0 [] DUPLICATE",
    ],
)
def test_excluded_corrupt_leg_still_invalidates_entire_input(tmp_path, capsys, body):
    path = write_dump(tmp_path, body)
    out = tmp_path / "out.csv"
    assert main([str(path), "--out", str(out), "--exclude-splays"]) == 1
    assert not out.exists()
    assert "line 7" in capsys.readouterr().err


def test_dump_input_needs_no_survex_and_auto_ignores_extension(tmp_path, capsys, monkeypatch):
    path = write_dump(tmp_path, "LEG 0 0 0 1 0 0 []")
    other = path.with_suffix(".3d")
    path.rename(other)
    monkeypatch.setenv("PATH", "")
    assert main([str(other), "--out", str(tmp_path / "out.csv")]) == 0
    assert json.loads(capsys.readouterr().err)["input_format"] == "dump"


def test_missing_dump3d_and_partial_stdout_failure_do_not_publish(tmp_path, capsys):
    path = Path(__file__).parent / "fixtures" / "survey.3d"
    out = tmp_path / "out.csv"
    assert main([str(path), "--out", str(out), "--dump3d", str(tmp_path / "missing")]) == 1
    assert not out.exists() and "dump3d" in capsys.readouterr().err
    program = tmp_path / "dump script"
    program.write_text(
        f"#!{sys.executable}\nimport sys\nprint({chr(10).join(full_dump(''))!r})\nsys.exit(3)\n"
    )
    program.chmod(0o755)
    assert main([str(path), "--out", str(out), "--dump3d", str(program)]) == 1
    assert not out.exists() and "code 3" in capsys.readouterr().err


def test_both_command_entry_points_write_identical_reports(tmp_path):
    path = write_dump(tmp_path, "LEG 0 0 0 1 0 0 []\nLEG 1 0 0 0 0 0 []")
    contents = []
    for i, command in enumerate(
        ([sys.executable, "-m", "survex_duplicates"], [sys.executable, "znajdz_duplikaty.py"])
    ):
        out = tmp_path / f"run{i}.csv"
        result = subprocess.run(
            [*command, str(path), "--out", str(out)], capture_output=True, text=True
        )
        assert result.returncode == 0 and result.stdout == ""
        contents.append(out.read_bytes())
    assert contents[0] == contents[1]
