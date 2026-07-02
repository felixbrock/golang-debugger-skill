// Package dlv is a minimal client for Delve's native JSON-RPC v2 API.
//
// The types below mirror the subset of github.com/go-delve/delve/service/api
// that gdbg uses. They are decoded from JSON, so missing fields are simply
// left at their zero value; this keeps gdbg dependency-free and tolerant of
// Delve version drift.
package dlv

// EvalScope selects the goroutine and frame expressions are evaluated in.
// GoroutineID -1 means the currently selected goroutine.
type EvalScope struct {
	GoroutineID  int64
	Frame        int
	DeferredCall int
}

// LoadConfig controls how much of a variable Delve loads.
type LoadConfig struct {
	FollowPointers     bool
	MaxVariableRecurse int
	MaxStringLen       int
	MaxArrayValues     int
	MaxStructFields    int
}

// DefaultLoad is used for vars/eval/stop rendering.
var DefaultLoad = LoadConfig{
	FollowPointers:     true,
	MaxVariableRecurse: 2,
	MaxStringLen:       200,
	MaxArrayValues:     24,
	MaxStructFields:    -1,
}

type Breakpoint struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	File          string   `json:"file"`
	Line          int      `json:"line"`
	FunctionName  string   `json:"functionName,omitempty"`
	Cond          string   `json:"Cond,omitempty"`
	HitCond       string   `json:"HitCond,omitempty"`
	Tracepoint    bool     `json:"continue"`
	Variables     []string `json:"variables,omitempty"`
	WatchExpr     string   `json:"WatchExpr,omitempty"`
	WatchType     int      `json:"WatchType,omitempty"`
	TotalHitCount uint64   `json:"totalHitCount"`
	Disabled      bool     `json:"disabled"`
}

type Function struct {
	Name string `json:"name"`
}

type Location struct {
	PC       uint64    `json:"pc"`
	File     string    `json:"file"`
	Line     int       `json:"line"`
	Function *Function `json:"function,omitempty"`
}

type Thread struct {
	ID          int         `json:"id"`
	File        string      `json:"file"`
	Line        int         `json:"line"`
	Function    *Function   `json:"function,omitempty"`
	GoroutineID int64       `json:"goroutineID"`
	Breakpoint  *Breakpoint `json:"breakPoint,omitempty"`
	ReturnValues []Variable `json:"ReturnValues,omitempty"`
}

type Goroutine struct {
	ID             int64    `json:"id"`
	CurrentLoc     Location `json:"currentLoc"`
	UserCurrentLoc Location `json:"userCurrentLoc"`
	GoStatementLoc Location `json:"goStatementLoc"`
	ThreadID       int      `json:"threadID"`
}

type Stackframe struct {
	Location
	Locals    []Variable `json:"Locals,omitempty"`
	Arguments []Variable `json:"Arguments,omitempty"`
}

type Variable struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	RealType   string     `json:"realType"`
	Kind       uint       `json:"kind"` // reflect.Kind
	Value      string     `json:"value"`
	Len        int64      `json:"len"`
	Cap        int64      `json:"cap"`
	Children   []Variable `json:"children"`
	Unreadable string     `json:"unreadable"`
	Addr       uint64     `json:"addr"`
	OnlyAddr   bool       `json:"onlyAddr"`
}

type DebuggerState struct {
	Pid               int         `json:"Pid"`
	Running           bool        `json:"Running"`
	CurrentThread     *Thread     `json:"currentThread,omitempty"`
	SelectedGoroutine *Goroutine  `json:"currentGoroutine,omitempty"`
	NextInProgress    bool        `json:"NextInProgress"`
	WatchOutOfScope   []*Breakpoint `json:"WatchOutOfScope,omitempty"`
	Exited            bool        `json:"exited"`
	ExitStatus        int         `json:"exitStatus"`
}

// DebuggerCommand names understood by RPCServer.Command.
const (
	Continue        = "continue"
	Next            = "next"
	Step            = "step"
	StepOut         = "stepOut"
	StepInstruction = "stepInstruction"
	Halt            = "halt"
	SwitchGoroutine = "switchGoroutine"
)

// Watchpoint types (api.WatchType bit flags).
const (
	WatchRead  = 1 << 0
	WatchWrite = 1 << 1
)

type DebuggerCommand struct {
	Name        string `json:"name"`
	GoroutineID int64  `json:"goroutineID,omitempty"`
	Expr        string `json:"expr,omitempty"`
}
