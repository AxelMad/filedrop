package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Resolver finds the current IP address of a counterpart by hostname.
//
//   - dns:       the system resolver (DNS, mDNS/avahi, /etc/hosts)
//   - directory: an HTTP lookup service the agents register with (for DHCP
//     networks where hostnames are not in DNS)
//   - static:    a local "hostname<whitespace>ip" file
type Resolver interface {
	// Resolve returns ALL known addresses of the host (at least one).
	// Choosing among them (same subnet first, reachable) is up to the caller.
	Resolve(hostname string) (ips []string, err error)
}

// ---- dns mode ----

// DNSResolver uses the system resolver and prefers IPv4 addresses.
type DNSResolver struct{}

func (DNSResolver) Resolve(hostname string) ([]string, error) {
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", hostname, err)
	}
	var v4, other []string
	for _, a := range addrs {
		if a.To4() != nil {
			v4 = append(v4, a.To4().String())
		} else {
			other = append(other, a.String())
		}
	}
	out := append(v4, other...)
	if len(out) == 0 {
		return nil, fmt.Errorf("resolving %s: no addresses", hostname)
	}
	return out, nil
}

// ---- directory mode ----

var errUnknownHost = errors.New("unknown to the directory")

// DirectoryResolver talks to one or several URLs of the same directory
// service. Several URLs are for a server that has more than one address (one per
// subnet): URLs are tried starting with the last one that worked, so a machine
// quickly settles on the address that is reachable from its own subnet, and the
// same list can be used on every machine.
type DirectoryResolver struct {
	URLs   []string
	Token  string
	Client *http.Client

	mu     sync.Mutex
	last   int
	warned bool
}

func NewDirectoryResolver(urls []string, token string) *DirectoryResolver {
	clean := make([]string, 0, len(urls))
	for _, u := range urls {
		clean = append(clean, strings.TrimRight(u, "/"))
	}
	return &DirectoryResolver{
		URLs:   clean,
		Token:  token,
		Client: &http.Client{Timeout: 5 * time.Second},
	}
}

type resolveResponse struct {
	IP        string   `json:"ip"`  // the address the directory saw the host connect from
	IPs       []string `json:"ips"` // every known address (observed one first)
	AgeSecond int      `json:"age_seconds"`
}

func (d *DirectoryResolver) order() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.URLs)
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, (d.last+i)%n)
	}
	return out
}

func (d *DirectoryResolver) markGood(i int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.last = i
	if i != 0 && !d.warned {
		d.warned = true
		log.Printf("directory URL %s is not reachable, using %s instead "+
			"(harmless; put the working address first in DIRECTORY_URL to avoid the short delay)",
			d.URLs[0], d.URLs[i])
	}
}

// try runs fn against the URLs in order until one answers. An answer of
// errUnknownHost is authoritative (all URLs are the same server) and stops
// the search.
func (d *DirectoryResolver) try(fn func(base string) error) error {
	if len(d.URLs) == 0 {
		return errors.New("no directory URL configured")
	}
	var failures []string
	for _, i := range d.order() {
		err := fn(d.URLs[i])
		if err == nil || errors.Is(err, errUnknownHost) {
			d.markGood(i)
			return err
		}
		failures = append(failures, err.Error())
	}
	return fmt.Errorf("no directory URL answered: %s", strings.Join(failures, "; "))
}

func (d *DirectoryResolver) Resolve(hostname string) ([]string, error) {
	var ips []string
	err := d.try(func(base string) error {
		req, err := http.NewRequest("GET", base+"/resolve?name="+url.QueryEscape(hostname), nil)
		if err != nil {
			return err
		}
		if d.Token != "" {
			req.Header.Set("X-Filedrop-Token", d.Token)
		}
		resp, err := d.Client.Do(req)
		if err != nil {
			return fmt.Errorf("directory %s: %w", base, err)
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
		case http.StatusNotFound:
			return fmt.Errorf("%s: %w (has not registered yet)", hostname, errUnknownHost)
		default:
			return fmt.Errorf("directory %s returned %s", base, resp.Status)
		}
		var rr resolveResponse
		if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
			return fmt.Errorf("directory %s: bad response: %w", base, err)
		}
		ips = rr.IPs
		if len(ips) == 0 && rr.IP != "" { // older directory servers
			ips = []string{rr.IP}
		}
		if len(ips) == 0 {
			return fmt.Errorf("directory %s returned no address for %s", base, hostname)
		}
		return nil
	})
	return ips, err
}

type registerBody struct {
	Hostname string   `json:"hostname"`
	Addrs    []string `json:"addrs,omitempty"`
}

// Register tells the directory that this machine is alive. The directory
// records the IP address the request came from AND the addresses the agent
// lists (all its local subnets, "192.168.1.5/24"), so that a counterpart can
// pick the address that is on its own subnet.
func (d *DirectoryResolver) Register(hostname string, addrs []string) error {
	return d.try(func(base string) error {
		payload, err := json.Marshal(registerBody{Hostname: hostname, Addrs: addrs})
		if err != nil {
			return err
		}
		body := strings.NewReader(string(payload))
		req, err := http.NewRequest("POST", base+"/register", body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if d.Token != "" {
			req.Header.Set("X-Filedrop-Token", d.Token)
		}
		resp, err := d.Client.Do(req)
		if err != nil {
			return fmt.Errorf("directory %s: %w", base, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("directory %s rejected the registration: %s", base, resp.Status)
		}
		return nil
	})
}

// ---- static mode ----

// StaticResolver reads a "hostname<whitespace>ip" file on every lookup, so a
// file updated by the administrator is picked up without restarting the agent.
type StaticResolver struct {
	Path string
}

func NewStaticResolver(path string) *StaticResolver {
	return &StaticResolver{Path: path}
}

func (s *StaticResolver) Resolve(hostname string) ([]string, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, fmt.Errorf("opening static table %s: %w", s.Path, err)
	}
	defer f.Close()

	// "hostname ip [ip ...]"; a host may be listed on several lines too
	var ips []string

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == hostname {
			ips = append(ips, fields[1:]...)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("hostname %s not found in %s", hostname, s.Path)
	}
	return ips, nil
}

// resolveCounterpart tries every candidate (a panel may be paired with a
// laptop or an all-in-one) and returns the first one that resolves.
func resolveCounterpart(r Resolver, candidates []string) (hostname string, ips []string, err error) {
	if len(candidates) == 0 {
		return "", nil, errors.New("no counterpart hostname candidates (check HOSTNAME_PATTERN / PEER_HOSTNAMES)")
	}
	var lastErr error
	for _, c := range candidates {
		ips, err := r.Resolve(c)
		if err == nil {
			return c, ips, nil
		}
		lastErr = err
	}
	return "", nil, fmt.Errorf("none of %v could be resolved: %w", candidates, lastErr)
}
