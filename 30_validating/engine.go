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
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

const (
	runFnName = "run"
)

// xref: https://github.com/tetratelabs/wazero/issues/985
type wasmEngine struct {
	code     wazero.CompiledModule
	rt       wazero.Runtime
	hostMod  api.Module
	guestMod api.Module
	stack    []uint64
	mallocFn api.Function
	freeFn   api.Function
	runFn    api.Function
}

func (we *wasmEngine) Close(ctx context.Context) error {
	// hostMod closed when we close the runtime
	// guestMod closed when we close the runtime
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
	hostMod, err := rt.NewHostModuleBuilder("httpwasm").
		NewFunctionBuilder().WithFunc(igets).Export("igets").
		NewFunctionBuilder().WithFunc(eputs).Export("eputs").
		NewFunctionBuilder().WithFunc(oputs).Export("oputs").
		Instantiate(ctx)
	log.Printf("host module instantiated in in %v", time.Since(ts))

	ts = time.Now()
	code, err := rt.CompileModule(ctx, wasmObj)
	if err != nil {
		return nil, err
	}
	log.Printf("module compiled in %v", time.Since(ts))

	ts = time.Now()
	config := wazero.NewModuleConfig().WithName("httpwasm_guest") // TODO
	// also invokes the _start function
	guestMod, err := rt.InstantiateModule(ctx, code, config)
	log.Printf("module instantiated in %v (%v)", time.Since(ts), err)
	if err != nil {
		rt.Close(ctx) // don't leak

		var exitErr *sys.ExitError
		if errors.As(err, &exitErr); exitErr.ExitCode() != 0 {
			return nil, err
		} else {
			return nil, fmt.Errorf("instantiation error: %w", err)
		}
	}

	ts = time.Now()
	mallocFn := guestMod.ExportedFunction("malloc")
	if mallocFn == nil {
		rt.Close(ctx) // don't leak
		return nil, fmt.Errorf("failed to lookup function %q", "malloc")
	}
	freeFn := guestMod.ExportedFunction("free")
	if freeFn == nil {
		rt.Close(ctx) // don't leak
		return nil, fmt.Errorf("failed to lookup function %q", "free")
	}

	ts = time.Now()
	runFn := guestMod.ExportedFunction(runFnName)
	if runFn == nil {
		rt.Close(ctx) // don't leak
		return nil, fmt.Errorf("failed to lookup function %q", runFnName)
	}
	log.Printf("function looked up in %v", time.Since(ts))

	for key, _ := range guestMod.ExportedFunctionDefinitions() {
		log.Printf("[%s] -> %q", guestMod.Name(), key)
	}

	return &wasmEngine{
		rt:       rt,
		code:     code,
		hostMod:  hostMod,
		guestMod: guestMod,
		stack:    make([]uint64, 16), // overkill
		mallocFn: mallocFn,
		freeFn:   freeFn,
		runFn:    runFn,
	}, nil
}

func (we *wasmEngine) Run(ctx context.Context, name string, stdin io.Reader, env map[string]string) (string, string, error) {
	var ts time.Time

	ts = time.Now()
	// TODO
	guestCtx, cdata := putCallData(ctx, we)

	stdinData, err := io.ReadAll(stdin)
	if err != nil {
		return "", "", err
	}
	stdinData = append(stdinData, byte('\x00'))
	log.Printf("stdin: [%s]", string(stdinData))
	_, err = cdata.stdin.Write(stdinData)
	if err != nil {
		return "", "", err
	}
	log.Printf("run function prepared in %v", time.Since(ts))

	ts = time.Now()
	// run is like main: take no args, returns no value. Still, we pass stack explicitly because why not
	err = we.runFn.CallWithStack(guestCtx, we.stack)
	log.Printf("run function executed in %v (%v)", time.Since(ts), err)

	ts = time.Now()
	dealloc(cdata)
	log.Printf("dealloc in %v (%v)", time.Since(ts), err)

	return cdata.stdout.String(), cdata.stderr.String(), err
}

func dealloc(cdata *callData) error {
	ctx := context.Background() // TODO
	count := 0
	for len(cdata.allocs) > 0 {
		ptr := cdata.allocs[0]
		cdata.allocs = cdata.allocs[1:]

		_, err := cdata.freeFn.Call(ctx, uint64(ptr))
		if err != nil {
			return err
		}
		count++
	}
	log.Printf("dealloc: %d pointers", count)
	return nil
}

func igets(ctx context.Context, mod api.Module) uint64 {
	cdata := getCallData(ctx)
	dealloc(cdata)

	stdinData, err := cdata.stdin.ReadBytes('\x00')
	if err != nil {
		log.Printf("stdin readstring failed: %v", err)
		return 0
	}

	results, err := cdata.mallocFn.Call(ctx, uint64(len(stdinData)))
	if err != nil {
		log.Printf("malloc failed: %v", err)
		return 0
	}

	ptr := results[0]
	size := uint64(len(stdinData))
	cdata.allocs = append(cdata.allocs, uint32(ptr))

	if ok := mod.Memory().Write(uint32(ptr), stdinData); !ok {
		log.Printf("memory write ed: %v", err)
		return 0
	}

	return (uint64(ptr) << uint64(32)) | uint64(size)
}

func eputs(ctx context.Context, mod api.Module, bufPtr uint32, bufLen uint32) {
	cdata := getCallData(ctx)

	bytes, ok := mod.Memory().Read(bufPtr, bufLen)
	if !ok {
		log.Printf("[%v]: eputs: unable to read wasm memory", mod.Name())
		return
	}

	cdata.stderr.Write(bytes)
}

func oputs(ctx context.Context, mod api.Module, bufPtr uint32, bufLen uint32) {
	cdata := getCallData(ctx)

	bytes, ok := mod.Memory().Read(bufPtr, bufLen)
	if !ok {
		log.Printf("[%v]: oputs: unable to read wasm memory", mod.Name())
		return
	}

	cdata.stdout.Write(bytes)
}

type callData struct {
	stdin    bytes.Buffer
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	mallocFn api.Function
	freeFn   api.Function
	allocs   []uint32
	// TODO: env
}

type callDataKey struct{}

func getCallData(ctx context.Context) *callData {
	return ctx.Value(callDataKey{}).(*callData)
}

func putCallData(ctx context.Context, we *wasmEngine) (context.Context, *callData) {
	cdata := callData{
		mallocFn: we.mallocFn,
		freeFn:   we.freeFn,
	}
	ctx = context.WithValue(ctx, callDataKey{}, &cdata)
	return ctx, &cdata
}
