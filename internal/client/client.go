// Package client connects the CLI (or MCP server) to the per-project daemon,
// spawning the daemon on first use.
package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/felixbrock/golang-debugger-skill/internal/proto"
)

// ProjectRoot walks up from dir looking for go.mod or .git; falls back to dir.
func ProjectRoot(dir string) string {
	d, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for cur := d; ; {
		for _, marker := range []string{"go.mod", ".git"} {
			if _, err := os.Stat(filepath.Join(cur, marker)); err == nil {
				return cur
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return d
		}
		cur = parent
	}
}

func StateDir(root string) string { return filepath.Join(root, ".gdbg") }
func LogPath(root string) string  { return filepath.Join(StateDir(root), "daemon.log") }

// SockPath lives under /tmp, not the project: Unix socket paths are limited
// to ~108 bytes and project paths can exceed that.
func SockPath(root string) string {
	h := sha256.Sum256([]byte(root))
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("gdbg-%d", os.Getuid()))
	os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, hex.EncodeToString(h[:8])+".sock")
}

// Send forwards argv to the daemon, starting it if needed.
func Send(root string, argv []string) (*proto.Response, error) {
	conn, err := dial(root)
	if err != nil {
		if err = spawnDaemon(root); err != nil {
			return nil, err
		}
		conn, err = dialRetry(root, 5*time.Second)
		if err != nil {
			return nil, fmt.Errorf("daemon did not start (see %s): %w", LogPath(root), err)
		}
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(proto.Request{Argv: argv, Env: os.Environ()}); err != nil {
		return nil, err
	}
	var resp proto.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("daemon connection lost: %w", err)
	}
	return &resp, nil
}

// SendIfRunning forwards argv only if a daemon is already up ("down", "stop").
func SendIfRunning(root string, argv []string) (*proto.Response, error) {
	conn, err := dial(root)
	if err != nil {
		return nil, errors.New("no daemon running")
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(proto.Request{Argv: argv}); err != nil {
		return nil, err
	}
	var resp proto.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func dial(root string) (net.Conn, error) {
	return net.DialTimeout("unix", SockPath(root), time.Second)
}

func dialRetry(root string, max time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(max)
	for {
		conn, err := dial(root)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func spawnDaemon(root string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(StateDir(root), 0o755); err != nil {
		return err
	}
	// A dead socket from a previous run blocks bind; remove it.
	os.Remove(SockPath(root))
	logf, err := os.OpenFile(LogPath(root), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(self, "__daemon", root)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.Dir = root
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
