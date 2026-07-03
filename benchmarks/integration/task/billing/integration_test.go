package billing

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// startRates launches the real third-party rates service (bin/rates, no
// source available) on a free port and waits until it is healthy.
func startRates(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cmd := exec.Command("../bin/rates", "-addr", addr)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting rates service: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
		}
	}()

	base := "http://" + addr
	for i := 0; i < 50; i++ {
		resp, err := http.Get(base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("rates service did not become healthy")
	return ""
}

// TestInvoiceIntegration runs the same invoice as TestInvoiceTotal against
// the REAL rates service instead of the mock.
func TestInvoiceIntegration(t *testing.T) {
	base := startRates(t)
	c := &RateClient{Base: base}
	lines := []Line{
		{"hosting", 100, "USD"},
		{"consulting", 100, "EUR"},
		{"licenses", 100, "GBP"},
	}
	got, err := InvoiceTotal(c, lines, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if got != 336.11 {
		t.Fatalf("InvoiceTotal against the real rates service = %s, want 336.11",
			strings.TrimRight(fmt.Sprintf("%.2f", got), "0"))
	}
}
