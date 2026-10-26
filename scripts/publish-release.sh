#!/usr/bin/env bash
# Publish only assembled assets after independent GitHub download verification.
set -euo pipefail
exec python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


class PublicationError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise PublicationError(message)


def read_json(path):
    return json.loads(path.read_text(encoding="utf-8"))


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def file_set(directory):
    entries = list(directory.iterdir())
    require(all(p.is_file() and not p.is_symlink() for p in entries),
            f"Non-regular release asset in {directory}")
    return {p.name for p in entries}


def gh(*args):
    result = subprocess.run(["gh", *args], text=True, capture_output=True, check=False)
    require(result.returncode == 0, f"GitHub command failed: gh {' '.join(args)}\n{result.stderr.strip()}")
    return result.stdout


def check_tag():
    actual = gh("api", f"repos/{repository}/commits/{version}", "--jq", ".sha").strip()
    require(actual == revision, f"Remote tag {version} does not identify {revision}")


def check_release(draft, expected_files):
    remote = json.loads(gh("release", "view", version, "--repo", repository,
                           "--json", "isDraft,tagName,targetCommitish,assets"))
    require(remote["isDraft"] is draft and remote["tagName"] == version and
            remote["targetCommitish"] == revision, "Remote release identity or draft state differs")
    names = [asset["name"] for asset in remote["assets"]]
    require(len(names) == len(set(names)) and set(names) == expected_files,
            "Remote release has missing, duplicate or unexpected assets")


def validate_local_assets():
    require(Path("VERSION").read_text(encoding="utf-8").strip() == version[1:],
            "Release version does not match VERSION")
    config = read_json(Path("release/targets.json"))
    target_ids = [target["id"] for target in config["targets"]]
    expected_targets = {f"{system}_{arch}" for system in ("darwin", "linux", "windows")
                        for arch in ("amd64", "arm64")}
    require(config["schema_version"] == 1 and len(target_ids) == 6 and
            set(target_ids) == expected_targets and
            all(target["archive"] == "zip" for target in config["targets"]),
            "Expected exactly six approved native ZIP targets")
    manifest = read_json(Path("dist/release-manifest.json"))
    require(manifest["schema_version"] == 1 and manifest["version"] == version and
            manifest["revision"] == revision and manifest["snapshot"] is False,
            "Release manifest identity differs or is a snapshot")
    assets = manifest["assets"]
    require(len(assets) == 6 and {a["target"] for a in assets} == expected_targets,
            "Release manifest does not contain exactly six target archives")
    expected_files = {"SHA256SUMS", "release-manifest.json"}
    checksum_lines = []
    for asset in sorted(assets, key=lambda a: a["name"]):
        name = f"survex-duplicates_{version}_{asset['target']}.zip"
        require(asset["name"] == name and re.fullmatch(r"[0-9a-f]{64}", asset["sha256"]),
                "Invalid archive name or SHA-256 in release manifest")
        require(name not in expected_files, "Duplicate release archive")
        expected_files.update((name, name + ".sha256"))
        archive = Path("dist") / name
        require(archive.is_file() and not archive.is_symlink() and digest(archive) == asset["sha256"],
                f"Archive checksum mismatch: {name}")
        line = f"{asset['sha256']}  {name}\n".encode("ascii")
        require(Path(str(archive) + ".sha256").read_bytes() == line,
                f"Invalid archive checksum sidecar: {name}")
        checksum_lines.append(line)
    require(file_set(Path("dist")) == expected_files, "Missing or unexpected local release assets")
    require(Path("dist/SHA256SUMS").read_bytes() == b"".join(checksum_lines), "Invalid SHA256SUMS")
    return {name: digest(Path("dist") / name) for name in sorted(expected_files)}


