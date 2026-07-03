// rates is the "third-party" currency-rate service for the integration
// benchmark. Its compiled binary is given to the agent; this source is NOT.
//
// The contract quirk the benchmark hinges on: the service stores each pair
// in one canonical direction only. Asked for the reverse direction, it
// returns the canonical rate plus "inverse": true, expecting the caller to
// divide. Clients that ignore the flag multiply by the wrong rate.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

var canonical = map[string]float64{
	"USD/EUR": 0.90,
	"USD/GBP": 0.80,
	"EUR/JPY": 160.0,
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7301", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/rate", func(w http.ResponseWriter, r *http.Request) {
		from := strings.ToUpper(r.URL.Query().Get("from"))
		to := strings.ToUpper(r.URL.Query().Get("to"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case from == "" || to == "":
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "from and to are required"})
		case from == to:
			json.NewEncoder(w).Encode(map[string]any{"rate": 1.0})
		default:
			if rate, ok := canonical[from+"/"+to]; ok {
				json.NewEncoder(w).Encode(map[string]any{"rate": rate})
				return
			}
			if rate, ok := canonical[to+"/"+from]; ok {
				json.NewEncoder(w).Encode(map[string]any{"rate": rate, "inverse": true})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "unknown pair " + from + "/" + to})
		}
	})

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rates listening on %s\n", ln.Addr())
	log.Fatal(http.Serve(ln, mux))
}
