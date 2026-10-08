package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "10.5.5.5:40000"
	if token != "" {
		req.Header.Set("X-Filedrop-Token", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterAndResolve(t *testing.T) {
	s := newStore("")
	mux := newMux(s, "tok", time.Minute)

	if c := do(t, mux, "POST", "/register", "bad", `{"hostname":"pc-1"}`).Code; c != 401 {
		t.Fatalf("bad token: %d", c)
	}
	if c := do(t, mux, "GET", "/register", "tok", "").Code; c != 405 {
		t.Fatalf("GET register: %d", c)
	}
	if c := do(t, mux, "POST", "/register", "tok", `{}`).Code; c != 400 {
		t.Fatalf("empty body: %d", c)
	}
	if c := do(t, mux, "POST", "/register", "tok", `{"hostname":"pc-1"}`).Code; c != 200 {
		t.Fatalf("register: %d", c)
	}

	r := do(t, mux, "GET", "/resolve?name=pc-1", "tok", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"ip":"10.5.5.5"`) {
		t.Fatalf("resolve: %d %s", r.Code, r.Body)
	}
	if c := do(t, mux, "GET", "/resolve?name=zzz", "tok", "").Code; c != 404 {
		t.Fatalf("unknown: %d", c)
	}
	if c := do(t, mux, "GET", "/resolve?name=pc-1", "", "").Code; c != 401 {
		t.Fatalf("missing token: %d", c)
	}
	if c := do(t, mux, "GET", "/healthz", "", "").Code; c != 200 {
		t.Fatalf("healthz: %d", c)
	}
	st := do(t, mux, "GET", "/status", "tok", "")
	if st.Code != 200 || !strings.Contains(st.Body.String(), "pc-1") {
		t.Fatalf("status: %d", st.Code)
	}
}

func TestOpenModeWithoutToken(t *testing.T) {
	mux := newMux(newStore(""), "", time.Minute)
	if c := do(t, mux, "POST", "/register", "", `{"hostname":"a"}`).Code; c != 200 {
		t.Fatalf("%d", c)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s := newStore(p)
	s.set("pc-1", "1.1.1.1", []string{"10.0.0.5/24"})
	s.saveSnapshot()
	s2 := newStore(p)
	if r, ok := s2.get("pc-1"); !ok || r.IP != "1.1.1.1" || len(r.Addrs) != 1 {
		t.Fatalf("%+v %v", r, ok)
	}
}

func TestReportedAddressesAreReturnedWithObservedFirst(t *testing.T) {
	mux := newMux(newStore(""), "tok", time.Minute)
	body := `{"hostname":"panel-1","addrs":["192.168.1.9/24","10.0.0.7/24","127.0.0.1/8","169.254.1.1/16","junk"]}`
	if c := do(t, mux, "POST", "/register", "tok", body).Code; c != 200 {
		t.Fatalf("register: %d", c)
	}
	r := do(t, mux, "GET", "/resolve?name=panel-1", "tok", "")
	var resp struct {
		IP  string   `json:"ip"`
		IPs []string `json:"ips"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	want := []string{"10.5.5.5", "192.168.1.9", "10.0.0.7"}
	if resp.IP != "10.5.5.5" || !reflect.DeepEqual(resp.IPs, want) {
		t.Fatalf("got %+v want ips %v (loopback, link-local and junk must be dropped)", resp, want)
	}
}

func TestOldAgentWithoutAddrs(t *testing.T) {
	mux := newMux(newStore(""), "", time.Minute)
	do(t, mux, "POST", "/register", "", `{"hostname":"old"}`)
	r := do(t, mux, "GET", "/resolve?name=old", "", "")
	if !strings.Contains(r.Body.String(), `"ips":["10.5.5.5"]`) {
		t.Fatalf("%s", r.Body)
	}
}
