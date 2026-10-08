package main

import (
	"log"
	"time"
)

// runRegisterLoop periodically tells the directory that this machine is alive
// and which addresses it has. The directory also records the IP the request
// came from. Used only with RESOLVE_MODE=directory.
func runRegisterLoop(cfg Config, id Identity, dr *DirectoryResolver) {
	interval := time.Duration(cfg.RegisterIntervalSec) * time.Second
	if interval <= 0 {
		interval = 45 * time.Second
	}
	for {
		// re-read the interfaces every time: Wi-Fi/cable/DHCP changes are
		// picked up without restarting the agent
		addrs := reportAddrs(localNets(cfg.IgnoreInterfaces))
		if err := dr.Register(id.Raw, addrs); err != nil {
			log.Printf("registration in the directory failed: %v", err)
		}
		time.Sleep(interval)
	}
}