created = False
published = False
publication_attempted = False
phase = "validate_environment"
evidence = None
try:
    required = ("GH_TOKEN", "GH_REPO", "RELEASE_VERSION", "RELEASE_REVISION", "RUN_URL")
    require(all(os.environ.get(name) for name in required), "Missing required publication environment variable")
    repository = os.environ["GH_REPO"]
    version = os.environ["RELEASE_VERSION"]
    revision = os.environ["RELEASE_REVISION"]
    run_url = os.environ["RUN_URL"]
    require(re.fullmatch(r"[A-Za-z0-9_-]+/[A-Za-z0-9_.-]+", repository), "Invalid GitHub repository")
    require(re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", version), "Expected a stable vMAJOR.MINOR.PATCH release")
    require(re.fullmatch(r"[0-9a-f]{40}", revision), "Expected a complete clean Git revision")
    require(re.fullmatch(r"https://github\.com/" + re.escape(repository) + r"/actions/runs/[0-9]+(?:/attempts/[0-9]+)?", run_url),
            "Invalid validation workflow URL")
    phase = "validate_local_assets"
    expected_hashes = validate_local_assets()
    notes = Path("docs/releases") / f"{version[1:]}.md"
    require(notes.is_file() and not notes.is_symlink(), "Reviewed version-specific release notes are missing")
    notes_text = notes.read_text(encoding="utf-8").strip()
    require(bool(notes_text), "Release notes are empty")
    notes_hash = digest(notes)
    phase = "check_remote_tag"
    check_tag()
    phase = "check_existing_release"
    existing = gh("api", f"repos/{repository}/releases?per_page=100", "--paginate", "--jq", ".[].tag_name").splitlines()
    require(version not in existing, "Release already exists; refusing to overwrite or resume it")
    Path("build").mkdir(exist_ok=True)
    readback = Path(tempfile.mkdtemp(prefix="release-readback-", dir="build"))
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", suffix=".md", prefix="release-notes-", dir="build") as rendered:
        rendered.write(notes_text + f"\n\nSource revision: `{revision}`.\n"
                       f"Release notes source SHA-256: `{notes_hash}`.\n\n[Validation workflow]({run_url})\n")
        rendered.flush()
        phase = "create_draft"
        gh("release", "create", version, "--repo", repository, "--verify-tag", "--draft",
           "--title", f"Survex Duplicate Detector {version}", "--target", revision, "--notes-file", rendered.name)
    created = True
    phase = "upload_assets"
    gh("release", "upload", version, *(str(Path("dist") / name) for name in expected_hashes), "--repo", repository)
    phase = "verify_remote_draft"
    check_release(True, set(expected_hashes))
    phase = "download_assets"
    gh("release", "download", version, "--repo", repository, "--dir", str(readback))
    phase = "verify_downloaded_assets"
    require(file_set(readback) == set(expected_hashes), "Downloaded release asset set differs")
    for name, expected_hash in expected_hashes.items():
        require(digest(readback / name) == expected_hash, f"Downloaded asset checksum mismatch: {name}")
    phase = "recheck_remote_tag"
    check_tag()
    phase = "publish_draft"
    publication_attempted = True
    gh("release", "edit", version, "--repo", repository, "--draft=false", "--latest")
    published = True
    phase = "verify_published_release"
    check_release(False, set(expected_hashes))
    check_tag()
    phase = "verified"
    evidence = {
        "schema_version": 1,
        "phase": phase,
        "version": version,
        "revision": revision,
        "run_url": run_url,
        "release_url": f"https://github.com/{repository}/releases/tag/{version}",
        "published": True,
        "notes_source_sha256": notes_hash,
        "readback_directory": str(readback),
        "assets": [{"name": name, "sha256": checksum} for name, checksum in expected_hashes.items()],
    }
    print(f"Published and verified https://github.com/{repository}/releases/tag/{version}")
    print(f"Independent readback retained in {readback}")
except (PublicationError, OSError, ValueError, KeyError, TypeError) as error:
    print(f"Release publication stopped: {error}", file=sys.stderr)
    if published:
        print("The release was made public, but postpublication verification failed; inspect it before further action.", file=sys.stderr)
    elif publication_attempted:
        print("Publication was attempted but not confirmed; inspect the current release state before retrying.", file=sys.stderr)
    elif created:
        print("The draft release was left for inspection; it was not made public.", file=sys.stderr)
    sys.exit(1)
finally:
    # On failure, published records confirmation. A failed network response does
    # not prove the draft remained private; the phase distinguishes that case.
    try:
        Path("build").mkdir(exist_ok=True)
        Path("build/release-publication.json").write_text(
            json.dumps(evidence or {"phase": phase, "published": published}, indent=2) + "\n", encoding="utf-8")
    except OSError:
        print("Could not save publication verification evidence.", file=sys.stderr)
        sys.exit(1)
PY
