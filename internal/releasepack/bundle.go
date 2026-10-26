package releasepack

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type demoProvenance struct {
	SHA256 map[string]string `json:"sha256"`
}
type demoRun struct {
	CSV               string `json:"csv"`
	CSVHash           string `json:"csv_sha256"`
	Mode              string `json:"tolerance_mode"`
	Value             string `json:"tolerance_value"`
	ExcludeSplays     bool   `json:"exclude_splays"`
	IncludeDuplicates bool   `json:"include_duplicates"`
}
type demoVerification struct {
	Input  string    `json:"input"`
	Before string    `json:"sha256_before"`
	After  string    `json:"sha256_after"`
	Runs   []demoRun `json:"runs"`
}

var requiredDocs = []string{"LICENSE", "NOTICE.md", "GLOSSARY.md", "docs/usage.md", "docs/platforms.md", "docs/releasing.md", "docs/specification.md"}

func bundleEntries(root string, target Target, version string) ([]Entry, error) {
	entries := []Entry{}
	add := func(rel string) error {
		p := filepath.Join(root, filepath.FromSlash(rel))
		s, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !s.Mode().IsRegular() {
			return fmt.Errorf("bundle input is not regular: %s", rel)
		}
		entries = append(entries, Entry{Name: rel, Path: p, Mode: 0644})
		return nil
	}
	for _, rel := range requiredDocs {
		if err := add(rel); err != nil {
			return nil, err
		}
	}
	for _, dir := range []string{"docs/adr", "examples/kataster"} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if dir == "docs/adr" && !strings.HasSuffix(rel, ".md") {
				return nil
			}
			return add(filepath.ToSlash(rel))
		})
		if err != nil {
			return nil, err
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "release/README.md.tmpl"))
	if err != nil {
		return nil, err
	}
	readme := strings.NewReplacer("{{VERSION}}", version, "{{TARGET}}", target.ID).Replace(string(b))
	if strings.Contains(readme, "{{") {
		return nil, fmt.Errorf("unresolved README placeholder")
	}
	entries = append(entries, Entry{Name: "README.md", Data: []byte(readme), Mode: 0644})
	if err := add("release/GO-LICENSE"); err != nil {
		return nil, err
	}
	entries[len(entries)-1].Name = "GO-LICENSE"

	return entries, nil
}

func verifyDemoEntries(entries []Entry) error {
	records := map[string]FileRecord{}
	contents := map[string][]byte{}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, "examples/kataster/") {
			continue
		}
		r, err := entryRecord(e)
		if err != nil {
			return err
		}
		records[e.Name] = r
		if e.Name == "examples/kataster/provenance.json" || e.Name == "examples/kataster/results/verification.json" {
			f, _, err := e.open()
			if err != nil {
				return err
			}
			b, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return err
			}
			contents[e.Name] = b
		}
	}
	return verifyDemo(records, contents)
}

func verifyDemo(records map[string]FileRecord, contents map[string][]byte) error {
	var p demoProvenance
	var v demoVerification
	if err := json.Unmarshal(contents["examples/kataster/provenance.json"], &p); err != nil {
		return fmt.Errorf("demo provenance: %w", err)
	}
	if err := json.Unmarshal(contents["examples/kataster/results/verification.json"], &v); err != nil {
		return fmt.Errorf("demo verification: %w", err)
	}
	if len(p.SHA256) != 3 {
		return fmt.Errorf("demo provenance needs model, dump and licence digests")
	}
	for _, file := range []string{"KATASTER.3d", "KATASTER.legs.dump", "LICENCE"} {
		digest := p.SHA256[file]
		if !validDigest(digest) || records["examples/kataster/"+file].SHA256 != digest {
			return fmt.Errorf("demo provenance mismatch for %s", file)
		}
	}
	if v.Input != "examples/kataster/KATASTER.3d" || v.Before != p.SHA256["KATASTER.3d"] || v.After != v.Before {
		return fmt.Errorf("demo model verification differs from provenance")
	}
	if len(v.Runs) != 8 {
		return fmt.Errorf("demo needs all eight saved reference reports")
	}
	seen := map[string]bool{}
	for _, r := range v.Runs {
		if !safeName(r.CSV) || strings.Contains(r.CSV, "/") || seen[r.CSV] || !validDigest(r.CSVHash) || records["examples/kataster/results/"+r.CSV].SHA256 != r.CSVHash {
			return fmt.Errorf("demo CSV verification failed for %q", r.CSV)
		}
		seen[r.CSV] = true
	}
	return nil
}
