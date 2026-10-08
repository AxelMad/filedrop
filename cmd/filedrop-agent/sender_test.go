package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

type sent struct {
	path  string
	isDir bool
	size  int64
	ip    string
}

func testSender(t *testing.T) (*sender, *[]sent, Config) {
	t.Helper()
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.OutboxDir = filepath.Join(root, "outbox")
	cfg.SentDir = filepath.Join(root, "outbox", ".sent")
	cfg.SettleChecks = 2
	os.MkdirAll(cfg.SentDir, 0755)

	var log []sent
	s := newSender(cfg, []string{"panel-1"}, fakeResolver{"panel-1": {"10.1.1.1"}})
	s.send = func(c Config, ip, path string, isDir bool, size int64) error {
		log = append(log, sent{path, isDir, size, ip})
		return nil
	}
	s.probe = func(ip string) error { return nil } // every port answers
	s.netInfo = func() ([]*net.IPNet, map[string]bool) { return nil, nil }
	return s, &log, cfg
}

func TestDirSize(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "a", "b"), 0755)
	os.WriteFile(filepath.Join(d, "x"), make([]byte, 10), 0644)
	os.WriteFile(filepath.Join(d, "a", "y"), make([]byte, 20), 0644)
	os.WriteFile(filepath.Join(d, "a", "b", "z"), make([]byte, 5), 0644)
	if got := dirSize(d); got != 35 {
		t.Fatalf("dirSize = %d", got)
	}
	if got := dirSize(filepath.Join(d, "missing")); got != 0 {
		t.Fatalf("dirSize(missing) = %d", got)
	}
}

func TestFileIsSentOnlyAfterItSettles(t *testing.T) {
	s, log, cfg := testSender(t)
	p := filepath.Join(cfg.OutboxDir, "a.txt")
	os.WriteFile(p, []byte("1"), 0644)

	s.scan() // first sight
	if len(*log) != 0 {
		t.Fatal("sent too early")
	}
	os.WriteFile(p, []byte("12"), 0644) // still growing
	s.scan()
	if len(*log) != 0 {
		t.Fatal("sent while growing")
	}
	s.scan() // stable for 2 polls
	if len(*log) != 1 || (*log)[0].ip != "10.1.1.1" || (*log)[0].isDir {
		t.Fatalf("log: %+v", *log)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("file should have been moved away")
	}
	entries, _ := os.ReadDir(cfg.SentDir)
	if len(entries) != 1 {
		t.Fatalf("sent dir: %v", entries)
	}
}

func TestFolderIsSentAsFolder(t *testing.T) {
	s, log, cfg := testSender(t)
	d := filepath.Join(cfg.OutboxDir, "lesson")
	os.MkdirAll(filepath.Join(d, "sub"), 0755)
	os.WriteFile(filepath.Join(d, "sub", "f"), make([]byte, 100), 0644)

	s.scan()
	s.scan()
	if len(*log) != 1 || !(*log)[0].isDir || (*log)[0].size != 100 {
		t.Fatalf("log: %+v", *log)
	}
}

func TestFolderGrowingIsNotSentYet(t *testing.T) {
	s, log, cfg := testSender(t)
	d := filepath.Join(cfg.OutboxDir, "lesson")
	os.MkdirAll(d, 0755)
	os.WriteFile(filepath.Join(d, "1"), make([]byte, 10), 0644)
	s.scan()
	os.WriteFile(filepath.Join(d, "2"), make([]byte, 10), 0644)
	s.scan()
	if len(*log) != 0 {
		t.Fatal("sent while the folder was still changing")
	}
	s.scan()
	if len(*log) != 1 {
		t.Fatalf("log: %+v", *log)
	}
}

func TestDotEntriesAreIgnored(t *testing.T) {
	s, log, cfg := testSender(t)
	os.WriteFile(filepath.Join(cfg.OutboxDir, ".hidden"), []byte("x"), 0644)
	for i := 0; i < 4; i++ {
		s.scan()
	}
	if len(*log) != 0 {
		t.Fatal("dot entries must be skipped")
	}
}

func TestFailedSendIsRetried(t *testing.T) {
	s, log, cfg := testSender(t)
	fail := true
	s.send = func(c Config, ip, path string, isDir bool, size int64) error {
		*log = append(*log, sent{path: path})
		if fail {
			return errors.New("network down")
		}
		return nil
	}
	os.WriteFile(filepath.Join(cfg.OutboxDir, "a"), []byte("x"), 0644)
	s.scan()
	s.scan() // attempt 1 fails
	fail = false
	s.scan() // attempt 2 succeeds
	if len(*log) != 2 {
		t.Fatalf("attempts: %d", len(*log))
	}
	if _, err := os.Stat(filepath.Join(cfg.OutboxDir, "a")); !os.IsNotExist(err) {
		t.Fatal("file should be gone after the successful retry")
	}
}

