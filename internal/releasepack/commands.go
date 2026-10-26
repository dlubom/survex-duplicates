package releasepack

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
)

type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Target string `json:"target"`
}
type ReleaseManifest struct {
	SchemaVersion int     `json:"schema_version"`
	Version       string  `json:"version"`
	Revision      string  `json:"revision"`
	Snapshot      bool    `json:"snapshot"`
	Assets        []Asset `json:"assets"`
}

func expectedAssets(c Config, version string) map[string]Target {
	expected := map[string]Target{}
	for _, t := range c.Targets {
		n := ArchiveName(version, t)
		expected[n] = t
		expected[n+".sha256"] = t
	}
	return expected
}
func checkAssetNames(dir string, expected map[string]Target) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range files {
		if _, ok := expected[f.Name()]; !ok || !f.Type().IsRegular() {
			return fmt.Errorf("unexpected release asset %s", f.Name())
		}
		seen[f.Name()] = true
	}
	for n := range expected {
		if !seen[n] {
			return fmt.Errorf("missing release asset %s", n)
		}
	}
	return nil
}

// Assemble refuses partial and surplus assets before writing the publication list.
func Assemble(root, dir, version, revision string, snapshot bool) (ReleaseManifest, error) {
	r := ReleaseManifest{SchemaVersion: 1, Version: version, Revision: revision, Snapshot: snapshot, Assets: []Asset{}}
	if !versionPattern.MatchString(version) || !revisionPattern.MatchString(revision) {
		return r, fmt.Errorf("invalid assembly version or revision")
	}
	c, err := LoadValidated(root)
	if err != nil {
		return r, err
	}
	if err = checkAssetNames(dir, expectedAssets(c, version)); err != nil {
		return r, err
	}
	var source map[string]string
	var epoch int64
	for _, t := range c.Targets {
		n := ArchiveName(version, t)
		p := filepath.Join(dir, n)
		m, err := Verify(root, p)
		if err != nil {
			return r, fmt.Errorf("%s: %w", n, err)
		}
		if m.Version != version || m.Revision != revision || m.Snapshot != snapshot || (!snapshot && m.Dirty) {
			return r, fmt.Errorf("inconsistent release identity in %s", n)
		}
		if source == nil {
			source, epoch = m.Source, m.Epoch
		} else if !reflect.DeepEqual(source, m.Source) || epoch != m.Epoch {
			return r, fmt.Errorf("archives were built from different source snapshots")
		}
		h, err := hashFile(p)
		if err != nil {
			return r, err
		}
		side, err := os.ReadFile(p + ".sha256")
		if err != nil {
			return r, err
		}
		if string(side) != h+"  "+n+"\n" {
			return r, fmt.Errorf("invalid archive checksum sidecar for %s", n)
		}
		r.Assets = append(r.Assets, Asset{Name: n, SHA256: h, Target: t.ID})
	}
	sort.Slice(r.Assets, func(i, j int) bool { return r.Assets[i].Name < r.Assets[j].Name })
	var sums strings.Builder
	for _, a := range r.Assets {
		fmt.Fprintf(&sums, "%s  %s\n", a.SHA256, a.Name)
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return r, err
	}
	return r, writeAssemblyMetadata(dir, []byte(sums.String()), append(b, '\n'))
}

func writeAssemblyMetadata(dir string, sums, manifest []byte) error {
	checksums := filepath.Join(dir, "SHA256SUMS")
	if err := writeExclusive(checksums, sums); err != nil {
		return err
	}
	if err := writeExclusive(filepath.Join(dir, "release-manifest.json"), manifest); err != nil {
		os.Remove(checksums)
		return err
	}
	return nil
}

