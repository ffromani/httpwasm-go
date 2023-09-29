package main

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

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
	mod, err := rt.InstantiateWithConfig(ctx, wasmObj, config)
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
