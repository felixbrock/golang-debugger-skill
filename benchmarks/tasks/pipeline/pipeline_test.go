package pipeline

import "testing"

// A morning of logs from two services. The expected numbers below were
// computed by hand from the stage specs (see the comments in each file):
// windows are [start, start+60) aligned to the minute, duplicates within 5s
// are dropped, adjacent quiet windows merge, and a service scores by how far
// each window's error rate exceeds 1/min.
const log = `
# ts | level | service | msg
60000|error|api|timeout calling db
60002|error|api|timeout calling db
60004|error|api|timeout calling db
60010|info|api|retry scheduled
60015|error|api|timeout calling db
60030|error|api|connection pool exhausted
60045|error|api|connection pool exhausted 2
60050|info|api|pool resized
60060|error|api|timeout calling db
60061|info|api|breaker probe
60070|info|api|breaker half-open
60090|info|api|probe ok
60119|info|api|breaker closed
60120|info|api|steady
60150|info|api|steady 2
60180|info|api|steady 3
60210|info|api|steady 4
60005|info|db|checkpoint start
60020|info|db|checkpoint done
60058|error|db|slow query 812ms
60060|info|db|vacuum start
60075|info|db|vacuum done
60120|error|db|slow query 903ms
60125|error|db|slow query 911ms
60127|error|db|slow query 911ms
60180|info|db|nightly stats
60240|error|db|replication lag 3s
60244|error|db|replication lag 4s
60250|error|db|replication lag 5s
60255|error|db|replication lag 5s
`

func TestReport(t *testing.T) {
	got, err := Report(log)
	if err != nil {
		t.Fatal(err)
	}
	want := Summary{Windows: 6, TotalErrors: 11, Worst: "api", WorstScore: 3.0}
	if got != want {
		t.Fatalf("Report() = %+v, want %+v", got, want)
	}
}
