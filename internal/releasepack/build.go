package releasepack

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type Manifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Version       string                `json:"version"`
	Revision      string                `json:"revision"`
	Dirty         bool                  `json:"dirty"`
	Snapshot      bool                  `json:"snapshot"`
	Epoch         int64                 `json:"source_date_epoch"`
	Toolchain     string                `json:"toolchain"`
	Target        Target                `json:"target"`
	Source        map[string]string     `json:"source_sha256"`
	Files         map[string]FileRecord `json:"files"`
}

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`)
var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}(-dirty)?$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validDigest(s string) bool { return digestPattern.MatchString(s) }

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

func fingerprints(root string) (map[string]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, rel := range strings.Split(string(b), "\x00") {
		if rel == "" {
			continue
		}
		if !safeName(rel) {
			return nil, fmt.Errorf("unsafe source path %q", rel)
		}
		p := filepath.Join(root, filepath.FromSlash(rel))
		s, err := os.Lstat(p)
		if os.IsNotExist(err) {
			m[rel] = "deleted"
			continue
		}
		if err != nil {
			return nil, err
		}
		if !s.Mode().IsRegular() {
			return nil, fmt.Errorf("source fingerprint requires regular file: %s", rel)
		}
		h, err := hashFile(p)
		if err != nil {
			return nil, err
		}
		m[rel] = h
	}
	return m, nil
}

func SourceManifest(root, version string, snapshot bool) (Manifest, error) {
	m := Manifest{SchemaVersion: 1, Version: version, Snapshot: snapshot, Files: map[string]FileRecord{}}
	if !versionPattern.MatchString(version) {
		return m, fmt.Errorf("version must be vMAJOR.MINOR.PATCH with optional prerelease")
	}
	ver, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return m, err
	}
	want := "v" + strings.TrimSpace(string(ver))
	if (!snapshot && version != want) || (snapshot && version != want && !strings.HasPrefix(version, want+"-")) {
		return m, fmt.Errorf("version must match VERSION (%s)", want)
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return m, err
	}
	status, err := git(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return m, err
	}
	m.Dirty = status != ""
	m.Revision = head
	if m.Dirty {
		m.Revision += "-dirty"
	}
	if !snapshot && m.Dirty {
		return m, fmt.Errorf("tagged release requires a clean tracked and untracked working tree")
	}
	if !snapshot {
		tag, err := git(root, "rev-parse", "refs/tags/"+version+"^{commit}")
		if err != nil {
			return m, fmt.Errorf("tagged release requires existing %s tag: %w", version, err)
		}
		if tag != head {
			return m, fmt.Errorf("release tag does not resolve to HEAD")
		}
	}
	timeText, err := git(root, "show", "-s", "--format=%ct", "HEAD")
	if err != nil {
		return m, err
	}
	m.Epoch, err = strconv.ParseInt(timeText, 10, 64)
	if err != nil {
		return m, err
	}
	toolchain, err := os.ReadFile(filepath.Join(root, ".go-version"))
	if err != nil {
		return m, err
	}
	m.Toolchain = "go" + strings.TrimSpace(string(toolchain))
	m.Source, err = fingerprints(root)
	return m, err
}

func Build(root, id, version, out string, snapshot bool) (string, error) {
	c, err := LoadValidated(root)
	if err != nil {
		return "", err
	}
	target, err := c.Find(id)
	if err != nil {
		return "", err
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if rel, e := filepath.Rel(root, out); e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		if rel == "." {
			return "", fmt.Errorf("output must be outside the repository or in an ignored directory")
		}
		if _, e = git(root, "check-ignore", filepath.ToSlash(rel)+"/"); e != nil {
			return "", fmt.Errorf("output inside repository must be git-ignored (use dist/)")
		}
	}
	m, err := SourceManifest(root, version, snapshot)
	if err != nil {
		return "", err
	}
	m.Target = target
	entries, err := bundleEntries(root, target, version)
	if err != nil {
		return "", err
	}
	if err = verifyDemoEntries(entries); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "survex-release-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	binary := filepath.Join(tmp, target.Executable())
	ldflags := "-X main.version=" + version + " -X main.revision=" + m.Revision
	cmd := exec.Command("go", "build", "-p", "1", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", binary, ".")
	cmd.Dir = root
	cmd.Env = buildEnv(target)
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("build %s: %w", id, err)
	}
	mode := int64(0755)
	entries = append(entries, Entry{Name: target.Executable(), Path: binary, Mode: mode})
	for _, e := range entries {
		r, err := entryRecord(e)
		if err != nil {
			return "", err
		}
		m.Files[e.Name] = r
	}
	after, err := SourceManifest(root, version, snapshot)
	if err != nil {
		return "", err
	}
	if m.Revision != after.Revision || !reflect.DeepEqual(m.Source, after.Source) {
		return "", fmt.Errorf("source changed during build; rebuild from a stable working tree")
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	entries = append(entries, Entry{Name: "manifest.json", Data: append(b, '\n'), Mode: 0644})
	if err = os.MkdirAll(out, 0755); err != nil {
		return "", err
	}
	archive := filepath.Join(out, ArchiveName(version, target))
	if _, err = os.Lstat(archive); err == nil {
		return "", fmt.Errorf("archive already exists: %s", archive)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = WriteArchive(archive, target.Archive, m.Epoch, entries); err != nil {
		return "", err
	}
	if _, err = Verify(root, archive); err != nil {
		os.Remove(archive)
		return "", fmt.Errorf("built archive verification: %w", err)
	}
	h, err := hashFile(archive)
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(archive+".sha256", []byte(h+"  "+filepath.Base(archive)+"\n"), 0644); err != nil {
		return "", err
	}
	return archive, nil
}