func TestUnresolvableCounterpartIsRetried(t *testing.T) {
	s, log, cfg := testSender(t)
	s.resolver = fakeResolver{}
	os.WriteFile(filepath.Join(cfg.OutboxDir, "a"), []byte("x"), 0644)
	s.scan()
	s.scan()
	if len(*log) != 0 {
		t.Fatal("must not send without an address")
	}
	if _, err := os.Stat(filepath.Join(cfg.OutboxDir, "a")); err != nil {
		t.Fatal("file must stay in the outbox")
	}
}

func TestDeliveredButNotMovedIsNotResent(t *testing.T) {
	s, log, cfg := testSender(t)
	// make the move fail: SentDir sits "under" a regular file
	blocker := filepath.Join(cfg.OutboxDir, ".blocker")
	os.WriteFile(blocker, []byte("x"), 0644)
	s.cfg.SentDir = filepath.Join(blocker, "sent")

	p := filepath.Join(cfg.OutboxDir, "a")
	os.WriteFile(p, []byte("x"), 0644)
	for i := 0; i < 6; i++ {
		s.scan()
	}
	if len(*log) != 1 {
		t.Fatalf("sent %d times, expected exactly once", len(*log))
	}

	// a changed file is a new entry and goes out again
	os.WriteFile(p, []byte("changed!"), 0644)
	for i := 0; i < 3; i++ {
		s.scan()
	}
	if len(*log) != 2 {
		t.Fatalf("sent %d times, expected 2", len(*log))
	}
}

func mustNet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	return &net.IPNet{IP: ip, Mask: n.Mask}
}

// The panel is registered with two addresses; the laptop is on the second
// one's subnet, so that is the address that must be used - not the first.
func TestSameSubnetAddressIsPreferred(t *testing.T) {
	s, log, cfg := testSender(t)
	s.resolver = fakeResolver{"panel-1": {"10.0.0.7", "192.168.5.20"}}
	s.netInfo = func() ([]*net.IPNet, map[string]bool) {
		return []*net.IPNet{mustNet(t, "192.168.5.11/24")}, nil
	}
	os.WriteFile(filepath.Join(cfg.OutboxDir, "a"), []byte("x"), 0644)
	s.scan()
	s.scan()
	if len(*log) != 1 || (*log)[0].ip != "192.168.5.20" {
		t.Fatalf("log: %+v", *log)
	}
}

// Different subnets: the routed address is used.
func TestRoutedAddressWhenNoCommonSubnet(t *testing.T) {
	s, log, cfg := testSender(t)
	s.resolver = fakeResolver{"panel-1": {"10.0.0.7"}}
	s.netInfo = func() ([]*net.IPNet, map[string]bool) {
		return []*net.IPNet{mustNet(t, "172.26.1.86/16")}, nil
	}
	os.WriteFile(filepath.Join(cfg.OutboxDir, "a"), []byte("x"), 0644)
	s.scan()
	s.scan()
	if len(*log) != 1 || (*log)[0].ip != "10.0.0.7" {
		t.Fatalf("log: %+v", *log)
	}
}

// An unreachable same-subnet address must not block the delivery: fall
// through to the next one.
func TestUnreachableAddressIsSkipped(t *testing.T) {
	s, log, cfg := testSender(t)
	s.resolver = fakeResolver{"panel-1": {"192.168.5.20", "10.0.0.7"}}
	s.netInfo = func() ([]*net.IPNet, map[string]bool) {
		return []*net.IPNet{mustNet(t, "192.168.5.11/24")}, nil
	}
	s.probe = func(ip string) error {
		if ip == "192.168.5.20" {
			return errors.New("connection refused")
		}
		return nil
	}
	os.WriteFile(filepath.Join(cfg.OutboxDir, "a"), []byte("x"), 0644)
	s.scan()
	s.scan()
	if len(*log) != 1 || (*log)[0].ip != "10.0.0.7" {
		t.Fatalf("log: %+v", *log)
	}
}

func TestNothingReachableKeepsFileAndRetries(t *testing.T) {
	s, log, cfg := testSender(t)
	s.probe = func(ip string) error { return errors.New("timeout") }
	os.WriteFile(filepath.Join(cfg.OutboxDir, "a"), []byte("x"), 0644)
	for i := 0; i < 4; i++ {
		s.scan()
	}
	if len(*log) != 0 {
		t.Fatal("must not send when no address answers")
	}
	if _, err := os.Stat(filepath.Join(cfg.OutboxDir, "a")); err != nil {
		t.Fatal("file must stay in the outbox")
	}
}
