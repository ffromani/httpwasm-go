package main

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"os"
	"time"
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
