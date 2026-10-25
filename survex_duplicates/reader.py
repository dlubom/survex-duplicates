"""Read full dump3d streams without guessing station identity."""

import os
import re
import subprocess
import tempfile
from datetime import date as calendar_date
from fractions import Fraction
from io import TextIOWrapper
from pathlib import Path
from typing import Iterable

import numpy as np

from ._decimal_syntax import DECIMAL_NUMBER_PATTERN
from .models import Leg, Point, StationNames, SurveyModel


class ReaderError(ValueError):
    """Invalid or incomplete Survex input."""


_SIGNATURE = b"Survex 3D Image File\n"
_LEG_FLAGS = {"SURFACE", "DUPLICATE", "SPLAY"}
_NODE_FLAGS = {"SURFACE", "UNDERGROUND", "ENTRANCE", "EXPORTED", "FIXED", "ANON", "WALL"}


def _number(text: str) -> Fraction:
    if not re.fullmatch(DECIMAL_NUMBER_PATTERN, text):
        raise ValueError(f"invalid number {text!r}")
    return Fraction(text)


def _point(values: list[str]) -> Point:
    coords = [_number(value) * 100 for value in values]
    if len(coords) != 3 or any(x.denominator != 1 or not -(2**31) <= x < 2**31 for x in coords):
        raise ValueError("coordinates must be on the centimetre grid and within int32 range")
    return int(coords[0]), int(coords[1]), int(coords[2])


def _named(text: str) -> tuple[str, list[str]]:
    name, suffix = text[1:].rsplit("]", 1)
    if not text.startswith("[") or (suffix and not suffix[0].isspace()):
        raise ValueError("missing [name] field")
    return name, suffix.split()


def _survey_date(text: str) -> None:
    if not re.fullmatch(r"[0-9]{4}\.[0-9]{2}\.[0-9]{2}(?:-[0-9]{4}\.[0-9]{2}\.[0-9]{2})?", text):
        raise ValueError(f"invalid survey date {text!r}")
    dates = [calendar_date.fromisoformat(part.replace(".", "-")) for part in text.split("-")]
    if len(dates) == 2 and dates[0] > dates[1]:
        raise ValueError("survey date range is reversed")


def _leg_suffix(suffix: list[str]) -> tuple[tuple[str, ...], str, str]:
    style = ""
    if suffix and suffix[0].startswith("STYLE="):
        style = suffix.pop(0)[6:]
        if not re.fullmatch(r"[A-Z][A-Z0-9_]*", style):
            raise ValueError("invalid STYLE")
    shot_date = ""
    if suffix and suffix[-1][:1].isdigit():
        shot_date = suffix.pop()
        _survey_date(shot_date)
    if set(suffix) - _LEG_FLAGS:
        raise ValueError("unknown leg metadata: " + " ".join(suffix))
    return tuple(sorted(set(suffix))), style, shot_date


def _validate_header(header: dict[str, str]) -> None:
    keys = ["TITLE", "DATE", "DATE_NUMERIC"]
    if "CS" in header:
        keys.append("CS")
    keys += ["VERSION", "SEPARATOR"]
    if list(header) != keys:
        raise ValueError("incomplete header or invalid field order")
    for key in ("TITLE", "DATE"):
        if not re.fullmatch(r'".*"', header[key]):
            raise ValueError(f"invalid header field {key}")
    if not re.fullmatch(r"-?[0-9]+", header["DATE_NUMERIC"]):
        raise ValueError("invalid DATE_NUMERIC")
    if header["VERSION"] not in {str(v) for v in range(1, 9)}:
        raise ValueError("unsupported VERSION (supported: 1–8)")
    if not re.fullmatch(r"'.'", header["SEPARATOR"]):
        raise ValueError("invalid SEPARATOR")


def _validate_non_leg(kind: str, text: str) -> None:
    if kind == "XSECT":
        parts = text.split(maxsplit=5)
        if len(parts) != 6 or any(_number(v) < -1 for v in parts[1:5]):
            raise ValueError("invalid XSECT")
        _, suffix = _named(parts[5])
        if len(suffix) > 1:
            raise ValueError("invalid XSECT metadata")
        if suffix:
            _survey_date(suffix[0])
    elif kind == "XSECT_END":
        if text != kind:
            raise ValueError("invalid XSECT_END")
    elif kind == "ERROR_INFO":
        match = re.fullmatch(
            rf"ERROR_INFO #legs ([0-9]+), len ({DECIMAL_NUMBER_PATTERN})m, E ({DECIMAL_NUMBER_PATTERN}) H ({DECIMAL_NUMBER_PATTERN}) V ({DECIMAL_NUMBER_PATTERN})",
            text,
        )
        if not match or any(_number(v) < 0 for v in match.groups()[1:]):
            raise ValueError("invalid ERROR_INFO")
    else:
        raise ValueError(f"unknown record {kind}")


