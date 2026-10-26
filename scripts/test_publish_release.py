"""Offline publication tests. The gh stub never contacts GitHub."""

import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("publish-release.sh").resolve()
VERSION = "v1.1.0"
REVISION = "1" * 40
TARGETS = [
    f"{system}_{arch}" for system in ("darwin", "linux", "windows") for arch in ("amd64", "arm64")
]

GH_STUB = r"""#!PYTHON
import json, os, pathlib, shutil, sys
args = sys.argv[1:]
root = pathlib.Path(os.environ["STUB_ROOT"])
state_path = root / "state.json"
state = json.loads(state_path.read_text()) if state_path.exists() else {"draft": True, "tag_reads": 0}
mode = os.environ.get("STUB_MODE", "ok")
with (root / "calls.jsonl").open("a") as log:
    log.write(json.dumps(args) + "\n")
remote = root / "remote"
remote.mkdir(exist_ok=True)
def save(): state_path.write_text(json.dumps(state))
def fail(): save(); print("simulated GitHub failure", file=sys.stderr); sys.exit(1)
if args[0] == "api":
    if "/commits/" in args[1]:
        state["tag_reads"] += 1
        save()
        moved = mode == "wrong_tag" or (mode == "tag_moved" and state["tag_reads"] >= 2) or (mode == "postpublish_moved" and not state["draft"])
        print("2" * 40 if moved else "1" * 40)
    elif "/releases?" in args[1]:
        if mode == "lookup_fail": fail()
        if mode == "existing": print("v1.1.0")
    else: fail()
elif args[:2] == ["release", "create"]:
    if mode == "create_fail": fail()
    assert "--draft" in args and "--verify-tag" in args
    state["created"] = True
    state["revision"] = args[args.index("--target") + 1]
    state["notes"] = pathlib.Path(args[args.index("--notes-file") + 1]).read_text()
    save()
elif args[:2] == ["release", "upload"]:
    if mode == "upload_fail": fail()
    for name in args[3:]:
        if name.startswith("--"): break
        shutil.copyfile(name, remote / pathlib.Path(name).name)
    if mode == "extra_remote": (remote / "unexpected.txt").write_text("extra")
elif args[:2] == ["release", "view"]:
    print(json.dumps({"isDraft": state["draft"], "tagName": "v1.1.0", "targetCommitish": state["revision"], "assets": [{"name": p.name} for p in remote.iterdir()]}))
elif args[:2] == ["release", "download"]:
    if mode == "download_fail": fail()
    dest = pathlib.Path(args[args.index("--dir") + 1])
    files = sorted(remote.iterdir())
    for path in files:
        if mode == "missing" and path == files[0]: continue
        shutil.copyfile(path, dest / path.name)
    if mode == "altered": (dest / next(p.name for p in files if p.suffix == ".zip")).write_text("corrupted")
    if mode == "altered_checksums": (dest / "SHA256SUMS").write_text("corrupted")
    if mode == "altered_manifest": (dest / "release-manifest.json").write_text("{}")
    if mode == "extra_download": (dest / "unexpected.txt").write_text("extra")
elif args[:2] == ["release", "edit"]:
    assert "--draft=false" in args and "--latest" in args
    if mode == "publish_fail": fail()
    state["draft"] = False
    save()
else: fail()
"""


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="survex-publish-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "bin").mkdir()
        stub = self.root / "bin" / "gh"
        stub.write_text(GH_STUB.replace("#!PYTHON", "#!" + sys.executable))
        stub.chmod(0o755)
        (self.root / "VERSION").write_text("1.1.0\n")
        (self.root / "release").mkdir()
        (self.root / "release" / "targets.json").write_text(
            json.dumps(
                {"schema_version": 1, "targets": [{"id": t, "archive": "zip"} for t in TARGETS]}
            )
        )
        notes = self.root / "docs" / "releases"
        notes.mkdir(parents=True)
        (notes / "1.1.0.md").write_text("# Release v1.1.0\n\nReviewed release notes.\n")
        dist = self.root / "dist"
        dist.mkdir()
        assets = []
        for target in TARGETS:
            name = f"survex-duplicates_{VERSION}_{target}.zip"
            data = f"Previously verified ZIP for {target}".encode()
            (dist / name).write_bytes(data)
            digest = hashlib.sha256(data).hexdigest()
            (dist / (name + ".sha256")).write_text(f"{digest}  {name}\n")
            assets.append({"name": name, "sha256": digest, "target": target})
        assets.sort(key=lambda a: a["name"])
        (dist / "SHA256SUMS").write_text("".join(f"{a['sha256']}  {a['name']}\n" for a in assets))
        self.manifest = {
            "schema_version": 1,
            "version": VERSION,
            "revision": REVISION,
            "snapshot": False,
            "assets": assets,
        }
        self.write_manifest()

    def write_manifest(self):
        (self.root / "dist" / "release-manifest.json").write_text(json.dumps(self.manifest))

    def run_publish(self, mode="ok"):
        env = dict(
            os.environ,
            PATH=str(self.root / "bin") + os.pathsep + os.environ["PATH"],
            GH_TOKEN="offline-test-token",
            GH_REPO="owner/repo",
            RELEASE_VERSION=VERSION,
            RELEASE_REVISION=REVISION,
            RUN_URL="https://github.com/owner/repo/actions/runs/123",
            STUB_ROOT=str(self.root),
            STUB_MODE=mode,
        )
        result = subprocess.run(
            ["bash", str(SCRIPT)],
            cwd=self.root,
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )
        log = self.root / "calls.jsonl"
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        return result, calls

    def test_verified_readback_publishes(self):
        result, calls = self.run_publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(any(call[:2] == ["release", "edit"] for call in calls))
        state = json.loads((self.root / "state.json").read_text())
        self.assertFalse(state["draft"])
        self.assertEqual(state["tag_reads"], 3)
        readbacks = list((self.root / "build").glob("release-readback-*"))
        self.assertEqual(len(readbacks), 1)
        self.assertEqual(len(list(readbacks[0].iterdir())), 14)
        evidence = json.loads((self.root / "build/release-publication.json").read_text())
        self.assertTrue(evidence["published"])
        self.assertEqual(evidence["revision"], REVISION)
        self.assertEqual(evidence["version"], VERSION)
        self.assertEqual(len(evidence["assets"]), 14)
        self.assertEqual(evidence["phase"], "verified")
        notes_hash = hashlib.sha256((self.root / "docs/releases/1.1.0.md").read_bytes()).hexdigest()
        self.assertEqual(evidence["notes_source_sha256"], notes_hash)
        self.assertIn(notes_hash, state["notes"])

    def test_remote_failures_never_publish(self):
        for mode in (
            "altered",
            "altered_checksums",
            "altered_manifest",
            "missing",
            "extra_remote",
            "extra_download",
            "tag_moved",
            "wrong_tag",
            "lookup_fail",
            "existing",
            "create_fail",
            "upload_fail",
            "download_fail",
        ):
            with self.subTest(mode=mode):
                case = PublicationTests()
                case.setUp()
                try:
                    result, calls = case.run_publish(mode)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertFalse(any(call[:2] == ["release", "edit"] for call in calls), calls)
                    evidence = json.loads(
                        (case.root / "build/release-publication.json").read_text()
                    )
                    self.assertEqual(set(evidence), {"phase", "published"})
                    self.assertFalse(evidence["published"])
                finally:
                    case.doCleanups()

    def test_invalid_local_assets_never_contact_github(self):
        for mode in (
            "missing",
            "extra",
            "corrupt",
            "snapshot",
            "revision",
            "duplicate",
            "checksum",
            "symlink",
        ):
            with self.subTest(mode=mode):
                case = PublicationTests()
                case.setUp()
                try:
                    dist = case.root / "dist"
                    archive = dist / case.manifest["assets"][0]["name"]
                    if mode == "missing":
                        archive.unlink()
                    elif mode == "extra":
                        (dist / "unexpected.txt").write_text("extra")
                    elif mode == "corrupt":
                        archive.write_text("corrupt")
                    elif mode == "snapshot":
                        case.manifest["snapshot"] = True
                    elif mode == "revision":
                        case.manifest["revision"] = "2" * 40
                    elif mode == "duplicate":
                        case.manifest["assets"][1] = case.manifest["assets"][0]
                    elif mode == "checksum":
                        (dist / "SHA256SUMS").write_text("invalid")
                    elif mode == "symlink":
                        copy = case.root / "copy.zip"
                        archive.rename(copy)
                        archive.symlink_to(copy)
                    case.write_manifest()
                    result, calls = case.run_publish()
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertEqual(calls, [])
                finally:
                    case.doCleanups()

    def test_publication_failure_is_reported(self):
        result, _ = self.run_publish("publish_fail")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(json.loads((self.root / "state.json").read_text())["draft"])

    def test_postpublication_tag_change_is_reported(self):
        result, _ = self.run_publish("postpublish_moved")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("made public", result.stderr)


if __name__ == "__main__":
    unittest.main()
