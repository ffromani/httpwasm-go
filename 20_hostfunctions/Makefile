all: build

build: build-guest build-host

build-host:
	go build -o httpwasm loader.go engine.go handler.go main.go

build-guest:
	tinygo build -o modules/echo.wasm -target=wasi -scheduler=none modules/echo.go
	tinygo build -o modules/env.wasm -target=wasi -scheduler=none modules/env.go
	tinygo build -o modules/hello.wasm -target=wasi -scheduler=none modules/hello.go

