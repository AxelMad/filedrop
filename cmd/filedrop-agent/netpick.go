package main

import (
	"fmt"
	"net"
	"path"
	"sort"
	"strings"
	"time"
)

// Subnet awareness.
//
// A laptop and the panel of the same room can be in the same subnet or in
// different (routed) subnets, and the same site may mix both layouts; a host
// may also have several addresses (wired + Wi-Fi). So an agent never assumes:
//
//  1. it reports ALL its usable local addresses to the directory,
//  2. it gets ALL addresses of the counterpart back,
//  3. it puts the ones that are on one of its own subnets first (no router
//     involved, fastest and most reliable), then the rest,
//  4. and it only connects to an address whose SSH port actually answers.

// defaultIgnoreInterfaces are interfaces whose addresses are never reported or
// used for locality: container/VM bridges exist on many machines with the same
// addresses (docker0 is 172.17.0.1 everywhere) and would look like a shared subnet.
var defaultIgnoreInterfaces = []string{
	"lo", "docker*", "br-*", "veth*", "virbr*", "lxcbr*", "lxdbr*",
	"cni*", "flannel*", "cali*", "podman*", "vmnet*", "vboxnet*",
}

// localNets returns the usable IPv4 networks of this machine (one entry per
// address, IP = the machine's own address, Mask = the subnet mask).
func localNets(ignore []string) []*net.IPNet {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []*net.IPNet
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if matchAny(ignore, ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
				continue
			}
			out = append(out, &net.IPNet{IP: ip4, Mask: ipn.Mask})
		}
	}
	return out
}

// allOwnIPs is every address of this machine, ignored interfaces included:
// the agent must never try to "send" to itself.
func allOwnIPs() map[string]bool {
	own := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return own
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			own[ipn.IP.String()] = true
		}
	}
	return own
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// reportAddrs formats the networks for the directory ("192.168.1.5/24").
func reportAddrs(nets []*net.IPNet) []string {
	out := make([]string, 0, len(nets))
	for _, n := range nets {
		out = append(out, n.String())
	}
	return out
}

// candidate is one address of the counterpart together with its locality.
type candidate struct {
	IP     string
	OnLink bool   // inside one of our own subnets
	Via    string // our address on that subnet (when OnLink)
}

func (c candidate) String() string {
	if c.OnLink {
		return fmt.Sprintf("%s (same subnet as %s)", c.IP, c.Via)
	}
	return fmt.Sprintf("%s (other subnet, routed)", c.IP)
}

// rankCandidates drops duplicates, invalid entries and our own addresses and
// puts on-link addresses first; the original order is kept inside each group.
func rankCandidates(ips []string, local []*net.IPNet, own map[string]bool) []candidate {
	seen := map[string]bool{}
	var onLink, routed []candidate
	for _, s := range ips {
		s = strings.TrimSpace(s)
		ip := net.ParseIP(s)
		if ip == nil || seen[s] || own[ip.String()] {
			continue
		}
		seen[s] = true
		c := candidate{IP: s}
		for _, n := range local {
			if n.Contains(ip) {
				c.OnLink, c.Via = true, n.IP.String()
				break
			}
		}
		if c.OnLink {
			onLink = append(onLink, c)
		} else {
			routed = append(routed, c)
		}
	}
	return append(onLink, routed...)
}

// probeTCP reports whether ip:port accepts a connection.
func probeTCP(ip string, port int, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, fmt.Sprint(port)), timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

// pickReachable returns the first candidate whose port answers. When none
// answers it returns all errors.
func pickReachable(cands []candidate, probe func(ip string) error) (candidate, error) {
	if len(cands) == 0 {
		return candidate{}, fmt.Errorf("no usable address of the counterpart")
	}
	var errs []string
	for _, c := range cands {
		if err := probe(c.IP); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", c.IP, err))
			continue
		}
		return c, nil
	}
	sort.Strings(errs)
	return candidate{}, fmt.Errorf("SSH port not reachable on any address (%s)", strings.Join(errs, "; "))
}
