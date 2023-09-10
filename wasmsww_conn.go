package wasmww

import (
	"context"
	"fmt"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/types"
)

// WasmSharedWebWorkerConn is a high level wrapper around the WasmSharedWebWorker, which
// provides a full duplex connection between the web worker.
// On the web worker, it is expected to call the SelfSharedConn.SetupConn() to build up the connection.
type WasmSharedWebWorkerConn struct {
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

	// URL represents the web worker script URL.
	// This is populated in in the Start(), and is required in the Connect().
	URL string

	ww      *WasmSharedWebWorker
	eventCh chan types.MessageEventMessage
	closeCh chan any
}

// Start starts a new Shared Web Worker. It spins up a goroutine to receive the events from the Web Worker,
// and exposes a channel for consuming those events, which can be accessed by the `EventChannel()` method.
// It will fail if the Shared Web Worker already exists. In this case, use Connect() instead.
func (conn *WasmSharedWebWorkerConn) Start() (*WasmSharedWebWorkerConsoleConn, error) {
	// The first connection to the web worker is for the stdout/stderr
	consoleConn := &WasmSharedWebWorkerConsoleConn{
		Name: conn.Name,
		Path: conn.Path,
		Args: conn.Args,
		Env:  conn.Env,
	}

	if err := consoleConn.start(); err != nil {
		return nil, err
	}
	if conn.Name == "" {
		conn.Name = consoleConn.Name
	}
	conn.URL = consoleConn.URL

	if err := conn.Connect(); err != nil {
		return nil, err
	}
	return consoleConn, nil
}

// Connect creates a new connection to an active Shared Web Worker.
func (conn *WasmSharedWebWorkerConn) Connect() (err error) {
	ww := &WasmSharedWebWorker{
		Name: conn.Name,
		URL:  conn.URL,
	}

	if err := ww.Connect(); err != nil {
		return err
	}
	conn.ww = ww

	ctx, ctxCancel := context.WithCancel(context.Background())

	rawCh, err := ww.Listen(ctx)
	if err != nil {
		ctxCancel()
		return err
	}

	defer func() {
		if err != nil {
			ctxCancel()
		}
	}()

	// Wait for the sync message
	if _, ok := <-rawCh; !ok {
		return fmt.Errorf("console channel closed (due to ctx canceled)")
	}

	// Create a channel to relay the event from the onmessage channel to the consuming channel,
	// except it will cancel the listening context and close the channel when the worker closes.
	eventCh := make(chan types.MessageEventMessage)
	closeCh := make(chan any)
	go func() {
		for event := range rawCh {
			if data, err := event.Data(); err == nil {
				if str, err := data.String(); err == nil {
					if str == CLOSE_EVENT {
						ctxCancel()
						continue
					}
				}
			}
			eventCh <- event
		}
		close(closeCh)
		close(eventCh)

		conn.ww = nil
	}()

	conn.eventCh = eventCh
	conn.closeCh = closeCh
	return nil
}

// Wait waits for the controller's internal event loop to quit. This can be caused by the worker closes itself or calling the Close().
func (conn *WasmSharedWebWorkerConn) Wait() {
	<-conn.closeCh
}

// PostMessage sends data in a message to the worker, optionally transferring ownership of all items in transfers.
func (conn *WasmSharedWebWorkerConn) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return conn.ww.PostMessage(data, transfers)
}

// EventChannel returns the channel that receives events sent from the Web Worker.
func (conn *WasmSharedWebWorkerConn) EventChannel() <-chan types.MessageEventMessage {
	return conn.eventCh
}
