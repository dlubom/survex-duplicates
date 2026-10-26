package releasepack

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The fixture is a real, frozen Git checkout. Packaging outputs live outside it,
// and no later repository edits can change its source identity during a build.
func nativePackageFixture(t *testing.T) (string, string, Manifest) {
	t.Helper()
	root := t.TempDir()
	source, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-files", "-z", "--cached")
	cmd.Dir = source
	names, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range strings.Split(string(names), "\x00") {
		if rel == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(rel)), b)
	}
	// Assembly checks the native archive first, so its checksum failures can be
	// reached without building five unrelated executables in every unit run.
	c, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	native := runtime.GOOS + "_" + runtime.GOARCH
	for i := range c.Targets {
		if c.Targets[i].ID == native {
			c.Targets[0], c.Targets[i] = c.Targets[i], c.Targets[0]
		}
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "release", "targets.json"), b)
	for _, args := range [][]string{{"init", "-q"}, {"config", "core.autocrlf", "false"}, {"add", "."}, {"-c", "user.name=Release Test", "-c", "user.email=release@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "frozen native package fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, b)
		}
	}
	versionBytes, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := Build(root, native, "v"+strings.TrimSpace(string(versionBytes))+"-test", t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Verify(root, archive)
	if err != nil {
		t.Fatalf("baseline public verification: %v", err)
	}
	return root, archive, m
}

func writeTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func archiveTestEntries(t *testing.T, archive string) []Entry {
	t.Helper()
	var entries []Entry
	_, err := ReadArchive(archive, func(name string, mode int64, r io.Reader) error {
		b, err := io.ReadAll(r)
		entries = append(entries, Entry{Name: name, Data: b, Mode: mode})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// Recompute the archive manifest after a mutation so the public guard under test
// sees the bad input, instead of every case stopping at an unrelated checksum.
func rewriteTestManifest(t *testing.T, entries []Entry, m Manifest, coherent bool) []Entry {
	t.Helper()
	if coherent {
		m.Files = map[string]FileRecord{}
		for _, e := range entries {
			if e.Name == "manifest.json" {
				continue
			}
			r, err := entryRecord(e)
			if err != nil {
				t.Fatal(err)
			}
			m.Files[e.Name] = r
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if entries[i].Name == "manifest.json" {
			entries[i].Data = b
		}
	}
	return entries
}

func TestReleasePublicBoundaries(t *testing.T) {
	root, archive, original := nativePackageFixture(t)
	tests := []struct {
		name, diagnostic string
		coherent         bool
		mutate           func(*testing.T, *Manifest, *[]Entry)
	}{
		{"missing_inventory_member", "manifest member count mismatch", false, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i, e := range *es {
				if e.Name == m.Target.Executable() {
					*es = append((*es)[:i], (*es)[i+1:]...)
					return
				}
			}
		}},
		{"manifest_digest", "manifest digest, size or mode mismatch", false, func(t *testing.T, m *Manifest, es *[]Entry) {
			r := m.Files[m.Target.Executable()]
			r.SHA256 = strings.Repeat("0", 64)
			m.Files[m.Target.Executable()] = r
		}},
		{"manifest_identity", "invalid archive identity metadata", true, func(t *testing.T, m *Manifest, es *[]Entry) { m.SchemaVersion = 9 }},
		{"manifest_mode", "invalid manifest mode", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i := range *es {
				if (*es)[i].Name == "manifest.json" {
					(*es)[i].Mode = 0755
				}
			}
		}},
		{"target_definition", "archive target differs", true, func(t *testing.T, m *Manifest, es *[]Entry) { m.Target.Runner = "unreviewed-runner" }},
		{"toolchain_pin", "archive toolchain does not match pin", true, func(t *testing.T, m *Manifest, es *[]Entry) { m.Toolchain = differentTestString(m.Toolchain) }},
		{"invalid_source_fingerprint", "invalid source fingerprint", true, func(t *testing.T, m *Manifest, es *[]Entry) { m.Source["main.go"] = "invalid" }},
		{"missing_source_fingerprint", "missing source fingerprint", true, func(t *testing.T, m *Manifest, es *[]Entry) { delete(m.Source, "main.go") }},
		{"source_hash", "archive source identity or fingerprints differ", true, func(t *testing.T, m *Manifest, es *[]Entry) { m.Source["main.go"] = strings.Repeat("0", 64) }},
		{"source_identity", "archive source identity or fingerprints differ", true, func(t *testing.T, m *Manifest, es *[]Entry) { m.Epoch++ }},
		{"trusted_readme", "archive member differs from trusted bundle source: README.md", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i := range *es {
				if (*es)[i].Name == "README.md" {
					(*es)[i].Data = []byte("Self-consistent untrusted README\n")
				}
			}
		}},
		{"missing_executable", "archive does not contain the complete expected bundle", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i, e := range *es {
				if e.Name == m.Target.Executable() {
					*es = append((*es)[:i], (*es)[i+1:]...)
					return
				}
			}
		}},
		{"missing_expected_member", "missing required member", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i, e := range *es {
				if e.Name == m.Target.Executable() {
					(*es)[i] = Entry{Name: "unexpected.txt", Data: []byte("replacement"), Mode: 0644}
					return
				}
			}
		}},
		{"executable_mode", "incorrect package mode", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i := range *es {
				if (*es)[i].Name == m.Target.Executable() {
					(*es)[i].Mode = 0644
				}
			}
		}},
		{"binary_metadata", "executable Go build metadata", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i := range *es {
				if (*es)[i].Name == m.Target.Executable() {
					(*es)[i].Data = []byte("not a Go executable")
				}
			}
		}},
		{"binary_toolchain", "executable module or toolchain mismatch", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i := range *es {
				if (*es)[i].Name == m.Target.Executable() {
					old := (*es)[i].Data
					(*es)[i].Data = bytes.ReplaceAll(old, []byte(m.Toolchain), []byte(differentTestString(m.Toolchain)))
					if bytes.Equal(old, (*es)[i].Data) {
						t.Fatal("toolchain mutation not applied")
					}
				}
			}
		}},
		{"binary_identity", "executable does not contain the claimed release identity", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for i := range *es {
				if (*es)[i].Name == m.Target.Executable() {
					(*es)[i].Data = bytes.ReplaceAll((*es)[i].Data, []byte(m.Version+"\x00"), []byte(strings.Repeat("x", len(m.Version))+"\x00"))
				}
			}
		}},
		{"binary_revision", "executable does not contain the claimed release identity", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			mutateTestBinary(t, es, m.Target.Executable(), m.Revision+"\x00", strings.Repeat("2", 40)+"\x00")
		}},
		{"binary_module", "executable module or toolchain mismatch", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			mutateTestBinary(t, es, m.Target.Executable(), "github.com/dlubom/survex-duplicates", "github.com/dlubom/survex-duplicatXs")
		}},
		{"binary_cgo", "executable CGO_ENABLED mismatch", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			mutateTestBinary(t, es, m.Target.Executable(), "CGO_ENABLED=0", "CGO_ENABLED=1")
		}},
		{"binary_trimpath", "executable -trimpath mismatch", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			mutateTestBinary(t, es, m.Target.Executable(), "-trimpath=true", "-trimpath=nope")
		}},
		{"binary_cpu", "executable CPU setting", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			for k, v := range m.Target.Env {
				mutateTestBinary(t, es, m.Target.Executable(), k+"="+v, k+"="+strings.ReplaceAll(strings.ReplaceAll(v, "v1", "v2"), "v8.0", "v8.1"))
			}
		}},
		{"binary_architecture", "executable GOARCH mismatch", true, func(t *testing.T, m *Manifest, es *[]Entry) {
			c, err := LoadConfig(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, target := range c.Targets {
				if target.GOOS == m.Target.GOOS && target.GOARCH != m.Target.GOARCH {
					m.Target = target
					break
				}
			}
			for i := range *es {
				if (*es)[i].Name == "README.md" {
					b, err := os.ReadFile(filepath.Join(root, "release/README.md.tmpl"))
					if err != nil {
						t.Fatal(err)
					}
					(*es)[i].Data = []byte(strings.NewReplacer("{{VERSION}}", m.Version, "{{TARGET}}", m.Target.ID).Replace(string(b)))
				}
			}
		}},
	}
	for _, tc := range tests {
		t.Run("Verify/"+tc.name, func(t *testing.T) {
			entries := archiveTestEntries(t, archive)
			// JSON cloning avoids sharing source/file maps across independent mutations.
			b, _ := json.Marshal(original)
			var m Manifest
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, &m, &entries)
			entries = rewriteTestManifest(t, entries, m, tc.coherent)
			bad := filepath.Join(t.TempDir(), ArchiveName(m.Version, m.Target))
			if err := WriteArchive(bad, "zip", m.Epoch, entries); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(root, bad); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("wanted %q, got %v", tc.diagnostic, err)
			}
		})
	}
	t.Run("Verify/filename", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "wrong.zip")
		b, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, bad, b)
		if _, err := Verify(root, bad); err == nil || !strings.Contains(err.Error(), "archive filename") {
			t.Fatal(err)
		}
	})
	t.Run("Build/preflight_errors", func(t *testing.T) {
		cases := []struct {
			name, relative, diagnostic string
			replacement                []byte
			remove                     bool
		}{
			{name: "missing_bundle", relative: "NOTICE.md", remove: true},
			{name: "unresolved_readme", relative: "release/README.md.tmpl", replacement: []byte("{{UNKNOWN}}"), diagnostic: "unresolved README placeholder"},
			{name: "changed_demo", relative: "examples/kataster/KATASTER.legs.dump", replacement: []byte("corrupt"), diagnostic: "demo provenance mismatch"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				name := filepath.Join(root, filepath.FromSlash(tc.relative))
				before, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { writeTestFile(t, name, before) })
				if tc.remove {
					if err := os.Remove(name); err != nil {
						t.Fatal(err)
					}
				} else {
					writeTestFile(t, name, tc.replacement)
				}
				out := t.TempDir()
				_, err = Build(root, original.Target.ID, original.Version, out, true)
				if err == nil || (tc.diagnostic != "" && !strings.Contains(err.Error(), tc.diagnostic)) {
					t.Fatalf("wanted %q, got %v", tc.diagnostic, err)
				}
				if files, err := os.ReadDir(out); err != nil || len(files) != 0 {
					t.Fatalf("preflight wrote output: %v, %v", files, err)
				}
			})
		}
		if _, err := Build(root, "unreviewed", original.Version, t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "unknown target") {
			t.Fatal(err)
		}
		if _, err := Build(root, original.Target.ID, "v9.9.9", t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "version must match") {
			t.Fatal(err)
		}
	})
	t.Run("Build/unsafe_output", func(t *testing.T) {
		for _, out := range []string{root, filepath.Join(root, "unignored-output")} {
			if _, err := Build(root, original.Target.ID, original.Version, out, true); err == nil || !strings.Contains(err.Error(), "output") {
				t.Fatal(err)
			}
		}
	})
	t.Run("Build/existing_archive", func(t *testing.T) {
		before, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Build(root, original.Target.ID, original.Version, filepath.Dir(archive), true); err == nil || !strings.Contains(err.Error(), "archive already exists") {
			t.Fatal(err)
		}
		after, err := os.ReadFile(archive)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("existing archive changed", err)
		}
	})
	t.Run("Build/existing_sidecar_is_preserved", func(t *testing.T) {
		out := t.TempDir()
		dest := filepath.Join(out, filepath.Base(archive))
		writeTestFile(t, dest+".sha256", []byte("keep existing sidecar\n"))
		if _, err := Build(root, original.Target.ID, original.Version, out, true); err == nil {
			t.Fatal("overwrote existing sidecar")
		}
		got, err := os.ReadFile(dest + ".sha256")
		if err != nil || string(got) != "keep existing sidecar\n" {
			t.Fatalf("existing sidecar changed: %q, %v", got, err)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("failed build left archive: %v", err)
		}
	})
	t.Run("Build/sidecar_failure_cleans_archive", func(t *testing.T) {
		out := t.TempDir()
		dest := filepath.Join(out, filepath.Base(archive))
		if err := os.Mkdir(dest+".sha256", 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := Build(root, original.Target.ID, original.Version, out, true); err == nil {
			t.Fatal("accepted blocked checksum sidecar")
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("failed build left archive: %v", err)
		}
		if info, err := os.Stat(dest + ".sha256"); err != nil || !info.IsDir() {
			t.Fatalf("existing sidecar blocker changed: %v", err)
		}
	})
	t.Run("Assemble/rejects_before_metadata", func(t *testing.T) {
		c, err := LoadConfig(root)
		if err != nil {
			t.Fatal(err)
		}
		archiveBytes, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"missing", "surplus", "bad_sidecar", "wrong_identity", "invalid_archive"} {
			t.Run(mode, func(t *testing.T) {
				dir := t.TempDir()
				for name := range expectedAssets(c, original.Version) {
					writeTestFile(t, filepath.Join(dir, name), []byte("placeholder"))
				}
				name := filepath.Base(archive)
				writeTestFile(t, filepath.Join(dir, name), archiveBytes)
				digest, err := hashFile(archive)
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(dir, name+".sha256"), []byte(digest+"  "+name+"\n"))
				revision := original.Revision
				want := ""
				switch mode {
				case "missing":
					if err := os.Remove(filepath.Join(dir, name)); err != nil {
						t.Fatal(err)
					}
					want = "missing release asset"
				case "surplus":
					writeTestFile(t, filepath.Join(dir, "extra.zip"), nil)
					want = "unexpected release asset"
				case "bad_sidecar":
					writeTestFile(t, filepath.Join(dir, name+".sha256"), []byte("wrong\n"))
					want = "invalid archive checksum sidecar"
				case "wrong_identity":
					revision = strings.Repeat("2", 40)
					want = "inconsistent release identity"
				case "invalid_archive":
					writeTestFile(t, filepath.Join(dir, name), []byte("invalid ZIP"))
					want = "zip: not a valid zip file"
				}
				if _, err := Assemble(root, dir, original.Version, revision, true); err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("wanted %q, got %v", want, err)
				}
				for _, n := range []string{"SHA256SUMS", "release-manifest.json"} {
					if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
						t.Fatalf("failure published %s: %v", n, err)
					}
				}
			})
		}
	})
}

