package main

import (
	"runtime"
	"unsafe"
)

//go:wasmimport httpwasm oputs
func putStringStdout(bufPtr, bufLen uint32)

//go:export run
func run() {
	msg := "hello, WASI guest\n"
	bufPtr, bufLen := stringToPtr(msg)
	putStringStdout(bufPtr, bufLen)
	runtime.KeepAlive(msg)
}

func stringToPtr(s string) (uint32, uint32) {
	ptr := unsafe.Pointer(unsafe.StringData(s))
	return uint32(uintptr(ptr)), uint32(len(s))
}

func main() {}
