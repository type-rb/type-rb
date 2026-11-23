// Synthetic test server only; ordinary compiler and application builds do not use Go.
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	root := os.Args[1]
	body, err := os.ReadFile(filepath.Join(root, "keys.json"))
	if err != nil {
		panic(err)
	}
	log, err := os.Create(filepath.Join(root, "requests.jsonl"))
	if err != nil {
		panic(err)
	}
	defer log.Close()
	var lock sync.Mutex
	record := func(value any) {
		lock.Lock()
		defer lock.Unlock()
		if err := json.NewEncoder(log).Encode(value); err != nil {
			panic(err)
		}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(map[string]any{"event": "request", "path": r.URL.Path, "protocol": r.Proto, "alpn": r.TLS.NegotiatedProtocol})
		if r.URL.Path == "/required/keys" && r.ProtoMajor != 2 {
			w.WriteHeader(505)
			return
		}
		if r.URL.Path == "/redirect/keys" {
			w.Header().Set("Location", "/required/keys")
			w.WriteHeader(302)
			return
		}
		if r.URL.Path == "/oversized/keys" {
			_, _ = w.Write([]byte(strings.Repeat(" ", (1<<20)+1)))
			return
		}
		if r.URL.Path == "/reset/keys" {
			w.WriteHeader(200)
			_, _ = w.Write(body[:8])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gzip/keys" {
			w.Header().Set("Content-Encoding", "gzip")
			writer := gzip.NewWriter(w)
			_, _ = writer.Write(body)
			_ = writer.Close()
			return
		}
		if r.URL.Path == "/stall/keys" {
			w.WriteHeader(200)
			_, _ = w.Write(body[:8])
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				record(map[string]any{"event": "cancelled", "path": r.URL.Path})
			case <-time.After(7 * time.Second):
				record(map[string]any{"event": "stall-completed", "path": r.URL.Path})
			}
			return
		}
		_, _ = w.Write(body)
	})
	h2 := httptest.NewUnstartedServer(handler)
	h2.EnableHTTP2 = true
	h2.StartTLS()
	defer h2.Close()
	h1 := httptest.NewUnstartedServer(handler)
	h1.StartTLS()
	defer h1.Close()
	cert := filepath.Join(root, "ca.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: h2.TLS.Certificates[0].Certificate[0]}), 0600); err != nil {
		panic(err)
	}
	value, _ := json.Marshal(map[string]string{"h2": h2.URL, "h1": h1.URL, "certificate": cert})
	fmt.Println(string(value))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
}
