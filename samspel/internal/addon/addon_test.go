// Package addon only holds tests. Home Assistant silently hides an add-on whose config.yaml it dislikes
// (version 0.6.5 vanished from the store that way), so the files it reads are checked here.
package addon

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// root is the add-on directory, two levels up from this package.
const root = "../.."

func readConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.yaml is not valid YAML: %v", err)
	}
	return cfg
}

func str(t *testing.T, cfg map[string]any, key string) string {
	t.Helper()
	s, ok := cfg[key].(string)
	if !ok || s == "" {
		t.Fatalf("config.yaml needs a text value for %q, got %v", key, cfg[key])
	}
	return s
}

func TestRequiredFields(t *testing.T) {
	cfg := readConfig(t)
	for _, key := range []string{"name", "version", "slug", "description"} {
		str(t, cfg, key)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(str(t, cfg, "version")) {
		t.Errorf("version %q is not major.minor.patch", cfg["version"])
	}
	if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(str(t, cfg, "slug")) {
		t.Errorf("slug %q must be lowercase letters, digits and underscores", cfg["slug"])
	}
}

func TestArchitecturesAreCurrentOnes(t *testing.T) {
	archs, _ := readConfig(t)["arch"].([]any)
	if len(archs) == 0 {
		t.Fatal("arch must list at least one architecture")
	}
	for _, a := range archs {
		if a != "aarch64" && a != "amd64" { // armhf, armv7 and i386 are deprecated
			t.Errorf("unsupported architecture %v", a)
		}
	}
}

// The pattern is the one Supervisor reported when it rejected the config:
// does not match regular expression ^(?:https?|\[PROTO:\w+\]):\/\/\[HOST\]:\[PORT:\d+\].*$
func TestWebUIMatchesSupervisorsPattern(t *testing.T) {
	cfg := readConfig(t)
	webui, ok := cfg["webui"].(string)
	if !ok {
		t.Skip("no webui")
	}
	re := regexp.MustCompile(`^(?:https?|\[PROTO:\w+\]):\/\/\[HOST\]:\[PORT:\d+\].*$`)
	if !re.MatchString(webui) {
		t.Fatalf("webui %q would be rejected by Supervisor", webui)
	}
	// The port in the link has to be one the add-on declares.
	port := regexp.MustCompile(`\[PORT:(\d+)\]`).FindStringSubmatch(webui)[1]
	ports, _ := cfg["ports"].(map[string]any)
	if _, ok := ports[port+"/tcp"]; !ok {
		t.Errorf("webui uses port %s but ports declares %v", port, ports)
	}
}

func TestPortKeysLookLikePorts(t *testing.T) {
	ports, _ := readConfig(t)["ports"].(map[string]any)
	for key := range ports {
		if !regexp.MustCompile(`^\d+/(tcp|udp)$`).MatchString(key) {
			t.Errorf("port %q should look like 8080/tcp", key)
		}
	}
}

func TestChangelogMentionsTheCurrentVersion(t *testing.T) {
	version := str(t, readConfig(t), "version")
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	changelog := string(data)
	if !strings.Contains(changelog, "\n## "+version+"\n") {
		t.Fatalf("CHANGELOG.md has no entry for version %s", version)
	}
	// Newest first: the current version must be the top entry.
	if first := regexp.MustCompile(`(?m)^## (\S+)$`).FindStringSubmatch(changelog); first == nil || first[1] != version {
		t.Errorf("the top entry in CHANGELOG.md should be %s, found %v", version, first)
	}
}

func TestDockerfileFilesExist(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "COPY" || strings.HasPrefix(fields[1], "--from") {
			continue
		}
		for _, src := range fields[1 : len(fields)-1] {
			if src == "." {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, src)); err != nil {
				t.Errorf("Dockerfile copies %q, which does not exist", src)
			}
		}
	}
}

func TestStartScriptIsExecutable(t *testing.T) {
	// The Dockerfile runs /run.sh directly, so the executable bit has to survive in git.
	info, err := os.Stat(filepath.Join(root, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("run.sh is not executable")
	}
}

func TestURLIsAnAbsoluteWebAddress(t *testing.T) {
	raw, ok := readConfig(t)["url"].(string)
	if !ok {
		t.Skip("no url")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		t.Errorf("url %q must be an absolute http or https address", raw)
	}
}
