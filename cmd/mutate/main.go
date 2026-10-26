// Command mutate runs the reviewed mutation catalogue in an isolated copy of
// the current working tree. It never edits source files in the original tree.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type mutation struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type catalogue struct {
	SchemaVersion int        `json:"schema_version"`
	Mutations     []mutation `json:"mutations"`
}

type result struct {
	Name         string   `json:"name"`
	File         string   `json:"file,omitempty"`
	Status       string   `json:"status"`
	ExitCode     int      `json:"exit_code"`
	Seconds      float64  `json:"seconds"`
	FailingTests []string `json:"failing_tests"`
	Log          string   `json:"log"`
}

type report struct {
	SchemaVersion  int      `json:"schema_version"`
	SourceRevision string   `json:"source_revision"`
	SourceDirty    bool     `json:"source_dirty"`
	Command        []string `json:"command"`
	Baseline       result   `json:"baseline"`
	Results        []result `json:"results"`
}

var errStale = errors.New("mutation must replace exactly one nonempty occurrence")

func classify(output []byte, exitCode int, timedOut bool) (string, []string) {
	failures := map[string]bool{}
	buildFailed := false
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var event struct{ Action, Test, Output string }
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if event.Action == "fail" && event.Test != "" {
			failures[event.Test] = true
		}
		if event.Action == "build-fail" || strings.Contains(event.Output, "[build failed]") {
			buildFailed = true
		}
		if strings.Contains(event.Output, "panic: test timed out after ") {
			timedOut = true
		}
	}
	names := make([]string, 0, len(failures))
	for name := range failures {
		names = append(names, name)
	}
	slices.Sort(names)
	switch {
	case timedOut:
		return "timeout", names
	case exitCode == 0:
		return "survived", names
	case exitCode < 0 || buildFailed || scanner.Err() != nil:
		return "invalid", names
	case len(names) > 0:
		return "killed", names
	default:
		return "invalid", names
	}
}

func safeRelativePath(relative string) bool {
	if relative == "" || path.IsAbs(relative) || strings.Contains(relative, "\\") || path.Clean(relative) != relative {
		return false
	}
	if _, err := filepath.Localize(relative); err != nil {
		return false
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if part == ".." || part == ".git" || part == "." {
			return false
		}
	}
	return true
}

// regularPath rejects links in every path component, including dangling links.
func regularPath(root, relative string) (string, os.FileInfo, error) {
	if !safeRelativePath(relative) {
		return "", nil, fmt.Errorf("unsafe source path %q", relative)
	}
	parts := strings.Split(relative, "/")
	path := root
	for i, part := range parts {
		if part == ".." || part == ".git" || part == "." {
			return "", nil, fmt.Errorf("unsafe source path %q", relative)
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("source path is a symlink: %s", relative)
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return "", nil, fmt.Errorf("source parent is not a directory: %s", relative)
			}
		} else {
			if !info.Mode().IsRegular() {
				return "", nil, fmt.Errorf("source is not a regular file: %s", relative)
			}
			return path, info, nil
		}
	}
	return "", nil, fmt.Errorf("unsafe source path %q", relative)
}

func replaceMutation(root string, m mutation) (func() error, error) {
	path, info, err := regularPath(root, m.File)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if m.Before == "" || bytes.Count(content, []byte(m.Before)) != 1 {
		return nil, errStale
	}
	if err := os.WriteFile(path, bytes.Replace(content, []byte(m.Before), []byte(m.After), 1), info.Mode().Perm()); err != nil {
		return nil, err
	}
	return func() error { return os.WriteFile(path, content, info.Mode().Perm()) }, nil
}

func gitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	cmd.Env = sanitizedGitEnvironment(os.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return output, nil
}

func sanitizedGitEnvironment(entries []string) []string {
	// An inherited GIT_DIR or index override must never redirect snapshot writes
	// into the user's repository, including Git commands launched by tests.
	// Git's ordinary global configuration still applies.
	var environment []string
	for _, entry := range entries {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			environment = append(environment, entry)
		}
	}
	return environment
}

func snapshot(ctx context.Context, root string) (copyRoot string, err error) {
	names, err := gitOutput(ctx, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	copyRoot, err = os.MkdirTemp("", "survex-mutations-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(copyRoot)
		}
	}()
	for _, name := range bytes.Split(names, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		source, info, pathErr := regularPath(root, string(name))
		if errors.Is(pathErr, os.ErrNotExist) {
			continue
		} // A deleted tracked path is absent in the current tree.
		if pathErr != nil {
			return copyRoot, pathErr
		}
		destination := filepath.Join(copyRoot, string(name))
		if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return copyRoot, err
		}
		var content []byte
		content, err = os.ReadFile(source)
		if err != nil {
			return copyRoot, err
		}
		if err = os.WriteFile(destination, content, info.Mode().Perm()); err != nil {
			return copyRoot, err
		}
	}
	// Release verification tests inventory the source with Git. Give the copy its
	// own repository and local commit; never copy the original Git metadata.
	for _, args := range [][]string{
		{"-c", "init.templateDir=", "init", "-q"},
		{"add", "--force", "--all"},
		{"-c", "user.name=Mutation Test", "-c", "user.email=mutation@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=.mutation-no-hooks", "commit", "-qm", "Isolated mutation source", "--allow-empty"},
	} {
		if _, err = gitOutput(ctx, copyRoot, args...); err != nil {
			return copyRoot, err
		}
	}
	return copyRoot, nil
}