def parse_dump(lines: Iterable[str]) -> SurveyModel:
    """Parse a full text dump; source_line is one-based in this stream."""
    starts: list[Point] = []
    ends: list[Point] = []
    legs: list[Leg] = []
    node_names: dict[Point, set[str]] = {}
    anonymous: set[Point] = set()
    header: dict[str, str] = {}
    in_body = False
    stopped = False
    line_number = 0
    current: Point | None = None
    geometry_kind: str | None = None
    for line_number, raw in enumerate(lines, 1):
        text = raw.strip()
        try:
            if not text:
                continue
            if text == "EXTENDED ELEVATION":
                raise ValueError("extended elevation: an ordinary spatial model is required")
            if stopped:
                raise ValueError("data after STOP (possible --rewind stream)")
            if not in_body:
                if text == "--":
                    _validate_header(header)
                    in_body = True
                else:
                    key, value = text.split(" ", 1)
                    if (
                        key not in {"TITLE", "DATE", "DATE_NUMERIC", "CS", "VERSION", "SEPARATOR"}
                        or key in header
                    ):
                        raise ValueError("unknown or repeated header field")
                    header[key] = value
                continue
            kind = text.split()[0]
            if kind == "MOVE":
                current = _point(text.split()[1:])
            elif kind in {"LEG", "LINE"}:
                if geometry_kind is not None and geometry_kind != kind:
                    raise ValueError("mixed LEG and LINE records")
                geometry_kind = kind
                count = 7 if kind == "LEG" else 4
                parts = text.split(maxsplit=count)
                if kind == "LEG":
                    start, end = _point(parts[1:4]), _point(parts[4:7])
                else:
                    if current is None:
                        raise ValueError("LINE before MOVE")
                    start, end = current, _point(parts[1:4])
                    current = end
                survey, suffix = _named(parts[count])
                flags, style, date = _leg_suffix(suffix)
                starts.append(start)
                ends.append(end)
                legs.append(Leg(len(legs) + 1, line_number, survey, flags, style, date))
            elif kind == "NODE":
                parts = text.split(maxsplit=4)
                point = _point(parts[1:4])
                name, node_flags = _named(parts[4])
                if set(node_flags) - _NODE_FLAGS:
                    raise ValueError("unknown NODE flags")
                names = node_names.setdefault(point, set())
                if name:
                    names.add(name)
                if not name or "ANON" in node_flags:
                    anonymous.add(point)
            elif kind == "STOP":
                if text != kind:
                    raise ValueError("invalid STOP")
                stopped = True
            else:
                _validate_non_leg(kind, text)
        except (ValueError, IndexError, ZeroDivisionError) as exc:
            raise ReaderError(f"line {line_number}: {exc}") from exc
    if not in_body or not stopped:
        raise ReaderError(f"line {line_number + 1}: truncated dump: missing header or STOP")
    stations = {
        point: StationNames(
            tuple(sorted(names)), point in anonymous, "named" if names else "unnamed"
        )
        for point, names in node_names.items()
    }
    return SurveyModel(
        np.array(starts, dtype=np.int64).reshape(-1, 3),
        np.array(ends, dtype=np.int64).reshape(-1, 3),
        tuple(legs),
        header,
        stations=stations,
    )


def read_model(path: str | Path, input_format: str = "auto", dump3d: str = "dump3d") -> SurveyModel:
    """Read a model file, preserving the order of all leg records."""
    path = Path(path)
    if input_format not in {"auto", "dump", "3d"}:
        raise ReaderError("input_format must be auto, dump or 3d")
    with path.open("rb") as binary:
        signature = binary.read(len(_SIGNATURE))
    if input_format == "auto":
        input_format = "3d" if signature == _SIGNATURE else "dump"
    if input_format == "dump":
        try:
            with path.open(encoding="utf-8") as stream:
                return parse_dump(stream)
        except UnicodeError as exc:
            raise ReaderError(f"dump is not valid UTF-8: {exc}") from exc
    env = os.environ.copy()
    env.pop("LC_ALL", None)
    env["LC_NUMERIC"] = "C"
    # A disk-backed stderr avoids both pipe deadlock and an unbounded RAM buffer.
    with tempfile.TemporaryFile() as errors:
        try:
            process = subprocess.Popen(
                [dump3d, "--legs", "--show-dates", str(path.resolve())],
                stdout=subprocess.PIPE,
                stderr=errors,
                env=env,
            )
        except FileNotFoundError as exc:
            raise ReaderError(
                f"dump3d not found: {dump3d}; install Survex or use --dump3d"
            ) from exc
        assert process.stdout is not None
        stream = TextIOWrapper(process.stdout, encoding="utf-8")
        try:
            model = None
            parse_error = None
            try:
                model = parse_dump(stream)
            except (ReaderError, UnicodeError) as exc:
                parse_error = exc
                # Drain a failed stream too: the child may still have a useful
                # exit diagnostic, and waiting with unread stdout could block.
                while process.stdout.read(65536):
                    pass
            returncode = process.wait()
            if returncode:
                errors.seek(0)
                diagnostic = errors.read(8192).decode("utf-8", errors="replace").strip()
                raise ReaderError(
                    f"dump3d exited with code {returncode}; required options --legs --show-dates: {diagnostic}"
                )
            if parse_error is not None:
                raise ReaderError(str(parse_error)) from parse_error
            assert model is not None
            model.input_format = "3d"
            return model
        except BaseException:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            raise
        finally:
            stream.close()
