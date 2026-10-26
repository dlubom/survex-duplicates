// Package releasepack builds and independently verifies portable release bundles.
package releasepack

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type Target struct {
	ID      string            `json:"id"`
	GOOS    string            `json:"goos"`
	GOARCH  string            `json:"goarch"`
	Env     map[string]string `json:"env"`
	Kind    string            `json:"kind"`
	Archive string            `json:"archive"`
	Runner  string            `json:"runner"`
}
type Config struct {
	SchemaVersion int      `json:"schema_version"`
	Targets       []Target `json:"targets"`
}
type DistTarget struct {
	GOOS, GOARCH string
	Broken       bool
}

func LoadConfig(root string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(root, "release/targets.json"))
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, nil
}

func (c Config) Validate(dist []DistTarget) error {
	if c.SchemaVersion != 1 || len(c.Targets) == 0 {
		return fmt.Errorf("invalid target manifest schema or empty targets")
	}
	ports := map[string]bool{}
	for _, d := range dist {
		if !d.Broken {
			ports[d.GOOS+"/"+d.GOARCH] = true
		}
	}
	ids := map[string]bool{}
	for _, t := range c.Targets {
		pair := t.GOOS + "/" + t.GOARCH
		if ids[t.ID] || !ports[pair] || t.ID != t.GOOS+"_"+t.GOARCH {
			return fmt.Errorf("duplicate, unsafe or unsupported target %q (%s)", t.ID, pair)
		}
		if t.GOOS != "linux" && t.GOOS != "darwin" && t.GOOS != "windows" {
			return fmt.Errorf("target %s is outside the desktop release scope", t.ID)
		}
		env := map[string]string{}
		switch t.GOARCH {
		case "amd64":
			env["GOAMD64"] = "v1"
		case "arm64":
			env["GOARM64"] = "v8.0"
		default:
			return fmt.Errorf("unsupported desktop architecture %s", t.GOARCH)
		}
		if !reflect.DeepEqual(env, t.Env) {
			return fmt.Errorf("target %s has unreviewed CPU/environment settings", t.ID)
		}
		if t.Kind != "native" || t.Archive != "zip" || t.Runner == "" {
			return fmt.Errorf("invalid package kind, archive or runner for %s", t.ID)
		}
		ids[t.ID] = true
	}
	return nil
}

func LoadValidated(root string) (Config, error) {
	c, err := LoadConfig(root)
	if err != nil {
		return c, err
	}
	if err = checkToolchain(root); err != nil {
		return c, err
	}
	cmd := exec.Command("go", "tool", "dist", "list", "-json")
	cmd.Env = buildEnv(Target{})
	b, err := cmd.Output()
	if err != nil {
		return c, fmt.Errorf("go tool dist list: %w", err)
	}
	var dist []DistTarget
	if err = json.Unmarshal(b, &dist); err != nil {
		return c, err
	}
	if err = c.Validate(dist); err != nil {
		return c, err
	}
	for _, osName := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			if _, err = c.Find(osName + "_" + arch); err != nil {
				return c, err
			}
		}
	}
	if len(c.Targets) != 6 {
		return c, fmt.Errorf("release manifest must contain exactly the six desktop targets")
	}
	return c, nil
}

func checkToolchain(root string) error {
	b, err := os.ReadFile(filepath.Join(root, ".go-version"))
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "env", "GOVERSION")
	cmd.Env = buildEnv(Target{})
	got, err := cmd.Output()
	if err != nil {
		return err
	}
	want := "go" + strings.TrimSpace(string(b))
	if strings.TrimSpace(string(got)) != want {
		return fmt.Errorf("release requires %s, got %s", want, strings.TrimSpace(string(got)))
	}
	return nil
}

func buildEnv(t Target) []string {
	blocked := map[string]bool{"GOENV": true, "GOTOOLCHAIN": true, "GOFLAGS": true, "GOOS": true, "GOARCH": true, "CGO_ENABLED": true, "GOEXPERIMENT": true, "GOWORK": true, "GODEBUG": true, "GO_EXTLINK_ENABLED": true, "GOROOT": true}
	for _, k := range []string{"GO386", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM"} {
		blocked[k] = true
	}
	env := []string{}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if !blocked[strings.ToUpper(k)] {
			env = append(env, v)
		}
	}
	env = append(env, "GOENV=off", "GOTOOLCHAIN=local", "GOFLAGS=", "CGO_ENABLED=0", "GOWORK=off", "GOEXPERIMENT=", "GODEBUG=")
	if t.GOOS != "" {
		env = append(env, "GOOS="+t.GOOS, "GOARCH="+t.GOARCH)
	}
	keys := []string{}
	for k := range t.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+t.Env[k])
	}
	return env
}

func (c Config) Find(id string) (Target, error) {
	for _, t := range c.Targets {
		if t.ID == id {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("unknown target %q", id)
}
func (t Target) Executable() string {
	if t.GOOS == "windows" {
		return "survex-duplicates.exe"
	}
	return "survex-duplicates"
}
func ArchiveName(version string, t Target) string {
	return "survex-duplicates_" + version + "_" + t.ID + "." + t.Archive
}
