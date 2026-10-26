package releasepack

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func verifySourceIdentity(root string, m Manifest) error {
	current, err := SourceManifest(root, m.Version, m.Snapshot)
	if err != nil {
		return err
	}
	if current.Revision != m.Revision || current.Dirty != m.Dirty || current.Epoch != m.Epoch || !reflect.DeepEqual(current.Source, m.Source) {
		return fmt.Errorf("archive source identity or fingerprints differ from current checkout")
	}
	return nil
}
func verifyExpectedEntries(entries []Entry, records map[string]FileRecord) error {
	for _, e := range entries {
		expected, err := entryRecord(e)
		if err != nil {
			return err
		}
		if records[e.Name] != expected {
			return fmt.Errorf("archive member differs from trusted bundle source: %s", e.Name)
		}
	}
	return nil
}

func verifyInventory(m Manifest, records map[string]FileRecord) error {
	if len(m.Files) != len(records) {
		return fmt.Errorf("manifest member count mismatch")
	}
	for n, want := range m.Files {
		got, ok := records[n]
		if !ok || got != want {
			return fmt.Errorf("manifest digest, size or mode mismatch for %s", n)
		}
	}
	return nil
}

// Verify checks content, exact package inventory, source fingerprints, target,
// toolchain and the bundled survey's separately recorded provenance.
func Verify(root, archive string) (Manifest, error) {
	var m Manifest
	contents := map[string][]byte{}
	var executable []byte
	records, err := ReadArchive(archive, func(name string, mode int64, r io.Reader) error {
		if name == "manifest.json" || name == "examples/kataster/provenance.json" || name == "examples/kataster/results/verification.json" {
			b, err := io.ReadAll(io.LimitReader(r, 8<<20))
			if err != nil {
				return err
			}
			contents[name] = b
		}
		if name == "survex-duplicates" || name == "survex-duplicates.exe" {
			var err error
			executable, err = io.ReadAll(r)
			return err
		}
		return nil
	})
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(contents["manifest.json"], &m); err != nil {
		return m, fmt.Errorf("archive manifest: %w", err)
	}
	if m.SchemaVersion != 1 || !versionPattern.MatchString(m.Version) || !revisionPattern.MatchString(m.Revision) || m.Dirty != strings.HasSuffix(m.Revision, "-dirty") || (!m.Snapshot && m.Dirty) || m.Epoch < 0 {
		return m, fmt.Errorf("invalid archive identity metadata")
	}
	if records["manifest.json"].Mode != 0644 {
		return m, fmt.Errorf("invalid manifest mode")
	}
	delete(records, "manifest.json")
	if err = verifyInventory(m, records); err != nil {
		return m, err
	}
	c, err := LoadConfig(root)
	if err != nil {
		return m, err
	}
	target, err := c.Find(m.Target.ID)
	if err != nil {
		return m, err
	}
	if !reflect.DeepEqual(target, m.Target) {
		return m, fmt.Errorf("archive target differs from reviewed manifest")
	}
	if filepath.Base(archive) != ArchiveName(m.Version, target) {
		return m, fmt.Errorf("archive filename does not match identity")
	}
	toolchain, err := os.ReadFile(filepath.Join(root, ".go-version"))
	if err != nil {
		return m, err
	}
	if m.Toolchain != "go"+strings.TrimSpace(string(toolchain)) {
		return m, fmt.Errorf("archive toolchain does not match pin")
	}
	for n, h := range m.Source {
		if !safeName(n) || (!validDigest(h) && h != "deleted") {
			return m, fmt.Errorf("invalid source fingerprint %q", n)
		}
	}
	for _, n := range []string{"go.mod", "main.go", "VERSION", ".go-version", "release/targets.json", "release/README.md.tmpl", "release/GO-LICENSE"} {
		if !validDigest(m.Source[n]) {
			return m, fmt.Errorf("missing source fingerprint for %s", n)
		}
	}
	entries, err := bundleEntries(root, target, m.Version)
	if err != nil {
		return m, err
	}
	if err = verifySourceIdentity(root, m); err != nil {
		return m, err
	}
	if err = verifyExpectedEntries(entries, records); err != nil {
		return m, err
	}
	expected := map[string]bool{target.Executable(): true}
	for _, e := range entries {
		expected[e.Name] = true
		sourceName := e.Name
		if sourceName == "GO-LICENSE" {
			sourceName = "release/GO-LICENSE"
		}
		if e.Name != "README.md" && m.Source[sourceName] != records[e.Name].SHA256 {
			return m, fmt.Errorf("bundled file differs from recorded source: %s", e.Name)
		}
	}
	if len(expected) != len(records) {
		return m, fmt.Errorf("archive does not contain the complete expected bundle")
	}
	for n := range expected {
		if _, ok := records[n]; !ok {
			return m, fmt.Errorf("missing required member %s", n)
		}
	}
	wantMode := int64(0755)
	for n, r := range records {
		mode := int64(0644)
		if n == target.Executable() {
			mode = wantMode
		}
		if r.Mode != mode {
			return m, fmt.Errorf("incorrect package mode for %s", n)
		}
	}
	if err = verifyDemo(records, contents); err != nil {
		return m, err
	}
	// The repository provenance is the trust anchor, so editing both a survey
	// file and its archived provenance cannot silently replace the known snapshot.
	trusted, err := os.ReadFile(filepath.Join(root, "examples/kataster/provenance.json"))
	if err != nil {
		return m, err
	}
	var want, got demoProvenance
	if err = json.Unmarshal(trusted, &want); err != nil {
		return m, err
	}
	if err = json.Unmarshal(contents["examples/kataster/provenance.json"], &got); err != nil {
		return m, err
	}
	if !reflect.DeepEqual(want.SHA256, got.SHA256) {
		return m, fmt.Errorf("archive survey provenance differs from repository")
	}
	if err = verifyBinary(executable, m); err != nil {
		return m, err
	}
	return m, nil
}

func verifyBinary(data []byte, m Manifest) error {
	bi, err := buildinfo.Read(bytes.NewReader(data))

	if err != nil {
		return fmt.Errorf("executable Go build metadata: %w", err)
	}
	if bi.GoVersion != m.Toolchain || bi.Path != "github.com/dlubom/survex-duplicates" {
		return fmt.Errorf("executable module or toolchain mismatch")
	}
	settings := map[string]string{}
	for _, s := range bi.Settings {
		settings[s.Key] = s.Value
	}
	for k, v := range map[string]string{"GOOS": m.Target.GOOS, "GOARCH": m.Target.GOARCH, "CGO_ENABLED": "0", "-trimpath": "true"} {
		if settings[k] != v {
			return fmt.Errorf("executable %s mismatch: got %q, want %q", k, settings[k], v)
		}
	}
	for k, v := range m.Target.Env {
		if settings[k] != v {
			return fmt.Errorf("executable CPU setting %s mismatch", k)
		}
	}
	// Go omits ldflags from build information with -trimpath. The injected
	// identity strings must still be present; smoke additionally executes --version.
	if !bytes.Contains(data, []byte(m.Version+"\x00")) || !bytes.Contains(data, []byte(m.Revision+"\x00")) {
		return fmt.Errorf("executable does not contain the claimed release identity")
	}
	return nil
}
