package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

type wasmHandler struct {
	engine *wasmEngine
	name   string
}

func (wh *wasmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var ts time.Time
	log.Printf("start")
	ctx := context.Background()

	ts = time.Now()
	stdout, stderr, err := wh.engine.Run(ctx, wh.name, r.Body, wh.makeEnviron(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if stderr != "" {
		log.Printf("module stderr: [%s]", stderr)
	}
	log.Printf("module stdout: [%s]", stdout)

	ts = time.Now()
	fmt.Fprint(w, stdout)
	log.Printf("response sent in %v", time.Since(ts))

	log.Printf("done!")
}

func (wh *wasmHandler) makeEnviron(r *http.Request) map[string]string {
	return map[string]string{
		"HTTP_PATH":   r.URL.Path,
		"HTTP_METHOD": r.Method,
		"HTTP_HOST":   r.Host,
		"HTTP_QUERY":  r.URL.Query().Encode(),
		"REMOTE_ADDR": r.RemoteAddr,
	}
}
