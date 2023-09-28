package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
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
	"github.com/tetratelabs/wazero/sys"
)

//go:embed modules/*.wasm
var builtinModules embed.FS

type wasmLoader struct {
	builtin fs.FS
	local   fs.FS
}

func (wl *wasmLoader) LoadModule(name string) (data []byte, err error) {
	var origin string

	modName := name + ".wasm"

	origin = "local"
	data, err = tryToReadAll(wl.local, modName, origin)

	if err == nil {
		return data, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err // non recoverable
	}

	origin = "builtin"
	data, err = tryToReadAll(wl.builtin, modName, origin)
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

// xref: https://github.com/tetratelabs/wazero/issues/985
type wasmEngine struct {
	code wazero.CompiledModule
	rt   wazero.Runtime
}

func (we *wasmEngine) Close(ctx context.Context) error {
	return we.rt.Close(ctx)
}

func newWasmEngine(ctx context.Context, wasmObj []byte) (*wasmEngine, error) {
	var ts time.Time

	ts = time.Now()
	rt := wazero.NewRuntime(ctx)
	log.Printf("wazero runtime created in %v", time.Since(ts))

	ts = time.Now()
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)
	log.Printf("wasi registered in %v", time.Since(ts))

	ts = time.Now()
	code, err := rt.CompileModule(ctx, wasmObj)
	if err != nil {
		return nil, err
	}
	log.Printf("module compiled in %v", time.Since(ts))

	return &wasmEngine{
		rt:   rt,
		code: code,
	}, nil
}

func (we *wasmEngine) Run(ctx context.Context, name string, stdin io.Reader, env map[string]string) (string, string, error) {
	var ts time.Time
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ts = time.Now()
	config := wazero.NewModuleConfig().WithName(name).WithStdout(&stdout).WithStderr(&stderr)
	if stdin != nil {
		config = config.WithStdin(stdin)
	}
	for key, val := range env {
		config = config.WithEnv(key, val)
	}
	log.Printf("module configured in %v", time.Since(ts))

	ts = time.Now()
	// also invokes the _start function
	mod, err := we.rt.InstantiateModule(ctx, we.code, config)
	log.Printf("module instantiated in %v", time.Since(ts))
	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr); exitErr.ExitCode() != 0 {
			return "", "", err
		} else {
			return "", "", fmt.Errorf("instantiation error: %w", err)
		}
	}

	mod.Close(ctx)

	ts = time.Now()
	log.Printf("module closed in %v", time.Since(ts))

	return stdout.String(), stderr.String(), nil
}

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