func testArguments(testTimeout time.Duration) []string {
	return []string{"test", "-json", "-count=1", "-timeout=" + strconv.FormatFloat(testTimeout.Seconds(), 'g', -1, 64) + "s", "./..."}
}

func runSuite(ctx context.Context, root, logPath string, testTimeout, commandTimeout time.Duration) result {
	started := time.Now()
	r := result{ExitCode: -1, FailingTests: []string{}}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", testArguments(testTimeout)...)
	cmd.Dir = root
	cmd.Env = sanitizedGitEnvironment(os.Environ())
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err == nil {
		r.ExitCode = 0
	}
	r.Status, r.FailingTests = classify(stdout.Bytes(), r.ExitCode, errors.Is(ctx.Err(), context.DeadlineExceeded))
	r.Seconds = time.Since(started).Seconds()
	log := append(bytes.Clone(stdout.Bytes()), stderr.Bytes()...)
	if err != nil {
		log = append(log, []byte("\nGo command: "+err.Error()+"\n")...)
	}
	if err := os.WriteFile(logPath, log, 0644); err != nil {
		r.Status = "invalid"
	}
	return r
}

func loadCatalogue(path string) (catalogue, error) {
	var c catalogue
	content, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return c, err
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return c, fmt.Errorf("unexpected trailing catalogue content")
	}
	if c.SchemaVersion != 1 || len(c.Mutations) == 0 {
		return c, fmt.Errorf("expected schema 1 and a nonempty mutation catalogue")
	}
	names := map[string]bool{}
	pattern := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, m := range c.Mutations {
		if !pattern.MatchString(m.Name) || names[m.Name] || !safeRelativePath(m.File) || m.Before == "" || m.Before == m.After {
			return c, fmt.Errorf("invalid or duplicate mutation %q", m.Name)
		}
		names[m.Name] = true
	}
	return c, nil
}

func saveReport(out string, r report) error {
	content, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "results.json"), append(content, '\n'), 0644)
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("mutate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	rootFlag := flags.String("root", ".", "repository working tree")
	catalogueFlag := flags.String("mutations", "quality/mutations.json", "reviewed catalogue relative to root")
	outFlag := flags.String("out", "reports/mutations", "logs and results directory relative to root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}
	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx := context.Background()
	top, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	root = strings.TrimSpace(string(top))
	cataloguePath := *catalogueFlag
	if !filepath.IsAbs(cataloguePath) {
		cataloguePath = filepath.Join(root, cataloguePath)
	}
	c, err := loadCatalogue(cataloguePath)
	if err != nil {
		fmt.Fprintln(stderr, "Mutation catalogue:", err)
		return 1
	}
	revision, err := gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	dirty, err := gitOutput(ctx, root, "status", "--porcelain")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	copyRoot, err := snapshot(ctx, root)
	if err != nil {
		fmt.Fprintln(stderr, "Source snapshot:", err)
		return 1
	}
	defer os.RemoveAll(copyRoot)
	out := *outFlag
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	r := report{SchemaVersion: 1, SourceRevision: strings.TrimSpace(string(revision)), SourceDirty: len(dirty) > 0, Command: append([]string{"go"}, testArguments(90*time.Second)...), Results: []result{}}
	r.Baseline = runSuite(ctx, copyRoot, filepath.Join(out, "baseline.log"), 90*time.Second, 2*time.Minute)
	r.Baseline.Name, r.Baseline.Log = "baseline", "baseline.log"
	if err := saveReport(out, r); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if r.Baseline.Status != "survived" {
		fmt.Fprintln(stderr, "Baseline did not pass; inspect baseline.log.")
		return 1
	}
	fmt.Fprintln(stdout, "Baseline passed.")
	status := 0
	for _, m := range c.Mutations {
		logName := m.Name + ".log"
		entry := result{Name: m.Name, File: m.File, ExitCode: -1, FailingTests: []string{}, Log: logName}
		restore, err := replaceMutation(copyRoot, m)
		if err != nil {
			entry.Status = "invalid"
			if errors.Is(err, errStale) {
				entry.Status = "stale"
			}
			// Keep infrastructure diagnostics in logs, never absolute temp paths in JSON.
			if logErr := os.WriteFile(filepath.Join(out, logName), []byte(err.Error()+"\n"), 0644); logErr != nil {
				fmt.Fprintln(stderr, logErr)
				return 1
			}
		} else {
			entry = runSuite(ctx, copyRoot, filepath.Join(out, logName), 90*time.Second, 2*time.Minute)
			entry.Name, entry.File, entry.Log = m.Name, m.File, logName
			if err := restore(); err != nil {
				fmt.Fprintln(stderr, "Could not restore isolated source; stopping.")
				return 1
			}
		}
		r.Results = append(r.Results, entry)
		if entry.Status != "killed" {
			status = 1
		}
		fmt.Fprintf(stdout, "%s: %s\n", m.Name, entry.Status)
		if err := saveReport(out, r); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return status
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
