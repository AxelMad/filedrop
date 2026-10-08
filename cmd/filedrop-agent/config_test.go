package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConf(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent.conf")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfigMissingFileGivesDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatal("expected defaults")
	}
}

func TestLoadConfig(t *testing.T) {
	p := writeConf(t, `
# comment
ROLE = Auto
HOSTNAME_PATTERN=^(?P<role>[pnm])1-(?P<room>\d+)$
PANEL_ROLES=p
CLIENT_ROLES="n, m"
PEER_HOSTNAMES=a b
RESOLVE_MODE=directory
DIRECTORY_URL=http://10.0.0.1:8781/, http://172.16.0.1:8781
DIRECTORY_TOKEN='secret'
SSH_PORT=2222
SETTLE_CHECKS=0
SHARED_FOLDER_NAME=Общая папка
`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Role != "auto" || cfg.HostnamePattern != `^(?P<role>[pnm])1-(?P<room>\d+)$` {
		t.Fatalf("%+v", cfg)
	}
	if !reflect.DeepEqual(cfg.ClientRoles, []string{"n", "m"}) || !reflect.DeepEqual(cfg.PeerHostnames, []string{"a", "b"}) {
		t.Fatalf("%+v", cfg)
	}
	if got := cfg.DirectoryURLs(); !reflect.DeepEqual(got, []string{"http://10.0.0.1:8781", "http://172.16.0.1:8781"}) {
		t.Fatalf("urls %v", got)
	}
	if cfg.DirectoryToken != "secret" || cfg.SSHPort != 2222 || cfg.SettleChecks != 1 || cfg.SharedFolderName != "Общая папка" {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	for _, body := range []string{"NOPE=1", "just text", "SSH_PORT=abc"} {
		if _, err := LoadConfig(writeConf(t, body)); err == nil {
			t.Errorf("%q should fail", body)
		} else if !strings.Contains(err.Error(), "agent.conf:1") {
			t.Errorf("error should name the line: %v", err)
		}
	}
}

func TestSharedDir(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SharedDir(KindPanel) != cfg.InboxDir || cfg.SharedDir(KindClient) != cfg.OutboxDir {
		t.Fatal("wrong shared dir")
	}
}

// The shipped agent.conf must parse and must equal the built-in defaults,
// otherwise the documentation and the code drift apart.
func TestShippedDefaultConfigMatchesDefaults(t *testing.T) {
	cfg, err := LoadConfig("../../packaging/common/agent.conf.default")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, DefaultConfig()) {
		t.Fatalf("agent.conf.default differs from DefaultConfig():\n got  %+v\n want %+v", cfg, DefaultConfig())
	}
}

// Every shipped example must parse, and its naming scheme must be valid.
func TestExamplesParse(t *testing.T) {
	files, _ := filepath.Glob("../../examples/agent.conf.*")
	more, _ := filepath.Glob("../../examples/*/agent.conf")
	files = append(files, more...)
	if len(files) < 5 {
		t.Fatalf("examples not found: %v", files)
	}
	for _, f := range files {
		cfg, err := LoadConfig(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if _, err := NewNamingScheme(cfg.HostnamePattern, cfg.PanelRoles, cfg.ClientRoles); err != nil && cfg.Role == "auto" {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestStructuredNamesExample(t *testing.T) {
	cfg, err := LoadConfig("../../examples/structured-names/agent.conf")
	if err != nil {
		t.Fatal(err)
	}
	id, err := Identify(cfg, "p1234-0-421-0")
	if err != nil || !id.IsPanel() {
		t.Fatalf("%+v %v", id, err)
	}
	want := []string{"n1234-0-421-0", "m1234-0-421-0"}
	if got := id.CounterpartCandidates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if cfg.SharedFolderName != "Общая папка" {
		t.Fatalf("%q", cfg.SharedFolderName)
	}
}
