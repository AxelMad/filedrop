// filedrop-directory is a tiny "hostname -> current IP" lookup service for
// networks where machines get their address by DHCP without a reservation and
// are not registered in DNS. Agents report themselves to it (RESOLVE_MODE=
// directory) and look their counterpart up in it.
//
// It takes no part in the file transfer - it only helps the agents find each
// other. Files go directly between the panel and the computer.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

type record struct {
	IP       string    `json:"ip"`              // the address the host connected from
	Addrs    []string  `json:"addrs,omitempty"` // addresses the host reported ("192.168.1.5/24")
	LastSeen time.Time `json:"last_seen"`
}

// ips lists every known address of the host: the observed one first, then the
// reported ones. A host in several subnets (cable + Wi-Fi) or reachable via
// different networks has several; the asking agent picks the one that is on its
// own subnet.
func (r record) ips() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(r.IP)
	for _, a := range r.Addrs {
		if ip, _, err := net.ParseCIDR(a); err == nil {
			add(ip.String())
		} else if ip := net.ParseIP(a); ip != nil {
			add(ip.String())
		}
	}
	return out
}

// cleanAddrs keeps plausible reported addresses only.
func cleanAddrs(in []string) []string {
	var out []string
	for _, a := range in {
		var ip net.IP
		if p, _, err := net.ParseCIDR(a); err == nil {
			ip = p
		} else {
			ip = net.ParseIP(a)
		}
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
			continue
		}
		out = append(out, a)
		if len(out) == 16 {
			break
		}
	}
	return out
}

type store struct {
	mu   sync.Mutex
	data map[string]record
	path string // periodic snapshot file (survives restarts)
}

func newStore(snapshotPath string) *store {
	s := &store{data: map[string]record{}, path: snapshotPath}
	s.loadSnapshot()
	return s
}

func (s *store) loadSnapshot() {
	if s.path == "" {
		return
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return // no snapshot is fine: the directory fills up from registrations
	}
	var data map[string]record
	if err := json.Unmarshal(b, &data); err != nil {
		log.Printf("cannot parse snapshot %s: %v", s.path, err)
		return
	}
	s.mu.Lock()
	s.data = data
	s.mu.Unlock()
	log.Printf("snapshot loaded: %d records", len(data))
}

func (s *store) saveSnapshot() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	b, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.Unlock()
	if err != nil {
		log.Printf("cannot serialize snapshot: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Printf("cannot write snapshot %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("cannot move the snapshot to %s: %v", s.path, err)
	}
}

func (s *store) set(hostname, ip string, addrs []string) {
	s.mu.Lock()
	s.data[hostname] = record{IP: ip, Addrs: addrs, LastSeen: time.Now()}
	s.mu.Unlock()
}

func (s *store) get(hostname string) (record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[hostname]
	return r, ok
}

func (s *store) all() map[string]record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]record, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func checkToken(want string, r *http.Request) bool {
	if want == "" {
		return true // no token configured: open mode (tests only)
	}
	got := r.Header.Get("X-Filedrop-Token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

type registerRequest struct {
	Hostname string   `json:"hostname"`
	Addrs    []string `json:"addrs"` // optional; older agents send only the hostname
}

// newMux builds the HTTP handler (separate from main so it can be tested).
func newMux(s *store, token string, staleAfter time.Duration) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		if !checkToken(token, r) {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		var req registerRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Hostname == "" {
			http.Error(w, `expected JSON {"hostname":"..."}`, http.StatusBadRequest)
			return
		}
		ip := remoteHost(r)
		s.set(req.Hostname, ip, cleanAddrs(req.Addrs))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"hostname":%q,"ip":%q}`, req.Hostname, ip)
	})

	mux.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(token, r) {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "specify ?name=hostname", http.StatusBadRequest)
			return
		}
		rec, ok := s.get(name)
		if !ok {
			http.Error(w, "unknown", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			IP        string   `json:"ip"`
			IPs       []string `json:"ips"`
			AgeSecond int      `json:"age_seconds"`
		}{rec.IP, rec.ips(), int(time.Since(rec.LastSeen).Seconds())})
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(token, r) {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		all := s.all()
		names := make([]string, 0, len(all))
		for k := range all {
			names = append(names, k)
		}
		sort.Strings(names)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><head><meta charset='utf-8'><title>filedrop-directory</title></head><body>")
		fmt.Fprintf(w, "<h1>filedrop-directory %s</h1><p>Records: %d</p>", html.EscapeString(version), len(names))
		fmt.Fprintf(w, "<table border=1 cellpadding=6 cellspacing=0><tr><th>hostname</th><th>seen from</th><th>reported addresses</th><th>last seen</th><th>status</th></tr>")
		for _, name := range names {
			rec := all[name]
			age := time.Since(rec.LastSeen)
			status := "alive"
			if age > staleAfter {
				status = "silent for a long time"
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s ago</td><td>%s</td></tr>",
				html.EscapeString(name), html.EscapeString(rec.IP),
				html.EscapeString(strings.Join(rec.Addrs, ", ")), age.Round(time.Second), status)
		}
		fmt.Fprintf(w, "</table></body></html>")
	})
	return mux
}

func main() {
	addr := flag.String("addr", ":8781", "listen address, e.g. :8781")
	token := flag.String("token", os.Getenv("FILEDROP_TOKEN"), "shared secret (X-Filedrop-Token); can also be set via FILEDROP_TOKEN")
	snapshot := flag.String("snapshot", "/var/lib/filedrop-directory/state.json", "state file kept between restarts")
	staleAfter := flag.Duration("stale-after", 5*time.Minute, "age after which /status marks a record as stale")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("filedrop-directory", version)
		return
	}
	if *token == "" {
		log.Printf("WARNING: no token set (-token / FILEDROP_TOKEN) - the directory accepts requests without any check")
	}
	if err := os.MkdirAll(filepath.Dir(*snapshot), 0755); err != nil {
		log.Printf("cannot create the snapshot directory: %v", err)
	}

	s := newStore(*snapshot)
	mux := newMux(s, *token, *staleAfter)

	// Periodic snapshot so that a restart does not lose the whole picture
	// (agents re-register on their own, but not instantly).
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for range t.C {
			s.saveSnapshot()
		}
	}()

	log.Printf("filedrop-directory %s listening on %s (snapshot: %s)", version, *addr, *snapshot)
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
