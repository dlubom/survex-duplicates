// Package releasepublish publishes only complete, independently read-back
// release assets. The gh boundary can be replaced for offline verification.
package releasepublish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Config supplies the repository directory and optional environment/command
// boundaries. Defaults use the process environment and execute gh in Root.
type Config struct {
	Root    string
	Getenv  func(string) string
	Command func(args ...string) (string, error)
}

type publisher struct {
	root, repository, version, revision, runURL string
	phase                                       string
	created, published, publicationAttempted    bool
	getenv                                      func(string) string
	command                                     func(...string) (string, error)
	evidence                                    map[string]any
	stdout                                      io.Writer
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+/[A-Za-z0-9_.-]+$`)
var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Run executes the publication state machine and always attempts to save its
// confirmation state. A failed publish response is explicitly ambiguous.
func Run(config Config, stdout, stderr io.Writer) int {
	if config.Root == "" {
		config.Root = "."
	}
	if config.Getenv == nil {
		config.Getenv = os.Getenv
	}
	p := &publisher{root: config.Root, phase: "validate_environment", getenv: config.Getenv, command: config.Command, stdout: stdout}
	if p.command == nil {
		p.command = func(args ...string) (string, error) {
			cmd := exec.Command("gh", args...)
			cmd.Dir = p.root
			var diagnostics bytes.Buffer
			cmd.Stderr = &diagnostics
			output, err := cmd.Output()
			if err != nil {
				return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(diagnostics.String()))
			}
			return string(output), nil
		}
	}
	status := 0
	if err := p.execute(); err != nil {
		fmt.Fprintln(stderr, "Release publication stopped:", err)
		switch {
		case p.published:
			fmt.Fprintln(stderr, "The release was made public, but postpublication verification failed; inspect it before further action.")
		case p.publicationAttempted:
			fmt.Fprintln(stderr, "Publication was attempted but not confirmed; inspect the current release state before retrying.")
		case p.created:
			fmt.Fprintln(stderr, "The draft release was left for inspection; it was not made public.")
		}
		status = 1
	}
	if err := p.saveEvidence(); err != nil {
		fmt.Fprintln(stderr, "Could not save publication verification evidence.")
		status = 1
	}
	return status
}

func (p *publisher) path(name string) string { return filepath.Join(p.root, filepath.FromSlash(name)) }
func (p *publisher) gh(args ...string) (string, error) {
	output, err := p.command(args...)
	if err != nil {
		return "", fmt.Errorf("GitHub command failed: gh %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}
func (p *publisher) validateEnvironment() error {
	p.repository = p.getenv("GH_REPO")
	p.version = p.getenv("RELEASE_VERSION")
	p.revision = p.getenv("RELEASE_REVISION")
	p.runURL = p.getenv("RUN_URL")
	for _, name := range []string{"GH_TOKEN", "GH_REPO", "RELEASE_VERSION", "RELEASE_REVISION", "RUN_URL"} {
		if p.getenv(name) == "" {
			return fmt.Errorf("missing required publication environment variable: %s", name)
		}
	}
	if !repositoryPattern.MatchString(p.repository) {
		return fmt.Errorf("invalid GitHub repository")
	}
	if !versionPattern.MatchString(p.version) {
		return fmt.Errorf("expected a stable vMAJOR.MINOR.PATCH release")
	}
	if !revisionPattern.MatchString(p.revision) {
		return fmt.Errorf("expected a complete clean Git revision")
	}
	runPattern := regexp.MustCompile(`^https://github\.com/` + regexp.QuoteMeta(p.repository) + `/actions/runs/[0-9]+(?:/attempts/[0-9]+)?$`)
	if !runPattern.MatchString(p.runURL) {
		return fmt.Errorf("invalid validation workflow URL")
	}
	return nil
}
func (p *publisher) checkTag() error {
	actual, err := p.gh("api", "repos/"+p.repository+"/commits/"+p.version, "--jq", ".sha")
	if err != nil {
		return err
	}
	if strings.TrimSpace(actual) != p.revision {
		return fmt.Errorf("remote tag %s does not identify %s", p.version, p.revision)
	}
	return nil
}
func (p *publisher) checkRelease(draft bool, files []string) error {
	output, err := p.gh("release", "view", p.version, "--repo", p.repository, "--json", "isDraft,tagName,targetCommitish,assets")
	if err != nil {
		return err
	}
	var remote struct {
		Draft    *bool  `json:"isDraft"`
		Tag      string `json:"tagName"`
		Revision string `json:"targetCommitish"`
		Assets   []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err = json.Unmarshal([]byte(output), &remote); err != nil {
		return fmt.Errorf("remote release JSON: %w", err)
	}
	if remote.Draft == nil || *remote.Draft != draft || remote.Tag != p.version || remote.Revision != p.revision {
		return fmt.Errorf("remote release identity or draft state differs")
	}
	names := make([]string, 0, len(remote.Assets))
	for _, a := range remote.Assets {
		names = append(names, a.Name)
	}
	if !sameNames(names, files) {
		return fmt.Errorf("remote release has missing, duplicate or unexpected assets")
	}
	return nil
}

func (p *publisher) execute() error {
	if err := p.validateEnvironment(); err != nil {
		return err
	}
	p.phase = "validate_local_assets"
	hashes, err := p.validateAssets()
	if err != nil {
		return err
	}
	files := sortedKeys(hashes)
	notes := p.path("docs/releases/" + strings.TrimPrefix(p.version, "v") + ".md")
	if err = regularFile(notes); err != nil {
		return fmt.Errorf("reviewed version-specific release notes are missing or not regular: %w", err)
	}
	notesBytes, err := os.ReadFile(notes)
	if err != nil {
		return err
	}
	notesText := strings.TrimSpace(string(notesBytes))
	if notesText == "" {
		return fmt.Errorf("release notes are empty")
	}
	notesHash := hashBytes(notesBytes)
	p.phase = "check_remote_tag"
	if err = p.checkTag(); err != nil {
		return err
	}
	p.phase = "check_existing_release"
	existing, err := p.gh("api", "repos/"+p.repository+"/releases?per_page=100", "--paginate", "--jq", ".[].tag_name")
	if err != nil {
		return err
	}
	for _, tag := range strings.Split(existing, "\n") {
		if strings.TrimSpace(tag) == p.version {
			return fmt.Errorf("release already exists; refusing to overwrite or resume it")
		}
	}
	if err = os.MkdirAll(p.path("build"), 0755); err != nil {
		return err
	}
	readback, err := os.MkdirTemp(p.path("build"), "release-readback-")
	if err != nil {
		return err
	}
	readbackRelative, err := filepath.Rel(p.root, readback)
	if err != nil {
		return err
	}
	readbackRelative = filepath.ToSlash(readbackRelative)
	rendered, err := os.CreateTemp(p.path("build"), "release-notes-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(rendered.Name())
	_, writeErr := fmt.Fprintf(rendered, "%s\n\nSource revision: `%s`.\nRelease notes source SHA-256: `%s`.\n\n[Validation workflow](%s)\n", notesText, p.revision, notesHash, p.runURL)
	closeErr := rendered.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	notesRelative, err := filepath.Rel(p.root, rendered.Name())
	if err != nil {
		return err
	}
	p.phase = "create_draft"
	_, err = p.gh("release", "create", p.version, "--repo", p.repository, "--verify-tag", "--draft", "--title", "Survex Duplicate Detector "+p.version, "--target", p.revision, "--notes-file", filepath.ToSlash(notesRelative))
	if err != nil {
		return err
	}
	p.created = true
	p.phase = "upload_assets"
	args := []string{"release", "upload", p.version}
	for _, name := range files {
		args = append(args, "dist/"+name)
	}
	args = append(args, "--repo", p.repository)
	if _, err = p.gh(args...); err != nil {
		return err
	}
	p.phase = "verify_remote_draft"
	if err = p.checkRelease(true, files); err != nil {
		return err
	}
	p.phase = "download_assets"
	if _, err = p.gh("release", "download", p.version, "--repo", p.repository, "--dir", readbackRelative); err != nil {
		return err
	}
	p.phase = "verify_downloaded_assets"
	actual, err := fileNames(readback)
	if err != nil {
		return err
	}
	if !sameNames(actual, files) {
		return fmt.Errorf("downloaded release asset set differs")
	}
	for _, name := range files {
		digest, err := hashFile(filepath.Join(readback, name))
		if err != nil {
			return err
		}
		if digest != hashes[name] {
			return fmt.Errorf("downloaded asset checksum mismatch: %s", name)
		}
	}
	p.phase = "recheck_remote_tag"
	if err = p.checkTag(); err != nil {
		return err
	}
	p.phase = "publish_draft"
	p.publicationAttempted = true
	if _, err = p.gh("release", "edit", p.version, "--repo", p.repository, "--draft=false", "--latest"); err != nil {
		return err
	}
	p.published = true
	p.phase = "verify_published_release"
	if err = p.checkRelease(false, files); err != nil {
		return err
	}
	if err = p.checkTag(); err != nil {
		return err
	}
	p.phase = "verified"
	assets := make([]map[string]string, 0, len(files))
	for _, name := range files {
		assets = append(assets, map[string]string{"name": name, "sha256": hashes[name]})
	}
	releaseURL := "https://github.com/" + p.repository + "/releases/tag/" + p.version
	p.evidence = map[string]any{"schema_version": 2, "phase": p.phase, "version": p.version, "revision": p.revision, "run_url": p.runURL, "release_url": releaseURL, "published": true, "notes_source_sha256": notesHash, "readback_directory": readbackRelative, "assets": assets}
	fmt.Fprintln(p.stdout, "Published and verified", releaseURL)
	fmt.Fprintln(p.stdout, "Independent readback retained in", readbackRelative)
	return nil
}

func (p *publisher) saveEvidence() error {
	value := p.evidence
	if value == nil {
		value = map[string]any{}
	}
	state := "not_attempted"
	var published any = false
	if p.published {
		state, published = "confirmed", true
	} else if p.publicationAttempted {
		state, published = "unknown", nil
	}
	value["schema_version"] = 2
	value["phase"] = p.phase
	value["version"] = p.version
	value["revision"] = p.revision
	value["run_url"] = p.runURL
	value["draft_created"] = p.created
	value["publication_attempted"] = p.publicationAttempted
	value["publication_state"] = state
	value["published"] = published
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(p.path("build"), 0755); err != nil {
		return err
	}
	return os.WriteFile(p.path("build/release-publication.json"), append(b, '\n'), 0644)
}
