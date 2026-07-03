// Package daemon implements the per-project gdbg daemon: it owns one live
// (usually paused) delve session and serves CLI/MCP requests over a Unix
// socket, so debugger state survives across agent tool calls.
package daemon

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/felixbrock/golang-debugger-skill/internal/client"
	"github.com/felixbrock/golang-debugger-skill/internal/proto"
)

const idleTimeout = 30 * time.Minute

var errNoSession = errors.New("no debug session")

type Daemon struct {
	root string

	sessMu sync.Mutex // guards sess pointer
	sess   *Session

	serialMu sync.Mutex // serializes command handling (except pause)
	reqEnv   []string   // requesting client's env; guarded by serialMu

	activeMu sync.Mutex
	lastUsed time.Time

	usage *usageLog
}

func (d *Daemon) session() *Session {
	d.sessMu.Lock()
	defer d.sessMu.Unlock()
	return d.sess
}

func (d *Daemon) setSession(s *Session) {
	d.sessMu.Lock()
	defer d.sessMu.Unlock()
	d.sess = s
}

func (d *Daemon) stopSession() {
	d.sessMu.Lock()
	s := d.sess
	d.sess = nil
	d.sessMu.Unlock()
	if s != nil {
		s.close()
	}
}

func (d *Daemon) touch() {
	d.activeMu.Lock()
	d.lastUsed = time.Now()
	d.activeMu.Unlock()
}

// Run serves until "down" or idleTimeout. Called from `gdbg __daemon <root>`.
func Run(root string) error {
	d := &Daemon{root: root, lastUsed: time.Now(), usage: openUsageLog(client.UsagePath(root))}
	sock := client.SockPath(root)
	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	defer ln.Close()
	defer os.Remove(sock)
	defer d.stopSession()
	log.Printf("gdbg daemon listening on %s", sock)

	go func() {
		for {
			time.Sleep(time.Minute)
			d.activeMu.Lock()
			idle := time.Since(d.lastUsed)
			d.activeMu.Unlock()
			if idle > idleTimeout {
				log.Printf("idle for %s, shutting down", idle.Round(time.Second))
				d.stopSession()
				os.Remove(sock)
				os.Exit(0)
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		shutdown := d.handle(conn)
		if shutdown {
			return nil
		}
	}
}

// handle serves one request; returns true when the daemon should exit.
func (d *Daemon) handle(conn net.Conn) bool {
	defer conn.Close()
	d.touch()
	var req proto.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return false
	}
	enc := json.NewEncoder(conn)

	if len(req.Argv) > 0 && req.Argv[0] == "down" {
		d.usage.record("down", nil, time.Now(), nil)
		d.stopSession()
		enc.Encode(proto.Response{Text: "daemon stopped", Shutdown: true})
		return true
	}
	// pause must bypass the serial lock: it interrupts a blocked resume.
	if len(req.Argv) > 0 && req.Argv[0] == "pause" {
		start := time.Now()
		s := d.session()
		if s == nil {
			d.usage.record("pause", nil, start, errNoSession)
			enc.Encode(proto.Response{Text: "no debug session", IsError: true})
			return false
		}
		text, err := s.cmdPause()
		d.usage.record("pause", nil, start, err)
		enc.Encode(response(text, err))
		return false
	}

	d.serialMu.Lock()
	d.reqEnv = req.Env
	text, err := d.dispatch(req.Argv)
	d.serialMu.Unlock()
	d.touch()
	enc.Encode(response(text, err))
	return false
}

func response(text string, err error) proto.Response {
	if err != nil {
		return proto.Response{Text: "error: " + err.Error(), IsError: true}
	}
	return proto.Response{Text: text}
}
