package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	baseTransferTimeout = 60 * time.Second
	maxTransferTimeout  = 20 * time.Minute
	assumedSlowRate     = 2 << 20 // bytes per second, pessimistic Wi-Fi estimate
)

// transferTimeout gives a transfer 60 s plus the time it would take at a slow
// 2 MB/s, capped at 20 minutes.
func transferTimeout(size int64) time.Duration {
	if size <= 0 {
		return baseTransferTimeout
	}
	t := baseTransferTimeout + time.Duration(size/assumedSlowRate)*time.Second
	if t > maxTransferTimeout {
		return maxTransferTimeout
	}
	return t
}

// sendEntry copies a file or a folder to the remote machine with scp, using
// the dedicated, restricted agent key (never an administrator's key). The
// system scp is used on purpose rather than an SSH library: openssh-client is
// already present on the target machines and its behaviour is well known and
// easy to reproduce by hand when debugging.
func sendEntry(cfg Config, remoteIP, localPath string, isDir bool, size int64) error {
	dest := fmt.Sprintf("%s@%s:%s/", cfg.SSHUser, remoteIP, cfg.RemoteInboxPath)

	port := cfg.SSHPort
	if port == 0 {
		port = 22
	}
	args := []string{
		"-P", fmt.Sprint(port),
		"-i", cfg.SSHKey,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + cfg.KnownHostsFile,
	}
	if cfg.ScpSftpFlag {
		args = append([]string{"-s"}, args...)
	}
	if isDir {
		// The receiver's sftp server creates directories with exactly the
		// modes they have on the sender (no umask), files are masked by the
		// umask. Make the directories world-writable first so that the
		// receiving user can move/delete them.
		if err := chmodDirsWorld(localPath); err != nil {
			log.Printf("warning: could not chmod folder %s: %v", localPath, err)
		}
		args = append(args, "-r")
	}
	args = append(args, localPath, dest)

	cmd := exec.Command("scp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting scp: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timeout := transferTimeout(size)
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("scp %s -> %s: %v: %s", localPath, dest, err, stderr.String())
		}
		return nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("scp %s -> %s: timed out after %s", localPath, dest, timeout)
	}
}

// chmodDirsWorld sets mode 0777 on root and every directory below it
// (regular files are left untouched).
func chmodDirsWorld(root string) error {
	var firstErr error
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return nil
		}
		if d.IsDir() {
			if err := os.Chmod(path, 0777); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return nil
	})
	return firstErr
}
