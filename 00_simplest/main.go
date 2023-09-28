package main

import (
	"bytes"
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed modules/*.wasm
var builtinModules embed.FS

type wasmHandler struct {
	builtin    fs.FS
	local      fs.FS
	moduleName string
}

func (wh *wasmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var ts time.Time
	log.Printf("start")
	ctx := context.Background()

	ts = time.Now()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)
	log.Printf("wazero runtime created in %v", time.Since(ts))

	ts = time.Now()
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)
	log.Printf("wasi registered in %v", time.Since(ts))

	// loadModule does its own logging
	wasmObj, err := wh.loadModule(wh.moduleName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ts = time.Now()
	var stdout bytes.Buffer
	config := wazero.NewModuleConfig().WithName(wh.moduleName).WithStdout(&stdout).WithStderr(os.Stderr)
	for key, val := range wh.makeEnviron(r) {
		config = config.WithEnv(key, val)
	}
	log.Printf("module configured in %v", time.Since(ts))

	// also invokes the _start function
	ts = time.Now()
	mod, err = rt.InstantiateWithConfig(ctx, wasmObj, config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("module instantiated in %v", time.Since(ts))

	ts = time.Now()
	mod.Close(ctx)
	log.Printf("module closed in %v", time.Since(ts))

	ts = time.Now()
	_, err = io.Copy(w, &stdout)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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

func (wh *wasmHandler) loadModule(name string) (data []byte, err error) {
	var origin string

	modName := name + ".wasm"

	origin = "local"
	data, err = tryToReadAll(wh.local, modName, origin)

	if err == nil {
		return data, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err // non recoverable
	}

	origin = "builtin"
	data, err = tryToReadAll(wh.builtin, modName, origin)
	return data, err
}

func tryToReadAll(fsys fs.FS, name, origin string) (data []byte, err error) {
	var ts time.Time

	defer func() {
		if err != nil {
			log.Printf("failed to read module %q from %q: %v", name, origin, err)
			return
		}
		log.Printf("loaded module %q from %q modules in %v", name, origin, time.Since(ts))
	}()

	if fsys == nil {
		return nil, fs.ErrNotExist
	}

	ts = time.Now()
	var src fs.File
	src, err = fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	data, err = io.ReadAll(src)
	return data, err
}

func main() {
	var handler string
	var modulesPath string
	var port int
	flag.StringVar(&handler, "handler", "http", "wasm module to serve requests (<name>.wasm)")
	flag.StringVar(&modulesPath, "modules", "modules", "external modules to load (use empty to disable)")
	flag.IntVar(&port, "port", 8080, "port to listen to")
	flag.Parse()

	var localModules fs.FS
	if modulesPath != "" {
		localModules = os.DirFS(modulesPath)
	}

	wh := wasmHandler{
		builtin:    builtinModules,
		local:      localModules,
		moduleName: handler,
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("starting, listen on [%s]", addr)
	defer log.Printf("done!")

	mux := http.NewServeMux()
	mux.HandleFunc("/", wh.ServeHTTP)
	log.Fatal(http.ListenAndServe(addr, mux))
}