func Smoke(root, archive string, w io.Writer) error {
	m, err := Verify(root, archive)
	if err != nil {
		return err
	}
	if m.Target.GOOS != runtime.GOOS || m.Target.GOARCH != runtime.GOARCH {
		return fmt.Errorf("cannot execute %s/%s on host %s/%s; no smoke test was run", m.Target.GOOS, m.Target.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	dir, err := os.MkdirTemp("", "survex-release-smoke-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	_, err = ReadArchive(archive, func(n string, mode int64, r io.Reader) error {
		p := filepath.Join(dir, filepath.FromSlash(n))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(mode))
		if err != nil {
			return err
		}
		_, err = io.Copy(f, r)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		return err
	}
	run := func(args ...string) ([]byte, error) {
		exe := filepath.Join(dir, m.Target.Executable())

		cmd := exec.Command(exe, args...)
		cmd.Dir = dir
		for _, s := range os.Environ() {
			k, _, _ := strings.Cut(s, "=")
			if !strings.EqualFold(k, "PATH") {
				cmd.Env = append(cmd.Env, s)
			}
		}
		cmd.Env = append(cmd.Env, "PATH=")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		b, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("smoke execution: %w: %s", err, stderr.String())
		}
		return b, nil
	}
	b, err := run("--version")
	if err != nil {
		return err
	}
	if string(b) != fmt.Sprintf("survex-duplicates %s (%s)\n", m.Version, m.Revision) {
		return fmt.Errorf("--version differs from archive identity")
	}
	b, err = os.ReadFile(filepath.Join(dir, "examples/kataster/results/verification.json"))
	if err != nil {
		return err
	}
	var verification demoVerification
	if err = json.Unmarshal(b, &verification); err != nil {
		return err
	}
	count := 0
	for _, v := range verification.Runs {
		if v.Mode != "relative_percent" || v.Value != "5" || v.IncludeDuplicates {
			continue
		}
		out := filepath.Join(dir, fmt.Sprintf("smoke-%d.csv", count))
		args := []string{"examples/kataster/KATASTER.legs.dump", "--input-format", "dump", "--out", out}
		if v.ExcludeSplays {
			args = append(args, "--exclude-splays")
		}
		if _, err = run(args...); err != nil {
			return err
		}
		actual, err := os.ReadFile(out)
		if err != nil {
			return err
		}
		expected, err := os.ReadFile(filepath.Join(dir, "examples/kataster/results", v.CSV))
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("full CSV differs from bundled reference %s", v.CSV)
		}
		fmt.Fprintf(w, "passed: %s (complete CSV bytes)\n", v.CSV)
		count++
	}
	if count != 2 {
		return fmt.Errorf("expected default and no-SPLAY reference runs")
	}
	fmt.Fprintf(w, "passed: %s --version (%s %s)\n", m.Target.ID, m.Version, m.Revision)
	return nil
}

func Run(args []string, stdout, stderr io.Writer) int {
	err := runCLI(args, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}
func runCLI(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: release matrix | build | verify | assemble | smoke")
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var target, version, out, archive, dir, revision string
	var snapshot bool
	switch command {
	case "matrix":
	case "build":
		fs.StringVar(&target, "target", "", "target ID")
		fs.StringVar(&version, "version", "", "version with v prefix")
		fs.StringVar(&out, "out", "dist", "archive output directory")
		fs.BoolVar(&snapshot, "snapshot", false, "allow a source snapshot instead of an exact clean tag")
	case "verify", "smoke":
		fs.StringVar(&archive, "archive", "", "archive path")
	case "assemble":
		fs.StringVar(&dir, "dir", "dist", "directory containing exactly the archives and checksum sidecars")
		fs.StringVar(&version, "version", "", "version with v prefix")
		fs.StringVar(&revision, "revision", "", "revision including -dirty for dirty snapshots")
		fs.BoolVar(&snapshot, "snapshot", false, "assemble snapshots, never tagged release assets")
	default:
		return fmt.Errorf("unknown release command %q", command)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	switch command {
	case "matrix":
		c, err := LoadValidated(root)
		if err != nil {
			return err
		}
		include := []map[string]string{}
		for _, t := range c.Targets {
			include = append(include, map[string]string{"target": t.ID, "runner": t.Runner})
		}
		return json.NewEncoder(stdout).Encode(map[string]any{"include": include})
	case "build":
		p, err := Build(root, target, version, out, snapshot)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, p)
	case "verify":
		m, err := Verify(root, archive)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(map[string]any{"verified": filepath.Base(archive), "target": m.Target.ID, "version": m.Version, "revision": m.Revision, "snapshot": m.Snapshot})
	case "smoke":
		return Smoke(root, archive, stdout)
	case "assemble":
		r, err := Assemble(root, dir, version, revision, snapshot)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(r)
	}
	return nil
}
