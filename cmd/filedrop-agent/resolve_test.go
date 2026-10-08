package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
)

func goodDirectory(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		if r.Header.Get("X-Filedrop-Token") != "tok" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/resolve":
			if r.URL.Query().Get("name") == "known" {
				w.Write([]byte(`{"ip":"1.2.3.4","ips":["1.2.3.4","192.168.0.9"],"age_seconds":3}`))
				return
			}
			if r.URL.Query().Get("name") == "oldserver" {
				w.Write([]byte(`{"ip":"5.6.7.8","age_seconds":3}`))
				return
			}
			http.NotFound(w, r)
		case "/register":
			w.Write([]byte(`{"ok":true}`))
		}
	}))
}

func TestDirectoryResolve(t *testing.T) {
	srv := goodDirectory(t, nil)
	defer srv.Close()
	d := NewDirectoryResolver([]string{srv.URL + "/"}, "tok")

	ips, err := d.Resolve("known")
	if err != nil || !reflect.DeepEqual(ips, []string{"1.2.3.4", "192.168.0.9"}) {
		t.Fatalf("%v %v", ips, err)
	}
	if ips, err := d.Resolve("oldserver"); err != nil || !reflect.DeepEqual(ips, []string{"5.6.7.8"}) {
		t.Fatalf("older directory servers send only \"ip\": %v %v", ips, err)
	}
	if _, err := d.Resolve("stranger"); err == nil {
		t.Fatal("unknown host must fail")
	}
	if err := d.Register("known", []string{"192.168.0.9/24"}); err != nil {
		t.Fatal(err)
	}
	bad := NewDirectoryResolver([]string{srv.URL}, "wrong")
	if _, err := bad.Resolve("known"); err == nil {
		t.Fatal("wrong token must fail")
	}
}

func TestDirectoryFallsBackToWorkingURLAndSticks(t *testing.T) {
	var badHits int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&badHits, 1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	good := goodDirectory(t, nil)
	defer good.Close()

	d := NewDirectoryResolver([]string{bad.URL, good.URL}, "tok")
	for i := 0; i < 3; i++ {
		if ips, err := d.Resolve("known"); err != nil || len(ips) != 2 {
			t.Fatalf("round %d: %v %v", i, ips, err)
		}
	}
	if badHits != 1 {
		t.Fatalf("the failing URL was tried %d times, expected once (sticky last-good URL)", badHits)
	}
	if !d.warned {
		t.Fatal("a one-time mismatch warning is expected")
	}
}

func TestDirectoryNoWarningWhenFirstURLWorks(t *testing.T) {
	good := goodDirectory(t, nil)
	defer good.Close()
	d := NewDirectoryResolver([]string{good.URL, "http://127.0.0.1:1"}, "tok")
	if _, err := d.Resolve("known"); err != nil {
		t.Fatal(err)
	}
	if d.warned {
		t.Fatal("unexpected warning")
	}
}

func TestDirectoryUnknownHostDoesNotTryOtherURLs(t *testing.T) {
	var h1, h2 int32
	a := goodDirectory(t, &h1)
	defer a.Close()
	b := goodDirectory(t, &h2)
	defer b.Close()
	d := NewDirectoryResolver([]string{a.URL, b.URL}, "tok")
	if _, err := d.Resolve("stranger"); err == nil {
		t.Fatal("expected error")
	}
	if h1 != 1 || h2 != 0 {
		t.Fatalf("hits %d/%d: 404 is authoritative", h1, h2)
	}
}

func TestDirectoryNoURLs(t *testing.T) {
	if _, err := NewDirectoryResolver(nil, "").Resolve("x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestStaticResolver(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts.tsv")
	os.WriteFile(p, []byte("# c\npanel-1\t10.0.0.1\n\npc-1   10.0.0.2 172.16.0.2\nbroken\n"), 0644)
	r := NewStaticResolver(p)
	if ips, err := r.Resolve("pc-1"); err != nil || !reflect.DeepEqual(ips, []string{"10.0.0.2", "172.16.0.2"}) {
		t.Fatalf("%v %v", ips, err)
	}
	if _, err := r.Resolve("nope"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := NewStaticResolver(p + "x").Resolve("pc-1"); err == nil {
		t.Fatal("missing file must be an error")
	}
}

type fakeResolver map[string][]string

func (f fakeResolver) Resolve(h string) ([]string, error) {
	if ips, ok := f[h]; ok {
		return ips, nil
	}
	return nil, os.ErrNotExist
}

func TestResolveCounterpart(t *testing.T) {
	h, ips, err := resolveCounterpart(fakeResolver{"m-1": {"9.9.9.9"}}, []string{"n-1", "m-1"})
	if err != nil || h != "m-1" || !reflect.DeepEqual(ips, []string{"9.9.9.9"}) {
		t.Fatalf("%s %v %v", h, ips, err)
	}
	if _, _, err := resolveCounterpart(fakeResolver{}, []string{"n-1"}); err == nil {
		t.Fatal("expected error")
	}
	if _, _, err := resolveCounterpart(fakeResolver{}, nil); err == nil {
		t.Fatal("expected error for no candidates")
	}
}

func TestDNSResolverLocalhost(t *testing.T) {
	ips, err := DNSResolver{}.Resolve("localhost")
	if err != nil || len(ips) == 0 {
		t.Skipf("no localhost resolution in this environment: %v", err)
	}
}
