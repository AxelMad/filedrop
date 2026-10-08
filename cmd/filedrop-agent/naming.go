package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Naming scheme.
//
// filedrop pairs an interactive panel with the computer standing next to it
// purely by hostname. The scheme is configurable: HOSTNAME_PATTERN is a Go
// (RE2) regular expression with a mandatory named group "role"; PANEL_ROLES
// and CLIENT_ROLES list the values of that group that mean "panel" (the
// receiving side) and "client" (laptop / desktop / all-in-one, the sending
// side).
//
// The counterpart of a machine is its own hostname with ONLY the "role" part
// replaced: with
//
//	HOSTNAME_PATTERN=^(?P<role>panel|pc)-(?P<room>.+)$
//	PANEL_ROLES=panel
//	CLIENT_ROLES=pc
//
// the machine "pc-room12" talks to "panel-room12" and vice versa. Every other
// part of the name (site, room, number ...) must be identical for a pair.
// Machines whose names do not follow any scheme can be paired explicitly with
// PEER_HOSTNAMES.

// Kind is the side of the transfer a machine plays.
type Kind int

const (
	KindUnknown Kind = iota
	KindPanel        // receives files
	KindClient       // sends files
)

func (k Kind) String() string {
	switch k {
	case KindPanel:
		return "panel"
	case KindClient:
		return "client"
	default:
		return "unknown"
	}
}

// NamingScheme turns hostnames into Identity values.
type NamingScheme struct {
	pattern     string
	re          *regexp.Regexp
	roleGroup   int
	panelRoles  []string
	clientRoles []string
}

// NewNamingScheme validates the pattern and role lists.
func NewNamingScheme(pattern string, panelRoles, clientRoles []string) (*NamingScheme, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("HOSTNAME_PATTERN is empty")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("HOSTNAME_PATTERN is not a valid regular expression: %w", err)
	}
	idx := re.SubexpIndex("role")
	if idx < 0 {
		return nil, fmt.Errorf("HOSTNAME_PATTERN must contain a named group (?P<role>...)")
	}
	if len(panelRoles) == 0 {
		return nil, fmt.Errorf("PANEL_ROLES is empty")
	}
	if len(clientRoles) == 0 {
		return nil, fmt.Errorf("CLIENT_ROLES is empty")
	}
	for _, p := range panelRoles {
		for _, c := range clientRoles {
			if p == c {
				return nil, fmt.Errorf("role %q is listed both in PANEL_ROLES and CLIENT_ROLES", p)
			}
		}
	}
	return &NamingScheme{
		pattern:     pattern,
		re:          re,
		roleGroup:   idx,
		panelRoles:  panelRoles,
		clientRoles: clientRoles,
	}, nil
}

// Identity describes this (or any) machine according to the scheme.
type Identity struct {
	Raw      string            // the hostname as given
	Role     string            // value of the "role" group, e.g. "panel"
	Kind     Kind              // panel or client
	Groups   map[string]string // all named groups of the pattern
	roleSpan [2]int            // byte range of the role inside Raw
	scheme   *NamingScheme
	Peers    []string // explicit counterparts (PEER_HOSTNAMES); override the scheme
}

func (id Identity) IsPanel() bool  { return id.Kind == KindPanel }
func (id Identity) IsClient() bool { return id.Kind == KindClient }

// Parse splits a hostname according to the scheme.
func (s *NamingScheme) Parse(hostname string) (Identity, error) {
	m := s.re.FindStringSubmatchIndex(hostname)
	if m == nil {
		return Identity{}, fmt.Errorf("hostname %q does not match HOSTNAME_PATTERN %s", hostname, s.pattern)
	}
	start, end := m[2*s.roleGroup], m[2*s.roleGroup+1]
	if start < 0 {
		return Identity{}, fmt.Errorf("hostname %q matches HOSTNAME_PATTERN but the \"role\" group is empty", hostname)
	}
	role := hostname[start:end]

	kind := KindUnknown
	if contains(s.panelRoles, role) {
		kind = KindPanel
	} else if contains(s.clientRoles, role) {
		kind = KindClient
	} else {
		return Identity{}, fmt.Errorf("role %q of hostname %q is neither in PANEL_ROLES (%s) nor in CLIENT_ROLES (%s)",
			role, hostname, strings.Join(s.panelRoles, ","), strings.Join(s.clientRoles, ","))
	}

	groups := map[string]string{}
	for i, name := range s.re.SubexpNames() {
		if name != "" && m[2*i] >= 0 {
			groups[name] = hostname[m[2*i]:m[2*i+1]]
		}
	}
	return Identity{
		Raw:      hostname,
		Role:     role,
		Kind:     kind,
		Groups:   groups,
		roleSpan: [2]int{start, end},
		scheme:   s,
	}, nil
}

// CounterpartCandidates returns the hostnames this machine may exchange files
// with. A panel may be paired with several client roles (a room has either a
// laptop or an all-in-one, so both names are tried); a client is paired with
// the panel role(s).
func (id Identity) CounterpartCandidates() []string {
	if len(id.Peers) > 0 {
		return id.Peers
	}
	if id.scheme == nil {
		return nil
	}
	var roles []string
	switch id.Kind {
	case KindPanel:
		roles = id.scheme.clientRoles
	case KindClient:
		roles = id.scheme.panelRoles
	default:
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range roles {
		h := id.Raw[:id.roleSpan[0]] + r + id.Raw[id.roleSpan[1]:]
		if h == id.Raw || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// Identify determines the identity of this machine for the given config:
// ROLE=auto uses the naming scheme; ROLE=panel|client forces the side (the
// hostname then need not follow any scheme, but PEER_HOSTNAMES must say whom
// to talk to). PEER_HOSTNAMES, when set, always overrides the derived
// counterpart.
func Identify(cfg Config, hostname string) (Identity, error) {
	scheme, err := NewNamingScheme(cfg.HostnamePattern, cfg.PanelRoles, cfg.ClientRoles)
	if err != nil {
		if cfg.Role == "auto" {
			return Identity{}, err
		}
		scheme = nil
	}

	var id Identity
	var perr error
	if scheme != nil {
		id, perr = scheme.Parse(hostname)
	} else {
		perr = err
	}

	switch cfg.Role {
	case "auto":
		if perr != nil {
			return Identity{}, perr
		}
	case "panel", "client":
		kind := KindPanel
		if cfg.Role == "client" {
			kind = KindClient
		}
		if perr != nil {
			if len(cfg.PeerHostnames) == 0 {
				return Identity{}, fmt.Errorf("ROLE=%s is forced and the hostname does not follow the naming scheme (%v): set PEER_HOSTNAMES", cfg.Role, perr)
			}
			id = Identity{Raw: hostname}
		}
		id.Kind = kind
	default:
		return Identity{}, fmt.Errorf("ROLE must be auto, panel or client, got %q", cfg.Role)
	}
	id.Peers = cfg.PeerHostnames
	return id, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// splitList splits "a, b c,a" into ["a" "b" "c"] (commas and/or spaces,
// duplicates removed, order kept).
func splitList(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}
