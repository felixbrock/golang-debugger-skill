// Package proto defines the tiny JSON protocol between the gdbg CLI/MCP
// client and the per-project daemon: one request, one response, per
// connection on a Unix socket at <project>/.gdbg/daemon.sock.
package proto

type Request struct {
	// Argv is the raw command line after "gdbg", e.g. ["break", "main.go:12"].
	Argv []string `json:"argv"`
	// Env is the client's environment. launch/trace spawn dlv with it, so a
	// long-lived daemon does not pin the env of whichever client started it.
	Env []string `json:"env,omitempty"`
}

type Response struct {
	Text    string `json:"text"`
	IsError bool   `json:"isError"`
	// Shutdown tells the client the daemon is exiting (reply to "down").
	Shutdown bool `json:"shutdown,omitempty"`
}
