package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

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
