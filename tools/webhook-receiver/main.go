// A simple webhook receiver for testing JobPlane callbacks.
// Logs every incoming request with headers, body, and timestamp.
//
// Usage: go run tools/webhook-receiver/main.go [port]
// Default port: 9999
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

var requestCount int64

func main() {
	port := "9999"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		// Pretty-print JSON body
		var prettyBody interface{}
		var bodyStr string
		if err := json.Unmarshal(body, &prettyBody); err == nil {
			pretty, _ := json.MarshalIndent(prettyBody, "  ", "  ")
			bodyStr = string(pretty)
		} else {
			bodyStr = string(body)
		}

		fmt.Println()
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("📩 Webhook #%d received at %s\n", count, time.Now().Format("15:04:05"))
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("  Method:       %s %s\n", r.Method, r.URL.Path)
		fmt.Printf("  Content-Type: %s\n", r.Header.Get("Content-Type"))
		fmt.Printf("  User-Agent:   %s\n", r.Header.Get("User-Agent"))

		// Show custom headers (skip standard ones)
		for key, values := range r.Header {
			switch key {
			case "Content-Type", "User-Agent", "Content-Length", "Accept-Encoding":
				continue
			default:
				fmt.Printf("  %s: %s\n", key, values[0])
			}
		}

		fmt.Printf("  Body:\n  %s\n", bodyStr)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
	})

	log.Printf("🎯 Webhook receiver listening on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
