all: build

build: build-guest build-host

build-host:
	go build -o httpwasm loader.go engine.go handler.go main.go

build-guest:
	tinygo build -o modules/validate.wasm -target=wasi -scheduler=none modules/validate.go

