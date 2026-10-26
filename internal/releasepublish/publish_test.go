package releasepublish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testVersion = "v1.1.0"
const testRevision = "1111111111111111111111111111111111111111"

type fixture struct {
	t        *testing.T
	root     string
	env      map[string]string
	assets   []map[string]string
	manifest map[string]any
	calls    [][]string
	remote   map[string][]byte
	draft    bool
	created  bool
	tagReads int
	notes    string
	mode     string
	stdout   bytes.Buffer
	stderr   bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), draft: true, remote: map[string][]byte{}, env: map[string]string{
		"GH_TOKEN": "offline-test-token", "GH_REPO": "owner/repo", "RELEASE_VERSION": testVersion,
		"RELEASE_REVISION": testRevision, "RUN_URL": "https://github.com/owner/repo/actions/runs/123",
	}}
	f.write("VERSION", []byte("1.1.0\n"))
	var targets []map[string]string
	for _, system := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			target := system + "_" + arch
			targets = append(targets, map[string]string{"id": target, "archive": "zip"})
			name := "survex-duplicates_" + testVersion + "_" + target + ".zip"
			data := []byte("Previously verified ZIP for " + target)
			digest := sum(data)
			f.write("dist/"+name, data)
			f.write("dist/"+name+".sha256", []byte(digest+"  "+name+"\n"))
			f.assets = append(f.assets, map[string]string{"name": name, "sha256": digest, "target": target})
		}
	}
	f.writeJSON("release/targets.json", map[string]any{"schema_version": 1, "targets": targets})
	f.write("docs/releases/1.1.0.md", []byte("# Release v1.1.0\n\nReviewed release notes.\n"))
	var checksums strings.Builder
	for _, a := range f.assets {
		fmt.Fprintf(&checksums, "%s  %s\n", a["sha256"], a["name"])
	}
	f.write("dist/SHA256SUMS", []byte(checksums.String()))
	f.manifest = map[string]any{"schema_version": 1, "version": testVersion, "revision": testRevision, "snapshot": false, "assets": f.assets}
	f.saveManifest()
	return f
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (f *fixture) write(name string, data []byte) {
	f.t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) read(name string) []byte {
	f.t.Helper()
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(f.root, filepath.FromSlash(name))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}
func (f *fixture) writeJSON(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(name, b)
}
func (f *fixture) saveManifest() { f.writeJSON("dist/release-manifest.json", f.manifest) }
func (f *fixture) evidence() map[string]any {
	f.t.Helper()
	var d map[string]any
	if err := json.Unmarshal(f.read("build/release-publication.json"), &d); err != nil {
		f.t.Fatal(err)
	}
	return d
}
func option(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func contains(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}
func (f *fixture) called(command string) bool {
	for _, args := range f.calls {
		if len(args) > 1 && args[0] == "release" && args[1] == command {
			return true
		}
	}
	return false
}

// The fake implements the external gh boundary, including independent remote
// bytes and downloaded files. No process or network access occurs in these tests.
func (f *fixture) gh(args ...string) (string, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	fail := func() (string, error) { return "", errors.New("simulated GitHub failure") }
	if args[0] == "api" {
		if strings.Contains(args[1], "/commits/") {
			f.tagReads++
			if f.mode == "wrong_tag" || (f.mode == "tag_moved" && f.tagReads >= 2) || (f.mode == "postpublish_moved" && !f.draft) {
				return strings.Repeat("2", 40) + "\n", nil
			}
			return testRevision + "\n", nil
		}
		if strings.Contains(args[1], "/releases?") {
			if f.mode == "lookup_fail" {
				return fail()
			}
			if f.mode == "existing" {
				return testVersion + "\n", nil
			}
			return "", nil
		}
	}
	if args[0] != "release" {
		return fail()
	}
	switch args[1] {
	case "create":
		if f.mode == "create_fail" {
			return fail()
		}
		if !contains(args, "--draft") || !contains(args, "--verify-tag") || option(args, "--target") != testRevision {
			f.t.Fatalf("unsafe creation: %v", args)
		}
		f.notes = string(f.read(option(args, "--notes-file")))
		f.created = true
	case "upload":
		if f.mode == "upload_fail" {
			return fail()
		}
		for _, name := range args[3:] {
			if strings.HasPrefix(name, "--") {
				break
			}
			f.remote[filepath.Base(name)] = f.read(name)
		}
		if f.mode == "extra_remote" {
			f.remote["unexpected.txt"] = []byte("extra")
		}
	case "view":
		var names []map[string]string
		for name := range f.remote {
			names = append(names, map[string]string{"name": name})
		}
		if f.mode == "duplicate_remote" && len(names) > 0 {
			names = append(names, names[0])
		}
		value := map[string]any{"isDraft": f.draft, "tagName": testVersion, "targetCommitish": testRevision, "assets": names}
		if f.mode == "missing_draft_state" {
			delete(value, "isDraft")
		}
		b, err := json.Marshal(value)
		return string(b), err
	case "download":
		if f.mode == "download_fail" {
			return fail()
		}
		dir := option(args, "--dir")
		var names []string
		for name := range f.remote {
			names = append(names, name)
		}
		sort.Strings(names)
		for i, name := range names {
			if f.mode == "missing" && i == 0 {
				continue
			}
			f.write(filepath.ToSlash(filepath.Join(dir, name)), f.remote[name])
		}
		switch f.mode {
		case "altered":
			for _, name := range names {
				if strings.HasSuffix(name, ".zip") {
					f.write(filepath.ToSlash(filepath.Join(dir, name)), []byte("corrupted"))
					break
				}
			}
		case "altered_checksums":
			f.write(filepath.ToSlash(filepath.Join(dir, "SHA256SUMS")), []byte("corrupted"))
		case "altered_manifest":
			f.write(filepath.ToSlash(filepath.Join(dir, "release-manifest.json")), []byte("{}"))
		case "extra_download":
			f.write(filepath.ToSlash(filepath.Join(dir, "unexpected.txt")), []byte("extra"))
		}
	case "edit":
		if !contains(args, "--draft=false") || !contains(args, "--latest") {
			f.t.Fatalf("unexpected publication: %v", args)
		}
		if f.mode == "publish_fail" {
			return fail()
		}
		f.draft = false
	default:
		return fail()
	}
	return "", nil
}
func (f *fixture) run() int {
	return Run(Config{Root: f.root, Getenv: func(k string) string { return f.env[k] }, Command: f.gh}, &f.stdout, &f.stderr)
}

func TestVerifiedReadbackPublishes(t *testing.T) {
	f := newFixture(t)
	if code := f.run(); code != 0 {
		t.Fatalf("status %d: %s", code, &f.stderr)
	}
	if !f.called("edit") || f.draft || f.tagReads != 3 {
		t.Fatalf("publication state: %+v", f.calls)
	}
	e := f.evidence()
	if e["schema_version"] != float64(2) || e["publication_state"] != "confirmed" || e["publication_attempted"] != true || e["draft_created"] != true || e["published"] != true || e["phase"] != "verified" || e["revision"] != testRevision || e["version"] != testVersion || len(e["assets"].([]any)) != 14 {
		t.Fatal(e)
	}
	notesHash := sum(f.read("docs/releases/1.1.0.md"))
	if e["notes_source_sha256"] != notesHash || !strings.Contains(f.notes, notesHash) || !strings.Contains(f.notes, testRevision) {
		t.Fatalf("missing note identity: %s", f.notes)
	}
	readback := filepath.Join(f.root, filepath.FromSlash(e["readback_directory"].(string)))
	entries, err := os.ReadDir(readback)
	if err != nil || len(entries) != 14 {
		t.Fatalf("readback: %d entries, %v", len(entries), err)
	}
}

func TestRemoteFailuresNeverPublish(t *testing.T) {
	for _, mode := range []string{"altered", "altered_checksums", "altered_manifest", "missing", "extra_remote", "extra_download", "tag_moved", "wrong_tag", "lookup_fail", "existing", "create_fail", "upload_fail", "download_fail", "duplicate_remote", "missing_draft_state"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.mode = mode
			if f.run() == 0 || f.called("edit") {
				t.Fatalf("unexpected publication: %v, %s", f.calls, &f.stdout)
			}
			e := f.evidence()
			if e["published"] != false || e["publication_state"] != "not_attempted" || e["publication_attempted"] != false || e["draft_created"] != f.created || e["version"] != testVersion || e["revision"] != testRevision || e["run_url"] != f.env["RUN_URL"] || e["phase"] == "verified" {
				t.Fatal(e)
			}
		})
	}
}

