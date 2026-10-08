package main

import (
	"errors"
	"net"
	"reflect"
	"testing"
)

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return &net.IPNet{IP: ip, Mask: n.Mask}
}

func ipsOf(c []candidate) []string {
	var out []string
	for _, x := range c {
		out = append(out, x.IP)
	}
	return out
}

func TestRankPutsOnLinkFirstKeepsOrderOtherwise(t *testing.T) {
	local := []*net.IPNet{cidr(t, "192.168.1.5/24"), cidr(t, "10.20.0.5/16")}
	got := rankCandidates([]string{"172.16.0.9", "10.20.4.4", "8.8.8.8", "192.168.1.77", "10.20.4.4"}, local, nil)
	want := []string{"10.20.4.4", "192.168.1.77", "172.16.0.9", "8.8.8.8"}
	if !reflect.DeepEqual(ipsOf(got), want) {
		t.Fatalf("got %v want %v", ipsOf(got), want)
	}
	if !got[0].OnLink || got[0].Via != "10.20.0.5" || got[2].OnLink {
		t.Fatalf("locality flags wrong: %+v", got)
	}
}

func TestRankDropsOwnAndInvalidAddresses(t *testing.T) {
	own := map[string]bool{"172.17.0.1": true}
	got := rankCandidates([]string{"172.17.0.1", "junk", "", "10.0.0.2"}, nil, own)
	if !reflect.DeepEqual(ipsOf(got), []string{"10.0.0.2"}) {
		t.Fatalf("%v", ipsOf(got))
	}
}

func TestSameAndDifferentSubnetLayouts(t *testing.T) {
	// laptop and panel in one subnet
	same := rankCandidates([]string{"10.1.1.20"}, []*net.IPNet{cidr(t, "10.1.1.10/24")}, nil)
	if !same[0].OnLink {
		t.Fatal("expected on-link")
	}
	// laptop and panel in different subnets of the same site
	diff := rankCandidates([]string{"10.173.1.20"}, []*net.IPNet{cidr(t, "172.26.1.10/16")}, nil)
	if diff[0].OnLink {
		t.Fatal("expected routed")
	}
}

func TestPickReachable(t *testing.T) {
	cands := []candidate{{IP: "a"}, {IP: "b"}, {IP: "c"}}
	got, err := pickReachable(cands, func(ip string) error {
		if ip == "b" {
			return nil
		}
		return errors.New("down")
	})
	if err != nil || got.IP != "b" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := pickReachable(cands, func(string) error { return errors.New("down") }); err == nil {
		t.Fatal("expected error")
	}
	if _, err := pickReachable(nil, func(string) error { return nil }); err == nil {
		t.Fatal("expected error for no candidates")
	}
}

func TestIgnoreInterfacePatterns(t *testing.T) {
	for _, n := range []string{"docker0", "br-1a2b3c", "veth12", "virbr0", "lo"} {
		if !matchAny(defaultIgnoreInterfaces, n) {
			t.Errorf("%s should be ignored", n)
		}
	}
	for _, n := range []string{"eth0", "enp3s0", "wlan0", "eno1"} {
		if matchAny(defaultIgnoreInterfaces, n) {
			t.Errorf("%s must not be ignored", n)
		}
	}
}

func TestLocalNetsDoesNotPanic(t *testing.T) {
	_ = reportAddrs(localNets(defaultIgnoreInterfaces))
	_ = allOwnIPs()
}
