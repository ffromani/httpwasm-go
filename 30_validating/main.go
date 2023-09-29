package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
)

func main() {
	var handler string
	var modulesPath string
	var port int
	flag.StringVar(&handler, "handler", "validate", "wasm module to serve requests (<name>.wasm)")
	flag.StringVar(&modulesPath, "modules", "modules", "external modules to load (use empty to disable)")
	flag.IntVar(&port, "port", 8080, "port to listen to")
	flag.Parse()

	var localModules fs.FS
	if modulesPath != "" {
		localModules = os.DirFS(modulesPath)
	}

	wl := wasmLoader{
		builtin: builtinModules,
		local:   localModules,
	}

	// loadModule does its own logging
	wasmObj, err := wl.LoadModule(handler)
	if err != nil {
		log.Fatalf("error loading %q: %v", handler, err)
	}

	ctx := context.Background()

	we, err := newWasmEngine(ctx, wasmObj)
	if err != nil {
		log.Fatalf("error creating engine: %v", err)
	}
	defer we.Close(ctx)

	wh := wasmHandler{
		engine: we,
		name:   handler,
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("starting, listen on [%s]", addr)
	defer log.Printf("done!")

	mux := http.NewServeMux()
	mux.HandleFunc("/", wh.ServeHTTP)
	log.Fatal(http.ListenAndServe(addr, mux))
}