func TestInvalidLocalAssetsNeverContactGitHub(t *testing.T) {
	for _, mode := range []string{"missing", "extra", "corrupt", "snapshot", "revision", "duplicate", "checksum", "symlink", "missing_snapshot", "notes_missing", "notes_empty"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			name := f.assets[0]["name"]
			archive := filepath.Join(f.root, "dist", name)
			switch mode {
			case "missing":
				if err := os.Remove(archive); err != nil {
					t.Fatal(err)
				}
			case "extra":
				f.write("dist/unexpected.txt", []byte("extra"))
			case "corrupt":
				f.write("dist/"+name, []byte("corrupt"))
			case "snapshot":
				f.manifest["snapshot"] = true
			case "revision":
				f.manifest["revision"] = strings.Repeat("2", 40)
			case "duplicate":
				f.assets[1] = f.assets[0]
			case "checksum":
				f.write("dist/SHA256SUMS", []byte("invalid"))
			case "symlink":
				copy := filepath.Join(f.root, "copy.zip")
				if err := os.Rename(archive, copy); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(copy, archive); err != nil {
					t.Skipf("symlink creation unavailable: %v", err)
				}
			case "missing_snapshot":
				delete(f.manifest, "snapshot")
			case "notes_missing":
				if err := os.Remove(filepath.Join(f.root, "docs/releases/1.1.0.md")); err != nil {
					t.Fatal(err)
				}
			case "notes_empty":
				f.write("docs/releases/1.1.0.md", []byte(" \n"))
			}
			f.saveManifest()
			if f.run() == 0 || len(f.calls) != 0 {
				t.Fatalf("invalid local state contacted GitHub: %v", f.calls)
			}
		})
	}
}

