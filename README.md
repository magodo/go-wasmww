# go-wasmww

A Web Worker abstraction Go, compiled to WebAssembly.

This is based on https://github.com/magodo/go-webworkers/ (forked from https://github.com/hack-pad/go-webworkers), which is a plane Web Worker wrapper for Go.

## Usage

At its basic, it abstracts the `exec.Cmd` structure, to allow the main thread (*the parent process*) to create a web worker (*the child process*), by specifying the WASM URL, together with any arguments or environment variables, if any.

The main types for this basic usage are:

- `WasmWebWorker`: Used in the main thread, for creating a Dedicated Web Worker
- `WasmSharedWebWorker`: Used in the main thread, for creating a Shared Web Worker

For the application running inside the worker, users are expected to use the `worker.GlobalSelf` and `sharedworker.GlobalSelf` in the package `github.com/magodo/go-webworkers`.

On top of this basic abstraction, we've added the support for the Web Worker connections. In that it supports initialization sync, controlling the peer (e.g. close the peer), and piping the stdout/stderr from the Web Worker back to the outside.

The main types for the connections are:

- `WasmWebWorkerConn`: Used in the main thread, for creating a *connected* Dedicated Web Worker
- `SelfConn`: Used in the Dedicated Web Worker
- `WasmSharedWebWorkerConn`: Used in the main thread, for creating a *connected* Shared Web Worker
- `SelfSharedConn`: Used in the Shared Web Worker

## Example

See */examples*.
