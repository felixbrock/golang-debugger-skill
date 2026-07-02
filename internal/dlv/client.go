package dlv

import (
	"fmt"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"
)

// Client talks to a headless Delve instance over its JSON-RPC v2 API.
type Client struct {
	rpc *rpc.Client
}

func Dial(addr string) (*Client, error) {
	var conn net.Conn
	var err error
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to delve at %s: %w", addr, err)
	}
	c := &Client{rpc: jsonrpc.NewClient(conn)}
	// Make sure we are on API v2.
	var out struct{}
	if err := c.rpc.Call("RPCServer.SetApiVersion", struct{ APIVersion int }{2}, &out); err != nil {
		return nil, fmt.Errorf("set api version: %w", err)
	}
	return c, nil
}

func (c *Client) Close() error { return c.rpc.Close() }

func (c *Client) call(method string, arg, out any) error {
	return c.rpc.Call("RPCServer."+method, arg, out)
}

// Command runs continue/next/step/... and blocks until the target stops.
func (c *Client) Command(cmd DebuggerCommand) (*DebuggerState, error) {
	var out struct{ State DebuggerState }
	if err := c.call("Command", cmd, &out); err != nil {
		return nil, err
	}
	return &out.State, nil
}

func (c *Client) State(nonBlocking bool) (*DebuggerState, error) {
	var out struct{ State *DebuggerState }
	err := c.call("State", struct{ NonBlocking bool }{nonBlocking}, &out)
	return out.State, err
}

// CreateBreakpoint sets a breakpoint. locExpr is a delve location expression
// ("file:line", "pkg.Func", …) resolved server-side with suffix matching.
func (c *Client) CreateBreakpoint(bp *Breakpoint, locExpr string) (*Breakpoint, error) {
	arg := struct {
		Breakpoint          Breakpoint
		LocExpr             string
		SubstitutePathRules [][2]string
		Suspended           bool
	}{Breakpoint: *bp, LocExpr: locExpr}
	var out struct{ Breakpoint Breakpoint }
	if err := c.call("CreateBreakpoint", arg, &out); err != nil {
		return nil, err
	}
	return &out.Breakpoint, nil
}

func (c *Client) AmendBreakpoint(bp *Breakpoint) error {
	var out struct{}
	return c.call("AmendBreakpoint", struct{ Breakpoint Breakpoint }{*bp}, &out)
}

func (c *Client) ListBreakpoints() ([]*Breakpoint, error) {
	var out struct{ Breakpoints []*Breakpoint }
	err := c.call("ListBreakpoints", struct{ All bool }{false}, &out)
	return out.Breakpoints, err
}

func (c *Client) ClearBreakpoint(id int) (*Breakpoint, error) {
	var out struct{ Breakpoint *Breakpoint }
	err := c.call("ClearBreakpoint", struct {
		Id   int
		Name string
	}{Id: id}, &out)
	return out.Breakpoint, err
}

func (c *Client) CreateWatchpoint(scope EvalScope, expr string, typ int) (*Breakpoint, error) {
	var out struct{ *Breakpoint }
	err := c.call("CreateWatchpoint", struct {
		Scope EvalScope
		Expr  string
		Type  int
	}{scope, expr, typ}, &out)
	return out.Breakpoint, err
}

func (c *Client) Stacktrace(goroutineID int64, depth int, cfg *LoadConfig) ([]Stackframe, error) {
	var out struct{ Locations []Stackframe }
	err := c.call("Stacktrace", struct {
		Id     int64
		Depth  int
		Full   bool
		Defers bool
		Opts   int
		Cfg    *LoadConfig
	}{Id: goroutineID, Depth: depth, Cfg: cfg}, &out)
	return out.Locations, err
}

func (c *Client) ListGoroutines(start, count int) ([]*Goroutine, int, error) {
	var out struct {
		Goroutines []*Goroutine
		Nextg      int
	}
	err := c.call("ListGoroutines", struct{ Start, Count int }{start, count}, &out)
	return out.Goroutines, out.Nextg, err
}

func (c *Client) ListLocalVars(scope EvalScope, cfg LoadConfig) ([]Variable, error) {
	var out struct{ Variables []Variable }
	err := c.call("ListLocalVars", struct {
		Scope EvalScope
		Cfg   LoadConfig
	}{scope, cfg}, &out)
	return out.Variables, err
}

func (c *Client) ListFunctionArgs(scope EvalScope, cfg LoadConfig) ([]Variable, error) {
	var out struct{ Args []Variable }
	err := c.call("ListFunctionArgs", struct {
		Scope EvalScope
		Cfg   LoadConfig
	}{scope, cfg}, &out)
	return out.Args, err
}

func (c *Client) Eval(scope EvalScope, expr string, cfg *LoadConfig) (*Variable, error) {
	var out struct{ Variable *Variable }
	err := c.call("Eval", struct {
		Scope EvalScope
		Expr  string
		Cfg   *LoadConfig
	}{scope, expr, cfg}, &out)
	return out.Variable, err
}

func (c *Client) Set(scope EvalScope, symbol, value string) error {
	var out struct{}
	return c.call("Set", struct {
		Scope  EvalScope
		Symbol string
		Value  string
	}{scope, symbol, value}, &out)
}

func (c *Client) FindLocation(scope EvalScope, loc string) ([]Location, error) {
	var out struct {
		Locations         []Location
		SubstituteLocExpr string
	}
	err := c.call("FindLocation", struct {
		Scope                     EvalScope
		Loc                       string
		IncludeNonExecutableLines bool
		SubstitutePathRules       [][2]string
	}{Scope: scope, Loc: loc}, &out)
	return out.Locations, err
}

// Restart relaunches the target. rebuild recompiles it first (dlv debug/test).
func (c *Client) Restart(rebuild bool) error {
	var out struct {
		DiscardedBreakpoints []any
	}
	return c.call("Restart", struct {
		Position     string
		ResetArgs    bool
		NewArgs      []string
		Rerecord     bool
		Rebuild      bool
		NewRedirects [3]string
	}{Rebuild: rebuild}, &out)
}

func (c *Client) Halt() (*DebuggerState, error) {
	// Halt must not go through Command (which would block on the same
	// connection); RPCServer.Command with name halt is still the API, but it
	// is safe on a second connection. Delve also exposes it via Command.
	return c.Command(DebuggerCommand{Name: Halt})
}

func (c *Client) Detach(kill bool) error {
	var out struct{}
	return c.call("Detach", struct{ Kill bool }{kill}, &out)
}
