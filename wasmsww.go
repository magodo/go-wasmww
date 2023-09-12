package wasmww

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/sharedworker"
	"github.com/magodo/go-webworkers/types"
)

type WasmSharedWebWorker struct {
	// Name specifies an identifying name for the Shared Web Worker.
	// If this is not specified, `Start` will create a UUIDv4 for it and populate back.
	//
	// This is required in the Connect().
	Name string

	// Path is the path of the WASM to run as the Web Worker.
	//
	// This is ignored in the Connect().
	Path string

	// Args holds command line arguments, including the WASM as Args[0].
	// If the Args field is empty or nil, Run uses {Path}.
	//
	// This is ignored in the Connect().
	Args []string

	// Env specifies the environment of the process.
	// Each entry is of the form "key=value".
	// If Env is nil, the new Web Worker uses the current context's
	// environment.
	// If Env contains duplicate environment keys, only the last
	// value in the slice for each duplicate key is used.
	//
	// This is ignored in the Connect().
	Env []string

	// url represents the web worker script url.
	// This is filled in in the Start(), and is required in the Connect().
	URL string

	worker *sharedworker.SharedWorker
}

func (ww *WasmSharedWebWorker) Start() error {
	workerJS, err := buildWorkerJS(ww.Args, ww.Env, ww.Path)
	if err != nil {
		return err
	}

	if ww.Name == "" {
		ww.Name = uuid.New().String()
	}

	wk, err := sharedworker.NewFromScript(workerJS, ww.Name)
	if err != nil {
		return err
	}

	ww.URL = wk.URL()
	ww.worker = wk

	return nil
}

func (ww *WasmSharedWebWorker) startForConn() error {
	workerJS, err := buildSharedWorkerJS(ww.Args, ww.Env, ww.Path)
	if err != nil {
		return err
	}

	if ww.Name == "" {
		ww.Name = uuid.New().String()
	}

	wk, err := sharedworker.NewFromScript(workerJS, ww.Name)
	if err != nil {
		return err
	}

	ww.URL = wk.URL()
	ww.worker = wk

	return nil
}

func (ww *WasmSharedWebWorker) Connect() error {
	if ww.Name == "" {
		return fmt.Errorf("Name is required when calling Connect()")
	}
	if ww.URL == "" {
		return fmt.Errorf("URL is required when calling Connect()")
	}
	wk, err := sharedworker.New(ww.URL, ww.Name)
	if err != nil {
		return err
	}
	ww.worker = wk
	return nil
}

// PostMessage sends data in a message to the worker, optionally transferring ownership of all items in transfers.
func (ww *WasmSharedWebWorker) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return ww.worker.PostMessage(data, transfers)
}

// Listen sends message events on a channel for events fired by port.postMessage() calls inside the Worker's.
// Stops the listener and closes the channel when ctx is canceled.
func (ww *WasmSharedWebWorker) Listen(ctx context.Context) (<-chan types.MessageEventMessage, error) {
	return ww.worker.Listen(ctx)
}

// Close closes the message port of this worker.
func (ww *WasmSharedWebWorker) Close() error {
	return ww.worker.Close()
}
