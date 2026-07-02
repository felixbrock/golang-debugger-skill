// Package pipeline turns a raw event log into a per-service anomaly report.
//
// Stages: Parse -> Window -> Dedupe -> Rate -> Merge -> Score. Each stage is
// in its own file; Report in report.go runs them all.
package pipeline

// Event is one parsed log line.
type Event struct {
	TS      int64 // unix seconds
	Level   string
	Service string
	Msg     string
}

// Window is a fixed-size time bucket of events for one service.
type Window struct {
	Service string
	Start   int64 // inclusive
	End     int64 // exclusive
	Events  []Event
	Errors  int
	// Rate fields, filled by the Rate stage.
	ErrPerMin float64
}

// WindowSize is the bucket width in seconds.
const WindowSize = 60
