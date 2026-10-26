package releasepack

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests catch missing target validation and unsafe archive acceptance.
func TestTargetCoverage(t *testing.T) {
	c := Config{SchemaVersion: 1, Targets: []Target{{ID: "linux_amd64", GOOS: "linux", GOARCH: "amd64", Env: map[string]string{"GOAMD64": "v1"}, Kind: "native", Archive: "zip", Runner: "ubuntu-24.04"}}}
	dist := []DistTarget{{GOOS: "linux", GOARCH: "amd64"}}
	if err := c.Validate(dist); err != nil {
		t.Fatal(err)
	}
	c.Targets[0].Env["GOAMD64"] = "v3"
	if err := c.Validate(dist); err == nil {
		t.Fatal("accepted unreviewed CPU setting")
	}
	c.Targets[0].Env["GOAMD64"] = "v1"
	dist = []DistTarget{{GOOS: "linux", GOARCH: "amd64", Broken: true}}
	if err := c.Validate(dist); err == nil {
		t.Fatal("accepted unsupported port")
	}
}

func TestArchiveDeterminismAndSafety(t *testing.T) {
	dir := t.TempDir()
	for _, format := range []string{"zip"} {
		a, b := filepath.Join(dir, "one."+format), filepath.Join(dir, "two."+format)
		entries := []Entry{{Name: "b.txt", Data: []byte("beta"), Mode: 0644}, {Name: "a.txt", Data: []byte("alpha"), Mode: 0644}}
		if err := WriteArchive(a, format, 1700000000, entries); err != nil {
			t.Fatal(err)
		}
		if err := WriteArchive(b, format, 1700000000, entries); err != nil {
			t.Fatal(err)
		}
		x, _ := os.ReadFile(a)
		y, _ := os.ReadFile(b)
		if !bytes.Equal(x, y) {
			t.Fatal("archive content is nondeterministic")
		}
		z, err := zip.OpenReader(a)
		if err != nil {
			t.Fatal(err)
		}
		if len(z.File) != 2 || z.File[0].Name != "a.txt" || z.File[1].Name != "b.txt" {
			t.Fatal("archive members are not sorted")
		}
		for _, f := range z.File {
			if f.Modified.Unix() != 1700000000 || f.Mode().Perm() != 0644 {
				t.Fatal("archive metadata is not normalized")
			}
		}
		z.Close()
		entries[0].Name = "../escape"
		if err := WriteArchive(a, format, 1700000000, entries); err == nil {
			t.Fatal("accepted unsafe member")
		}
	}
	for _, names := range [][]string{{".."}, {"../escape"}, {"same", "same"}, {"/absolute"}, {"C:/drive"}} {
		name := filepath.Join(dir, "bad.zip")
		f, _ := os.Create(name)
		z := zip.NewWriter(f)
		for _, n := range names {
			h := &zip.FileHeader{Name: n}
			h.SetMode(0644)
			w, _ := z.CreateHeader(h)
			w.Write([]byte("test"))
		}
		z.Close()
		f.Close()
		if _, err := ReadArchive(name, nil); err == nil {
			t.Fatalf("accepted unsafe/duplicate archive %v", names)
		}
	}
}

