package main

import (
	"reflect"
	"runtime"
	"unsafe"
)

//go:export run
func run() {
	got := gets()
	msg := "hello, " + got + "\n"
	puts(msg)
}

//go:wasmimport httpwasm oputs
func putStringStdout(bufPtr, bufLen uint32)

//go:wasmimport httpwasm igets
func getStringStdin() uint64

func gets() string {
	ret := getStringStdin()
	ptr := uint32(ret >> 32)
	size := uint32(ret)
	data := ptrToBytes(ptr, size)
	return string(data)
}

func puts(s string) {
	ptr, size := stringToPtr(s)
	putStringStdout(ptr, size)
	runtime.KeepAlive(s)
}

func stringToPtr(s string) (uint32, uint32) {
	ptr := unsafe.Pointer(unsafe.StringData(s))
	return uint32(uintptr(ptr)), uint32(len(s))
}

func ptrToBytes(ptr, size uint32) []byte {
	var b []byte
	s := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	s.Len = uintptr(size)
	s.Cap = uintptr(size)
	s.Data = uintptr(ptr)
	return b
}

func main() {}
