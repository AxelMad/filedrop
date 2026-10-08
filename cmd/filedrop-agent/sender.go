package main

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// watch remembers one outbox entry between polls, so that a file that is
// still being written (a big presentation being copied in) is not sent
// half-finished.
type watch struct {
	size        int64
	stableCount int
}

// sendFunc transfers one outbox entry (file or folder) to remoteIP.
type sendFunc func(cfg Config, remoteIP, localPath string, isDir bool, size int64) error

// sender is the state of the "client" role.
type sender struct {
	cfg        Config
	candidates []string
	resolver   Resolver
	send       sendFunc
	probe      func(ip string) error // SSH port reachability check
	netInfo    func() ([]*net.IPNet, map[string]bool)

	watched map[string]*watch
	// delivered holds entries that were delivered successfully but could not be
	// moved to SentDir. They are left alone (and not sent again) until their
	// size changes - otherwise a failing move would resend them forever.
	delivered map[string]int64
}

func newSender(cfg Config, candidates []string, resolver Resolver) *sender {
	return &sender{
		cfg:        cfg,
		candidates: candidates,
		resolver:   resolver,
		send:       sendEntry,
		probe: func(ip string) error {
			port := cfg.SSHPort
			if port == 0 {
				port = 22
			}
			return probeTCP(ip, port, 3*time.Second)
		},
		netInfo: func() ([]*net.IPNet, map[string]bool) {
			return localNets(cfg.IgnoreInterfaces), allOwnIPs()
		},
		watched:   map[string]*watch{},
		delivered: map[string]int64{},
	}
}

// runSender is the main loop of the "client" role (laptop / desktop /
// all-in-one): every PollIntervalSec seconds it scans OutboxDir; an entry is
// ready to go when its size has not changed for SettleChecks polls in a row.
func runSender(cfg Config, id Identity, resolver Resolver) {
	if err := os.MkdirAll(cfg.OutboxDir, 0755); err != nil {
		log.Fatalf("cannot create outbox %s: %v", cfg.OutboxDir, err)
	}
	if err := os.MkdirAll(cfg.SentDir, 0755); err != nil {
		log.Fatalf("cannot create %s: %v", cfg.SentDir, err)
	}

	candidates := id.CounterpartCandidates()
	log.Printf("role: client. Outbox: %s. Counterpart candidates: %v", cfg.OutboxDir, candidates)

	s := newSender(cfg, candidates, resolver)
	interval := time.Duration(cfg.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		s.scan()
		time.Sleep(interval)
	}
}

// scan is one poll of the outbox.
func (s *sender) scan() {
	cfg := s.cfg
	entries, err := os.ReadDir(cfg.OutboxDir)
	if err != nil {
		log.Printf("cannot read outbox %s: %v", cfg.OutboxDir, err)
		return
	}

	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		seen[name] = true

		full := filepath.Join(cfg.OutboxDir, name)
		isDir := e.IsDir()
		var size int64
		if isDir {
			size = dirSize(full)
		} else {
			info, err := e.Info()
			if err != nil {
				continue
			}
			size = info.Size()
		}

		if dsize, stuck := s.delivered[name]; stuck {
			if dsize == size {
				continue // already delivered, just could not be moved away
			}
			delete(s.delivered, name) // changed after delivery: a new entry
		}

		w, ok := s.watched[name]
		if !ok || w.size != size {
			s.watched[name] = &watch{size: size, stableCount: 1}
			continue
		}
		w.stableCount++
		if w.stableCount < cfg.SettleChecks {
			continue
		}

		// The entry has settled - send it.
		if err := s.trySend(full, isDir, size); err != nil {
			log.Printf("cannot send %s %s: %v (will retry)", kindLabel(isDir), name, err)
			w.stableCount = cfg.SettleChecks - 1
			continue
		}
		delete(s.watched, name)
	}

	// Forget entries that disappeared from the outbox.
	for name := range s.watched {
		if !seen[name] {
			delete(s.watched, name)
		}
	}
	for name := range s.delivered {
		if !seen[name] {
			delete(s.delivered, name)
		}
	}
}

func (s *sender) trySend(fullPath string, isDir bool, size int64) error {
	targetHost, ips, err := resolveCounterpart(s.resolver, s.candidates)
	if err != nil {
		return fmt.Errorf("resolving the counterpart: %w", err)
	}
	// Same-subnet addresses first, then routed ones; use the first address
	// whose SSH port really answers.
	local, own := s.netInfo()
	pick, err := pickReachable(rankCandidates(ips, local, own), s.probe)
	if err != nil {
		return fmt.Errorf("%s: %w", targetHost, err)
	}
	ip := pick.IP
	if err := s.send(s.cfg, ip, fullPath, isDir, size); err != nil {
		return fmt.Errorf("transfer to %s (%s): %w", targetHost, ip, err)
	}

	name := filepath.Base(fullPath)
	sentPath := filepath.Join(s.cfg.SentDir, time.Now().Format("2006-01-02_15-04-05_")+name)
	if err := os.Rename(fullPath, sentPath); err != nil {
		// The transfer itself succeeded, so do NOT report an error (that would
		// trigger a resend); remember it instead.
		log.Printf("%s %s was delivered to %s but could not be moved to %s: %v",
			kindLabel(isDir), name, targetHost, s.cfg.SentDir, err)
		s.delivered[name] = size
		return nil
	}
	log.Printf("%s %s delivered to %s (%s) -> %s", kindLabel(isDir), name, targetHost, ip, sentPath)
	return nil
}

func kindLabel(isDir bool) string {
	if isDir {
		return "folder"
	}
	return "file"
}

// dirSize is the total size of all regular files below dir. It is used both
// for settle detection (a folder is "stable" when its total size stopped
// changing) and for choosing the transfer timeout.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
