package main

import (
	"reflect"
	"testing"
)

func mustScheme(t *testing.T, pattern string, panels, clients []string) *NamingScheme {
	t.Helper()
	s, err := NewNamingScheme(pattern, panels, clients)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDefaultScheme(t *testing.T) {
	cfg := DefaultConfig()
	s := mustScheme(t, cfg.HostnamePattern, cfg.PanelRoles, cfg.ClientRoles)

	id, err := s.Parse("pc-room12")
	if err != nil {
		t.Fatal(err)
	}
	if !id.IsClient() || id.Role != "pc" || id.Groups["site"] != "room12" {
		t.Fatalf("bad identity: %+v", id)
	}
	if got := id.CounterpartCandidates(); !reflect.DeepEqual(got, []string{"panel-room12"}) {
		t.Fatalf("candidates = %v", got)
	}

	pid, err := s.Parse("panel-room12")
	if err != nil || !pid.IsPanel() {
		t.Fatalf("panel: %+v %v", pid, err)
	}
	if got := pid.CounterpartCandidates(); !reflect.DeepEqual(got, []string{"pc-room12"}) {
		t.Fatalf("candidates = %v", got)
	}
}

// A scheme with three device kinds: p/n/m + "<site>-<room>-<n>".
func TestLegacySchoolScheme(t *testing.T) {
	s := mustScheme(t, `^(?P<role>[pnm])1234-(?P<site>\d+)-(?P<room>\d+)-(?P<n>\d+)$`,
		[]string{"p"}, []string{"n", "m"})

	panel, err := s.Parse("p1234-0-421-0")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"n1234-0-421-0", "m1234-0-421-0"}
	if got := panel.CounterpartCandidates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("panel candidates = %v, want %v", got, want)
	}

	laptop, _ := s.Parse("m1234-20-5-0")
	if got := laptop.CounterpartCandidates(); !reflect.DeepEqual(got, []string{"p1234-20-5-0"}) {
		t.Fatalf("laptop candidates = %v", got)
	}
	if laptop.Groups["site"] != "20" || laptop.Groups["room"] != "5" {
		t.Fatalf("groups: %v", laptop.Groups)
	}
}

func TestParseErrors(t *testing.T) {
	s := mustScheme(t, `^(?P<role>panel|pc|other)-(?P<x>.+)$`, []string{"panel"}, []string{"pc"})
	for _, h := range []string{"random", "panel", "other-1"} {
		if _, err := s.Parse(h); err == nil {
			t.Errorf("Parse(%q) should fail", h)
		}
	}
}

func TestNewNamingSchemeValidation(t *testing.T) {
	cases := []struct {
		name, pattern string
		p, c          []string
	}{
		{"empty pattern", "", []string{"a"}, []string{"b"}},
		{"bad regexp", "(", []string{"a"}, []string{"b"}},
		{"no role group", `^(a|b)$`, []string{"a"}, []string{"b"}},
		{"no panel roles", `^(?P<role>a|b)$`, nil, []string{"b"}},
		{"no client roles", `^(?P<role>a|b)$`, []string{"a"}, nil},
		{"overlap", `^(?P<role>a|b)$`, []string{"a"}, []string{"a"}},
	}
	for _, tc := range cases {
		if _, err := NewNamingScheme(tc.pattern, tc.p, tc.c); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestRoleInTheMiddleOfName(t *testing.T) {
	s := mustScheme(t, `^(?P<site>[a-z]+)-(?P<role>board|desk)-(?P<n>\d+)$`, []string{"board"}, []string{"desk"})
	id, err := s.Parse("north-desk-7")
	if err != nil {
		t.Fatal(err)
	}
	if got := id.CounterpartCandidates(); !reflect.DeepEqual(got, []string{"north-board-7"}) {
		t.Fatalf("got %v", got)
	}
}

func TestIdentifyForcedRoleAndPeers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Role = "client"
	cfg.PeerHostnames = []string{"10.0.0.5"}
	id, err := Identify(cfg, "weird-name")
	if err != nil {
		t.Fatal(err)
	}
	if !id.IsClient() || !reflect.DeepEqual(id.CounterpartCandidates(), []string{"10.0.0.5"}) {
		t.Fatalf("%+v", id)
	}

	cfg.PeerHostnames = nil
	if _, err := Identify(cfg, "weird-name"); err == nil {
		t.Fatal("forced role without peers and without scheme match must fail")
	}

	cfg = DefaultConfig()
	if _, err := Identify(cfg, "weird-name"); err == nil {
		t.Fatal("auto role with a non-matching hostname must fail")
	}
	cfg.Role = "robot"
	if _, err := Identify(cfg, "pc-1"); err == nil {
		t.Fatal("bad ROLE must fail")
	}
}

func TestPeersOverrideScheme(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PeerHostnames = []string{"special-panel"}
	id, err := Identify(cfg, "pc-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := id.CounterpartCandidates(); !reflect.DeepEqual(got, []string{"special-panel"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a, b  c,a\t,d ")
	if !reflect.DeepEqual(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("got %v", got)
	}
}
