// filedrop-agent - sends files from a computer to the interactive panel in
// the same room (and receives them on the panel) with no user interaction:
// the user just drops a file or folder into the "Shared folder".
//
// The role (sender / receiver) is derived from the machine's own hostname
// using a configurable naming scheme (see naming.go); the same binary and
// package are installed on every machine.
//
// Usage:
//
//	filedrop-agent [-config FILE] [run]   run the agent (default)
//	filedrop-agent [-config FILE] info    print role, shared folder, link name (for scripts)
//	filedrop-agent [-config FILE] check   diagnose this machine and its counterpart
//	filedrop-agent version
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/filedrop/agent.conf", "path to the configuration file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-config FILE] [run|info|check|version]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	cmd := flag.Arg(0)
	if *showVersion || cmd == "version" {
		fmt.Println("filedrop-agent", version)
		return
	}

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fatal(cmd, "configuration error: %v", err)
	}

	switch cmd {
	case "", "run":
		run(cfg)
	case "info":
		cmdInfo(cfg)
	case "check":
		cmdCheck(cfg, *configPath)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// fatal prints to stderr for the helper subcommands, and logs for the daemon.
func fatal(cmd, format string, args ...interface{}) {
	if cmd == "" || cmd == "run" {
		log.Fatalf(format, args...)
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func currentHostname() (string, error) {
	// Lets you override the hostname for tests/debugging without renaming
	// the machine. Not set in production.
	if h := os.Getenv("FILEDROP_HOSTNAME"); h != "" {
		return h, nil
	}
	return os.Hostname()
}

func newResolver(cfg Config, id Identity) (Resolver, *DirectoryResolver, error) {
	switch cfg.ResolveMode {
	case "dns":
		return DNSResolver{}, nil, nil
	case "static":
		return NewStaticResolver(cfg.StaticHostsFile), nil, nil
	case "directory":
		urls := cfg.DirectoryURLs()
		if len(urls) == 0 {
			return nil, nil, fmt.Errorf("RESOLVE_MODE=directory but DIRECTORY_URL is not set")
		}
		dr := NewDirectoryResolver(urls, cfg.DirectoryToken)
		return dr, dr, nil
	default:
		return nil, nil, fmt.Errorf("unknown RESOLVE_MODE=%q (expected dns, directory or static)", cfg.ResolveMode)
	}
}

func run(cfg Config) {
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("cannot open log file %s: %v", cfg.LogFile, err)
		}
		log.SetOutput(f)
	}
	log.SetFlags(log.LstdFlags)

	var id Identity
	for {
		hostname, err := currentHostname()
		if err != nil {
			log.Printf("cannot get the hostname: %v, retrying in 60s", err)
			time.Sleep(60 * time.Second)
			continue
		}
		id, err = Identify(cfg, hostname)
		if err != nil {
			log.Printf("%v - cannot determine the role, retrying in 60s", err)
			time.Sleep(60 * time.Second)
			continue
		}
		break
	}
	log.Printf("filedrop-agent %s: hostname=%s role=%s kind=%s resolve_mode=%s",
		version, id.Raw, id.Role, id.Kind, cfg.ResolveMode)

	resolver, dir, err := newResolver(cfg, id)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if dir != nil {
		go runRegisterLoop(cfg, id, dir)
	}

	switch {
	case id.IsPanel():
		runReceiver(cfg)
	case id.IsClient():
		runSender(cfg, id, resolver)
	default:
		log.Fatalf("hostname %s has no usable role", id.Raw)
	}
}

// cmdInfo prints KEY=VALUE lines for the helper shell scripts (cleanup,
// desktop shortcut) so that they never duplicate the naming logic.
func cmdInfo(cfg Config) {
	hostname, err := currentHostname()
	if err != nil {
		fatal("info", "cannot get the hostname: %v", err)
	}
	id, err := Identify(cfg, hostname)
	if err != nil {
		fatal("info", "%v", err)
	}
	fmt.Printf("role=%s\n", id.Kind)
	fmt.Printf("target=%s\n", cfg.SharedDir(id.Kind))
	fmt.Printf("link_name=%s\n", cfg.SharedFolderName)
}

func cmdCheck(cfg Config, configPath string) {
	ok := true
	report := func(good bool, format string, args ...interface{}) {
		mark := "OK  "
		if !good {
			mark = "FAIL"
			ok = false
		}
		fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(format, args...))
	}

	fmt.Printf("filedrop-agent %s, config %s\n", version, configPath)

	hostname, err := currentHostname()
	if err != nil {
		report(false, "cannot get the hostname: %v", err)
		os.Exit(1)
	}
	id, err := Identify(cfg, hostname)
	if err != nil {
		report(false, "%v", err)
		os.Exit(1)
	}
	report(true, "hostname %s -> role %q, this machine is a %s", id.Raw, id.Role, id.Kind)

	if _, err := os.Stat(cfg.SharedDir(id.Kind)); err != nil {
		report(false, "shared folder %s: %v", cfg.SharedDir(id.Kind), err)
	} else {
		report(true, "shared folder %s exists", cfg.SharedDir(id.Kind))
	}

	if id.IsPanel() {
		fmt.Println("This is a panel (receiver): it waits for files; run `filedrop-agent check` on the client to test the whole path.")
		finish(ok)
		return
	}

	if _, err := os.Stat(cfg.SSHKey); err != nil {
		report(false, "SSH key %s: %v", cfg.SSHKey, err)
	} else {
		report(true, "SSH key %s is present", cfg.SSHKey)
	}

	resolver, _, err := newResolver(cfg, id)
	if err != nil {
		report(false, "%v", err)
		finish(ok)
		return
	}
	candidates := id.CounterpartCandidates()
	if len(candidates) == 0 {
		report(false, "no counterpart candidates (check HOSTNAME_PATTERN / PANEL_ROLES / PEER_HOSTNAMES)")
		finish(ok)
		return
	}
	fmt.Printf("      counterpart candidates: %v\n", candidates)

	local := localNets(cfg.IgnoreInterfaces)
	own := allOwnIPs()
	fmt.Printf("      this machine's usable addresses: %v\n", reportAddrs(local))

	found := false
	for _, c := range candidates {
		ips, err := resolver.Resolve(c)
		if err != nil {
			fmt.Printf("      %s: %v\n", c, err)
			continue
		}
		found = true
		report(true, "%s resolves to %v (%s mode)", c, ips, cfg.ResolveMode)
		ranked := rankCandidates(ips, local, own)
		if len(ranked) == 0 {
			report(false, "%s: no usable address (only this machine's own addresses?)", c)
			continue
		}
		reachable := false
		for _, cand := range ranked {
			if err := probeTCP(cand.IP, cfg.SSHPort, 4*time.Second); err != nil {
				report(false, "%s: cannot connect to port %d: %v", cand, cfg.SSHPort, err)
				continue
			}
			report(true, "%s: port %d is reachable", cand, cfg.SSHPort)
			reachable = true
			break
		}
		if !reachable {
			fmt.Println("      (firewall? sshd not running on the panel? no route between the subnets?)")
		}
	}
	if !found {
		report(false, "none of the counterpart candidates could be resolved")
	}
	finish(ok)
}

func finish(ok bool) {
	if !ok {
		os.Exit(1)
	}
}
