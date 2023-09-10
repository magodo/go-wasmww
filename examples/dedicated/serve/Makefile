serve: control worker
	ln -sf "$$(go env GOROOT)/misc/wasm/wasm_exec.js" .
control: ../control/main.go
	GOOS=js GOARCH=wasm go build -C ../control -o ../serve/main.wasm
worker: ../worker/main.go
	GOOS=js GOARCH=wasm go build -C ../worker -o ../serve/hello.wasm


