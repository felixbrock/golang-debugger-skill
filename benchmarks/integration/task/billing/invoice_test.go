package billing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockRates serves the rates API the way the billing team understands it:
// GET /rate?from=X&to=Y -> {"rate": <direct conversion rate>}.
func mockRates(t *testing.T, rates map[string]float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("from") + "/" + r.URL.Query().Get("to")
		rate, ok := rates[key]
		if !ok {
			http.Error(w, "unknown pair", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `{"rate": %v}`, rate)
	}))
}

func TestConvert(t *testing.T) {
	srv := mockRates(t, map[string]float64{"USD/EUR": 0.90})
	defer srv.Close()
	c := &RateClient{Base: srv.URL}
	got, err := c.Convert(200, "USD", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	if got != 180 {
		t.Fatalf("Convert(200, USD, EUR) = %v, want 180", got)
	}
}

func TestInvoiceTotal(t *testing.T) {
	srv := mockRates(t, map[string]float64{
		"USD/USD": 1,
		"EUR/USD": 1.1111,
		"GBP/USD": 1.25,
	})
	defer srv.Close()
	c := &RateClient{Base: srv.URL}
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
		t.Fatalf("InvoiceTotal = %v, want 336.11", got)
	}
}
