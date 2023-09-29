package main

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	"github.com/tidwall/gjson"
)

//go:export run
func run() {
	puts(validate(gets()))
}

type validation struct {
	fieldPath        string
	expectedValue    string
	errorDescription string
}

func validate(jsonText string) string {
	validations := []validation{
		{
			fieldPath:        "name.last",
			expectedValue:    "Doe",
			errorDescription: "mismatched last name",
		},
	}
	for _, validation := range validations {
		gotValue := gjson.Get(jsonText, validation.fieldPath).String()
		if gotValue != validation.expectedValue {
			return makeError(1, validation.fieldPath, gotValue, validation.errorDescription)
		}
	}
	return makeSuccess()
}

func makeSuccess() string {
	return `{"status":"success", "reason":{}}`
}

func makeError(code int, field, value, desc string) string {
	return fmt.Sprintf(`{"status":"error", "reason":{"code":%d,"field":%q,"value":%q,"description":%q}}`, code, field, value, desc)
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
