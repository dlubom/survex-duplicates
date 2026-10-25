"""Regenerate real fixtures with installed Survex; never touches the kataster."""

import hashlib
import json
import subprocess
from pathlib import Path

HERE = Path(__file__).resolve().parent
COMMANDS = [
    ["cavern", "--no-auxiliary-files", "--output=survey.3d", "survey.svx"],
    ["cavern", "--no-auxiliary-files", "--3d-version=3", "--output=legacy.3d", "legacy.svx"],
    ["cavern", "--output=closure.3d", "legacy.svx"],
    ["extend", "survey.3d", "extended.3d"],
]


def regenerate():
    for command in COMMANDS:
        subprocess.run(command, cwd=HERE, check=True)
    dumps = {
        "survey.legs.dump": ["--legs", "--show-dates", "survey.3d"],
        "survey.lines.dump": ["--show-dates", "survey.3d"],
        "legacy.legs.dump": ["--legs", "--show-dates", "legacy.3d"],
        "closure.legs.dump": ["--legs", "--show-dates", "closure.3d"],
        "extended.legs.dump": ["--legs", "--show-dates", "extended.3d"],
    }
    for filename, args in dumps.items():
        with (HERE / filename).open("wb") as output:
            subprocess.run(["dump3d", *args], cwd=HERE, stdout=output, check=True)
    (HERE / "closure.err").unlink(missing_ok=True)
    versions = {
        tool: subprocess.check_output([tool, "--version"], text=True).strip()
        for tool in ("cavern", "dump3d", "extend")
    }
    hashes = {
        path.name: hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted(HERE.iterdir())
        if path.suffix in {".svx", ".3d", ".dump"}
    }
    (HERE / "provenance.json").write_text(
        json.dumps(
            {"versions": versions, "commands": COMMANDS, "dumps": dumps, "sha256": hashes},
            ensure_ascii=False,
            indent=2,
        )
        + "\n"
    )


if __name__ == "__main__":
    regenerate()
