all: build

build: build-guest build-host

build-host:
	go build -o httpwasm loader.go handler.go main.go

build-guest:
	GOOS=wasip1 GOARCH=wasm go build -o modules/env.wasm modules/env.go
	GOOS=wasip1 GOARCH=wasm go build -o modules/hello.wasm modules/hello.go