func TestPublicationFailureReportsUncertainty(t *testing.T) {
	f := newFixture(t)
	f.mode = "publish_fail"
	if f.run() == 0 || !f.draft || !strings.Contains(f.stderr.String(), "attempted but not confirmed") {
		t.Fatalf("%s", &f.stderr)
	}
	e := f.evidence()
	if e["phase"] != "publish_draft" || e["published"] != nil || e["publication_state"] != "unknown" || e["publication_attempted"] != true {
		t.Fatal(e)
	}
}
func TestPostpublicationTagChangeIsReported(t *testing.T) {
	f := newFixture(t)
	f.mode = "postpublish_moved"
	if f.run() == 0 || !strings.Contains(f.stderr.String(), "made public") {
		t.Fatalf("%s", &f.stderr)
	}
	e := f.evidence()
	if e["published"] != true || e["phase"] != "verify_published_release" {
		t.Fatal(e)
	}
}
func TestInvalidEnvironmentNeverContactsGitHub(t *testing.T) {
	for _, key := range []string{"GH_TOKEN", "GH_REPO", "RELEASE_VERSION", "RELEASE_REVISION", "RUN_URL"} {
		t.Run(key, func(t *testing.T) {
			f := newFixture(t)
			delete(f.env, key)
			if f.run() == 0 || len(f.calls) != 0 {
				t.Fatal(f.calls)
			}
			if f.evidence()["phase"] != "validate_environment" {
				t.Fatal(f.evidence())
			}
		})
	}
}

func TestAcceptedPublicationWithLostResponseRecordsUnknown(t *testing.T) {
	f := newFixture(t)
	command := func(args ...string) (string, error) {
		output, err := f.gh(args...)
		if len(args) > 1 && args[0] == "release" && args[1] == "edit" && err == nil {
			return "", errors.New("response lost after server accepted publication")
		}
		return output, err
	}
	code := Run(Config{Root: f.root, Getenv: func(k string) string { return f.env[k] }, Command: command}, &f.stdout, &f.stderr)
	e := f.evidence()
	if code != 1 || f.draft || e["published"] != nil || e["publication_state"] != "unknown" || e["publication_attempted"] != true || e["draft_created"] != true || e["version"] != testVersion || e["revision"] != testRevision || e["run_url"] != f.env["RUN_URL"] || !strings.Contains(f.stderr.String(), "attempted but not confirmed") {
		t.Fatalf("remote draft=%v evidence=%v stderr=%s", f.draft, e, &f.stderr)
	}
	edits := 0
	for _, args := range f.calls {
		if len(args) > 1 && args[0] == "release" && args[1] == "edit" {
			edits++
		}
	}
	if edits != 1 {
		t.Fatalf("publication retried %d times", edits)
	}
}
