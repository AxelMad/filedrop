package main

import (
	"log"
	"os"
	"time"
)

// runReceiver is the "panel" role. The transfer itself is handled by sshd
// (chroot + internal-sftp, configured by the package), so the agent only has
// to keep the inbox in place and, once a minute, mention in the journal
// entries that have been lying around unusually long - a hint for diagnostics.
func runReceiver(cfg Config) {
	if err := os.MkdirAll(cfg.InboxDir, 0755); err != nil {
		log.Fatalf("cannot create inbox %s: %v", cfg.InboxDir, err)
	}
	log.Printf("role: panel. Inbox: %s (files arrive via sftp/scp, see sshd_config.d/filedrop.conf)", cfg.InboxDir)

	for {
		time.Sleep(60 * time.Second)
		entries, err := os.ReadDir(cfg.InboxDir)
		if err != nil {
			log.Printf("cannot read inbox %s: %v", cfg.InboxDir, err)
			continue
		}
		stale := 0
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if time.Since(info.ModTime()) > 10*time.Minute {
				stale++
			}
		}
		if stale > 0 {
			log.Printf("%d entr(y/ies) in %s older than 10 minutes", stale, cfg.InboxDir)
		}
	}
}
