package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the agent settings, read from a plain KEY=VALUE file
// (default /etc/filedrop/agent.conf). The format is deliberately primitive so
// that it can be generated or patched by any shell script without YAML/JSON
// dependencies.
type Config struct {
	// --- who is who -------------------------------------------------

	// Role: "auto" (derive from the hostname via the naming scheme),
	// "panel" or "client" (force the side).
	Role string
	// HostnamePattern is a Go (RE2) regular expression with a named group
	// (?P<role>...). See naming.go.
	HostnamePattern string
	PanelRoles      []string // values of the "role" group that mean "panel"
	ClientRoles     []string // values of the "role" group that mean "client"
	// PeerHostnames explicitly names the counterpart(s); overrides the scheme.
	PeerHostnames []string

	// --- finding the counterpart's IP address -------------------------

	// ResolveMode: "dns" (system resolver: DNS/mDNS/hosts), "directory"
	// (HTTP lookup service for DHCP networks without DNS registration) or
	// "static" (local file "hostname<whitespace>ip").
	ResolveMode     string
	DirectoryURL    string // one URL or several, comma/space separated
	DirectoryToken  string // shared secret for the directory service
	StaticHostsFile string

	// --- folders ----------------------------------------------------

	OutboxDir string // sender: the folder users drop files into
	InboxDir  string // receiver: where received files land (real path)
	SentDir   string // sender: where successfully delivered entries are moved

	// RemoteInboxPath is the inbox path as seen from inside the receiver's
	// sshd chroot. The receiver chroots into /opt/filedrop, so the real
	// /opt/filedrop/inbox is "/inbox" inside the chroot - this is the path that
	// has to be given to scp/sftp as destination.
	RemoteInboxPath string

	// SharedFolderName is the name of the desktop shortcut users see.
	SharedFolderName string

	// --- transport --------------------------------------------------

	SSHUser        string
	SSHKey         string
	SSHPort        int
	KnownHostsFile string
	// ScpSftpFlag adds "-s" to scp, forcing the SFTP protocol. Needed with
	// OpenSSH 8.7-8.9 clients; OpenSSH 9.0+ uses SFTP by default and older
	// clients do not know the flag. The receiver only speaks SFTP.
	ScpSftpFlag bool

	// IgnoreInterfaces: glob patterns of network interfaces whose addresses are
	// neither reported to the directory nor used to judge "same subnet"
	// (container/VM bridges).
	IgnoreInterfaces []string

	// --- timing -----------------------------------------------------

	RegisterIntervalSec int
	PollIntervalSec     int
	SettleChecks        int // polls in a row the size must stay unchanged

	LogFile string // empty = stderr (systemd journal)
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() Config {
	return Config{
		Role:                "auto",
		HostnamePattern:     `^(?P<role>panel|pc)-(?P<site>.+)$`,
		PanelRoles:          []string{"panel"},
		ClientRoles:         []string{"pc"},
		ResolveMode:         "dns",
		StaticHostsFile:     "/etc/filedrop/hosts.tsv",
		OutboxDir:           "/opt/filedrop/outbox",
		InboxDir:            "/opt/filedrop/inbox",
		SentDir:             "/opt/filedrop/outbox/.sent",
		RemoteInboxPath:     "/inbox",
		SharedFolderName:    "Shared folder",
		SSHUser:             "filedrop",
		SSHKey:              "/etc/filedrop/id_ed25519",
		SSHPort:             22,
		KnownHostsFile:      "/var/lib/filedrop/known_hosts",
		IgnoreInterfaces:    defaultIgnoreInterfaces,
		RegisterIntervalSec: 45,
		PollIntervalSec:     2,
		SettleChecks:        2,
	}
}

// DirectoryURLs returns the configured directory URLs (one or several).
func (c Config) DirectoryURLs() []string {
	var out []string
	for _, u := range splitList(c.DirectoryURL) {
		out = append(out, strings.TrimRight(u, "/"))
	}
	return out
}

// SharedDir is the folder users see as "the shared folder" on this machine.
func (c Config) SharedDir(kind Kind) string {
	if kind == KindPanel {
		return c.InboxDir
	}
	return c.OutboxDir
}

func parseBool(v string) bool {
	switch strings.ToLower(v) {
	case "1", "yes", "true", "on":
		return true
	}
	return false
}

func parseInt(path string, lineNo int, key, val string) (int, error) {
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("%s:%d: %s must be a number: %w", path, lineNo, key, err)
	}
	return n, nil
}

// LoadConfig reads a KEY=VALUE file on top of the defaults. A missing file is
// not an error (defaults are used), which is convenient for the first start
// and for tests.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("opening config %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			return cfg, fmt.Errorf("%s:%d: expected KEY=VALUE, got %q", path, lineNo, line)
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}

		switch key {
		case "ROLE":
			cfg.Role = strings.ToLower(val)
		case "HOSTNAME_PATTERN":
			cfg.HostnamePattern = val
		case "PANEL_ROLES":
			cfg.PanelRoles = splitList(val)
		case "CLIENT_ROLES":
			cfg.ClientRoles = splitList(val)
		case "PEER_HOSTNAMES":
			cfg.PeerHostnames = splitList(val)
		case "RESOLVE_MODE":
			cfg.ResolveMode = val
		case "DIRECTORY_URL":
			cfg.DirectoryURL = val
		case "DIRECTORY_TOKEN":
			cfg.DirectoryToken = val
		case "STATIC_HOSTS_FILE":
			cfg.StaticHostsFile = val
		case "OUTBOX_DIR":
			cfg.OutboxDir = val
		case "INBOX_DIR":
			cfg.InboxDir = val
		case "REMOTE_INBOX_PATH":
			cfg.RemoteInboxPath = val
		case "SENT_DIR":
			cfg.SentDir = val
		case "SHARED_FOLDER_NAME":
			cfg.SharedFolderName = val
		case "SSH_USER":
			cfg.SSHUser = val
		case "SSH_KEY":
			cfg.SSHKey = val
		case "IGNORE_INTERFACES":
			cfg.IgnoreInterfaces = splitList(val)
		case "SCP_SFTP_FLAG":
			cfg.ScpSftpFlag = parseBool(val)
		case "KNOWN_HOSTS_FILE":
			cfg.KnownHostsFile = val
		case "SSH_PORT":
			if cfg.SSHPort, err = parseInt(path, lineNo, key, val); err != nil {
				return cfg, err
			}
		case "REGISTER_INTERVAL_SEC":
			if cfg.RegisterIntervalSec, err = parseInt(path, lineNo, key, val); err != nil {
				return cfg, err
			}
		case "POLL_INTERVAL_SEC":
			if cfg.PollIntervalSec, err = parseInt(path, lineNo, key, val); err != nil {
				return cfg, err
			}
		case "SETTLE_CHECKS":
			if cfg.SettleChecks, err = parseInt(path, lineNo, key, val); err != nil {
				return cfg, err
			}
		case "LOG_FILE":
			cfg.LogFile = val
		default:
			return cfg, fmt.Errorf("%s:%d: unknown setting %q", path, lineNo, key)
		}
	}
	if err := sc.Err(); err != nil {
		return cfg, fmt.Errorf("reading config %s: %w", path, err)
	}

	if cfg.SentDir == "" {
		cfg.SentDir = cfg.OutboxDir + "/.sent"
	}
	if cfg.SettleChecks < 1 {
		cfg.SettleChecks = 1
	}
	return cfg, nil
}