func TestArchiveDoesNotReplaceExistingOutput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "existing.zip")
	if err := os.WriteFile(p, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteArchive(p, "zip", 1700000000, []Entry{{Name: "member", Data: []byte("new"), Mode: 0644}}); err == nil {
		t.Fatal("replaced existing output")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "keep" {
		t.Fatal("existing output changed")
	}
}

// Missing and surplus files must block assembly before any metadata is written.
func TestAssemblyRequiresCompleteAssetSet(t *testing.T) {
	dir := t.TempDir()
	expected := map[string]Target{"release.zip": {}, "release.zip.sha256": {}}
	if err := checkAssetNames(dir, expected); err == nil {
		t.Fatal("accepted no release assets")
	}
	for n := range expected {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkAssetNames(dir, expected); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unreviewed.zip"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkAssetNames(dir, expected); err == nil {
		t.Fatal("accepted surplus release asset")
	}
}

// A checkout and an exact release tag are different identities; untracked input
// must invalidate release mode and appear in a snapshot's source fingerprints.
func TestSourceIdentityRequiresCleanExactTag(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	run("init", "-q")
	for n, s := range map[string]string{"VERSION": "1.1.0\n", ".go-version": "1.26.3\n", "main.go": "package main\n"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("-c", "user.name=Release Test", "-c", "user.email=release@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	if _, err := SourceManifest(dir, "v1.1.0", false); err == nil {
		t.Fatal("accepted untagged source as a release")
	}
	run("tag", "v1.1.0")
	m, err := SourceManifest(dir, "v1.1.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Dirty || m.Snapshot || m.Revision != run("rev-parse", "HEAD") {
		t.Fatal("wrong clean release identity")
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.go"), []byte("untracked source"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SourceManifest(dir, "v1.1.0", false); err == nil {
		t.Fatal("accepted untracked source in tagged release")
	}
	m, err = SourceManifest(dir, "v1.1.0-test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Dirty || !m.Snapshot || !strings.HasSuffix(m.Revision, "-dirty") || !validDigest(m.Source["extra.go"]) {
		t.Fatal("snapshot hid uncommitted source")
	}
	if err := verifySourceIdentity(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.go"), []byte("changed source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifySourceIdentity(dir, m); err == nil {
		t.Fatal("accepted different source bytes with same revision")
	}
}

func TestBuildEnvironmentRemovesCaseInsensitiveOverrides(t *testing.T) {
	t.Setenv("goarch", "incorrect")
	t.Setenv("GoFlags", "-race")
	t.Setenv("GoArM64", "v9.5")
	target := Target{GOOS: "linux", GOARCH: "arm64", Env: map[string]string{"GOARM64": "v8.0"}}
	seen := map[string]string{}
	for _, s := range buildEnv(target) {
		k, v, _ := strings.Cut(s, "=")
		upper := strings.ToUpper(k)
		if _, ok := seen[upper]; ok && (upper == "GOARCH" || upper == "GOFLAGS" || upper == "GOARM64") {
			t.Fatalf("duplicate case-insensitive override %s", k)
		}
		seen[upper] = v
	}
	if seen["GOARCH"] != "arm64" || seen["GOFLAGS"] != "" || seen["GOARM64"] != "v8.0" {
		t.Fatal("environment changed pinned build settings")
	}
}

func TestTrustedBundleRejectsSelfConsistentTampering(t *testing.T) {
	entries := []Entry{{Name: "README.md", Data: []byte("alpha"), Mode: 0644}}
	records := map[string]FileRecord{"README.md": {SHA256: "8ed3f6ad685b959ead7022518e1af76cd816f8e8ec7ccdda1ed4018e8f2223f8", Mode: 0644, Size: 5}}
	if err := verifyExpectedEntries(entries, records); err != nil {
		t.Fatal(err)
	}
	r := records["README.md"]
	r.SHA256 = strings.Repeat("0", 64)
	records["README.md"] = r
	if err := verifyExpectedEntries(entries, records); err == nil {
		t.Fatal("accepted archive manifest updated together with tampered README")
	}
}

func TestManifestRequiresExactInventory(t *testing.T) {
	m := Manifest{Files: map[string]FileRecord{"a": {SHA256: "expected", Mode: 0644, Size: 1}}}
	if err := verifyInventory(m, map[string]FileRecord{}); err == nil {
		t.Fatal("accepted missing member")
	}
	if err := verifyInventory(m, map[string]FileRecord{"a": {SHA256: "wrong", Mode: 0644, Size: 1}}); err == nil {
		t.Fatal("accepted tampered member")
	}
	if err := verifyInventory(m, map[string]FileRecord{"a": {SHA256: "expected", Mode: 0644, Size: 1}, "extra": {}}); err == nil {
		t.Fatal("accepted extra member")
	}
}

func TestBundledProvenance(t *testing.T) {
	root := filepath.Join("..", "..")
	entries, err := bundleEntries(root, Target{ID: "linux_amd64", GOOS: "linux", GOARCH: "amd64", Kind: "native"}, "v1.1.0-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if _, err := entryRecord(entry); err != nil {
			t.Fatalf("required bundle member %s is unreadable: %v", entry.Name, err)
		}
	}
	if err := verifyDemoEntries(entries); err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if entries[i].Name == "examples/kataster/KATASTER.legs.dump" {
			entries[i].Path = ""
			entries[i].Data = []byte("corrupt")
		}
	}
	if err := verifyDemoEntries(entries); err == nil {
		t.Fatal("accepted corrupted survey snapshot")
	}
}