func TestAssemblyMetadataFailureCleansPartialPublication(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "release-manifest.json")
	if err := os.Mkdir(blocked, 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeAssemblyMetadata(dir, []byte("checksums\n"), []byte("{}\n")); err == nil {
		t.Fatal("accepted blocked manifest")
	}
	if _, err := os.Stat(filepath.Join(dir, "SHA256SUMS")); !os.IsNotExist(err) {
		t.Fatalf("failed assembly left partial metadata: %v", err)
	}
	if info, err := os.Stat(blocked); err != nil || !info.IsDir() {
		t.Fatalf("existing blocker changed: %v", err)
	}
}

func mutateTestBinary(t *testing.T, entries *[]Entry, name, old, replacement string) {
	t.Helper()
	if len(old) != len(replacement) {
		t.Fatal("binary metadata mutations must preserve size")
	}
	for i := range *entries {
		if (*entries)[i].Name == name {
			before := (*entries)[i].Data
			(*entries)[i].Data = bytes.ReplaceAll(before, []byte(old), []byte(replacement))
			if bytes.Equal(before, (*entries)[i].Data) {
				t.Fatalf("binary mutation %q was not applied", old)
			}
			return
		}
	}
	t.Fatalf("missing fixture executable %s", name)
}

func TestAssemblyMetadataPreservesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "SHA256SUMS"), []byte("keep\n"))
	if err := writeAssemblyMetadata(dir, []byte("replace\n"), []byte("{}\n")); err == nil {
		t.Fatal("replaced existing assembly metadata")
	}
	got, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("existing checksums changed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "release-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("failure published manifest: %v", err)
	}
}

func TestAssemblyMetadataWritesCompletePair(t *testing.T) {
	dir := t.TempDir()
	if err := writeAssemblyMetadata(dir, []byte("checksums\n"), []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"SHA256SUMS": "checksums\n", "release-manifest.json": "{}\n"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", name, got, err)
		}
	}
}

func differentTestString(s string) string {
	replacement := byte('0')
	if s[len(s)-1] == replacement {
		replacement = '1'
	}
	return s[:len(s)-1] + string(replacement)
}
