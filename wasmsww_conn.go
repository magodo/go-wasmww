package wasmww

import (
	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/types"
)

// WasmSharedWebWorkerConn is a high level wrapper around the WasmSharedWebWorker, which
// provides a full duplex connection between the web worker.
// On the web worker, it is expected to call the SelfSharedConn.SetupConn() to build up the connection.
type WasmSharedWebWorkerConn struct {
	// Name specifies an identifying name for the Shared Web Worker.
	// If this is not specified, `Start` will create a UUIDv4 for it and populate back.
	Name string

	// Path is the path of the WASM to run as the Web Worker.
	Path string

	// Args holds command line arguments, including the WASM as Args[0].
	// If the Args field is empty or nil, Run uses {Path}.
	Args []string

	// Env specifies the environment of the process.
	// Each entry is of the form "key=value".
	// If Env is nil, the new Web Worker uses the current context's
	// environment.
	// If Env contains duplicate environment keys, only the last
	// value in the slice for each duplicate key is used.
	Env []string

	// url represents the web worker script url.
	// This is populated in the Start().
	url string

	ww        *WasmSharedWebWorker
	mgmtPort  *WasmSharedWebWorkerMgmtConn
	closeFunc WebWorkerCloseFunc
	eventCh   chan types.MessageEventMessage
	closeCh   chan any
}

// Start starts a new Shared Web Worker. It spins up a goroutine to receive the events from the Web Worker,
// and exposes a channel for consuming those events, which can be accessed by the `EventChannel()` method.
// It will fail if the Shared Web Worker already exists. In this case, use Connect() instead.
// The returned WasmSharedWebWorkerMgmtConn is a special connection, that is used to manage the web worker, or
// create another WasmSharedWebWorkerConn to this web worker via its Connect() method.
func (conn *WasmSharedWebWorkerConn) Start() (*WasmSharedWebWorkerMgmtConn, error) {
	// The first connection to the web worker is for the stdout/stderr
	mgmtConn := &WasmSharedWebWorkerMgmtConn{
		name: conn.Name,
		path: conn.Path,
		args: conn.Args,
		env:  conn.Env,
	}

	if err := mgmtConn.start(); err != nil {
		return nil, err
	}
	if conn.Name == "" {
		conn.Name = mgmtConn.name
	}
	conn.url = mgmtConn.url

	newConn, err := mgmtConn.Connect()
	if err != nil {
		return nil, err
	}
	*conn = *newConn
	return mgmtConn, nil
}

// Wait waits for the controller's internal event loop to quit. This can be caused by the worker closes itself.
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
