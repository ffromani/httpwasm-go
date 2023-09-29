all: build

build: build-guest build-host

build-host:
	go build -o httpwasm loader.go engine.go handler.go main.go

build-guest:
	GOOS=wasip1 GOARCH=wasm go build -o modules/echo.wasm modules/echo.go
	GOOS=wasip1 GOARCH=wasm go build -o modules/env.wasm modules/env.go
	GOOS=wasip1 GOARCH=wasm go build -o modules/hello.wasm modules/hello.go

