package releasepublish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	return nil
}
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
func fileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if err = regularFile(filepath.Join(dir, entry.Name())); err != nil {
			return nil, fmt.Errorf("non-regular release asset: %w", err)
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}
func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] || (i > 0 && a[i] == a[i-1]) {
			return false
		}
	}
	return true
}
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (p *publisher) validateAssets() (map[string]string, error) {
	version, err := os.ReadFile(p.path("VERSION"))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(version)) != strings.TrimPrefix(p.version, "v") {
		return nil, fmt.Errorf("release version does not match VERSION")
	}
	var config struct {
		SchemaVersion int `json:"schema_version"`
		Targets       []struct {
			ID      string `json:"id"`
			Archive string `json:"archive"`
		} `json:"targets"`
	}
	if err = readJSON(p.path("release/targets.json"), &config); err != nil {
		return nil, err
	}
	var expectedTargets []string
	for _, system := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			expectedTargets = append(expectedTargets, system+"_"+arch)
		}
	}
	var targetIDs []string
	for _, target := range config.Targets {
		if target.Archive != "zip" {
			return nil, fmt.Errorf("expected exactly six approved native ZIP targets")
		}
		targetIDs = append(targetIDs, target.ID)
	}
	if config.SchemaVersion != 1 || !sameNames(targetIDs, expectedTargets) {
		return nil, fmt.Errorf("expected exactly six approved native ZIP targets")
	}
	var manifest struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
		Revision      string `json:"revision"`
		Snapshot      *bool  `json:"snapshot"`
		Assets        []struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
			Target string `json:"target"`
		} `json:"assets"`
	}
	if err = readJSON(p.path("dist/release-manifest.json"), &manifest); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != 1 || manifest.Version != p.version || manifest.Revision != p.revision || manifest.Snapshot == nil || *manifest.Snapshot {
		return nil, fmt.Errorf("release manifest identity differs or is a snapshot")
	}
	var assetTargets []string
	for _, a := range manifest.Assets {
		assetTargets = append(assetTargets, a.Target)
	}
	if !sameNames(assetTargets, expectedTargets) {
		return nil, fmt.Errorf("release manifest does not contain exactly six target archives")
	}
	sort.Slice(manifest.Assets, func(i, j int) bool { return manifest.Assets[i].Name < manifest.Assets[j].Name })
	expectedFiles := []string{"SHA256SUMS", "release-manifest.json"}
	var checksumLines strings.Builder
	for _, asset := range manifest.Assets {
		name := "survex-duplicates_" + p.version + "_" + asset.Target + ".zip"
		if asset.Name != name || !digestPattern.MatchString(asset.SHA256) {
			return nil, fmt.Errorf("invalid archive name or SHA-256 in release manifest")
		}
		expectedFiles = append(expectedFiles, name, name+".sha256")
		archive := p.path("dist/" + name)
		if err = regularFile(archive); err != nil {
			return nil, err
		}
		digest, err := hashFile(archive)
		if err != nil {
			return nil, err
		}
		if digest != asset.SHA256 {
			return nil, fmt.Errorf("archive checksum mismatch: %s", name)
		}
		line := asset.SHA256 + "  " + name + "\n"
		sidecar, err := os.ReadFile(archive + ".sha256")
		if err != nil {
			return nil, err
		}
		if string(sidecar) != line {
			return nil, fmt.Errorf("invalid archive checksum sidecar: %s", name)
		}
		checksumLines.WriteString(line)
	}
	actual, err := fileNames(p.path("dist"))
	if err != nil {
		return nil, err
	}
	if !sameNames(actual, expectedFiles) {
		return nil, fmt.Errorf("missing or unexpected local release assets")
	}
	checksums, err := os.ReadFile(p.path("dist/SHA256SUMS"))
	if err != nil {
		return nil, err
	}
	if string(checksums) != checksumLines.String() {
		return nil, fmt.Errorf("invalid SHA256SUMS")
	}
	hashes := map[string]string{}
	for _, name := range actual {
		digest, err := hashFile(p.path("dist/" + name))
		if err != nil {
			return nil, err
		}
		hashes[name] = digest
	}
	return hashes, nil
}
