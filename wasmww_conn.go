//go:build js && wasm

package wasmww

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"

	"github.com/hack-pad/safejs"
	"github.com/magodo/chanio"
	"github.com/magodo/go-webworkers/types"
)

const CLOSE_EVENT = "__WASMWW_CLOSE__"
const STDOUT_EVENT = "__WASMWW_STDOUT__"
const STDERR_EVENT = "__WASMWW_STDERR__"

// WasmWebWorkerConn is a high level wrapper around the WasmWebWorker, which
// provides a full duplex connection between the web worker.
// On the web worker, it is expected to call the GlobalSelfConn.SetupConn() to build up the connection.
type WasmWebWorkerConn struct {
	// Name specifies an identifying name for the DedicatedWorkerGlobalScope representing the scope of the worker, which is mainly useful for debugging purposes.
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

	Stdout io.Writer
	Stderr io.Writer

	pipes []io.Closer

	ww        *WasmWebWorker
	ctx       context.Context
	ctxCancel context.CancelFunc
	eventCh   chan types.MessageEvent
	closeCh   chan any
}

// Start starts a new Web Worker. It spins up a goroutine to receive the events from the Web Worker,
// and exposes a channel for consuming those events, which can be accessed by the `EventChannel()` method.
func (conn *WasmWebWorkerConn) Start() error {
	ww := &WasmWebWorker{
		Name: conn.Name,
		Path: conn.Path,
		Args: conn.Args,
		Env:  conn.Env,
	}
	if err := ww.Start(); err != nil {
		return err
	}
	if conn.Name == "" {
		conn.Name = ww.Name
	}
	conn.ww = ww
	conn.ctx, conn.ctxCancel = context.WithCancel(context.Background())

	rawCh, err := ww.Listen(conn.ctx)
	if err != nil {
		return err
	}

	// Wait for the worker's initial sync event, which indicates the worker is ready to receive events.
	// NOTE: Since JS is single-threaded, we are careful to avoid introducing a switch point until here,
	// so that we ensure the controller started listening before the worker actually sends the initial sync event back,
	// as otherwise, this event will be lost.
	<-rawCh

	// Create a channel to relay the event from the onmessage channel to the consuming channel,
	// except it will cancel the listening context and close the channel when the worker closes.
	eventCh := make(chan types.MessageEvent)
	closeCh := make(chan any)
	go func() {
		for event := range rawCh {
			if data, err := event.Data(); err == nil {
				if str, err := data.String(); err == nil {
					if str == CLOSE_EVENT {
						conn.ctxCancel()
						continue
					}
					if strings.HasPrefix(str, STDOUT_EVENT) {
						if conn.Stdout != nil {
							if _, err := conn.Stdout.Write([]byte(str[len(STDOUT_EVENT):])); err != nil {
								log.Fatalf("Controller writing to stdout: %v", err)
							}
						}
						continue
					}
					if strings.HasPrefix(str, STDERR_EVENT) {
						if conn.Stderr != nil {
							if _, err := conn.Stderr.Write([]byte(str[len(STDERR_EVENT):])); err != nil {
								log.Fatalf("Controller writing to stderr: %v", err)
							}
						}
						continue
					}
				}
			}
			eventCh <- event
		}
		close(closeCh)
		close(eventCh)

		for _, closer := range conn.pipes {
			closer.Close()
		}

		conn.ww = nil
	}()

	conn.eventCh = eventCh
	conn.closeCh = closeCh
	return nil
}

// Wait waits for the controller's internal event loop to quit. This can be caused by either worker closes itself, or controler calls `Terminate`.
func (conn *WasmWebWorkerConn) Wait() {
	<-conn.closeCh
}

// StdoutPipe returns a channel that will be connected to the worker's
// standard output when the worker starts.
//
// Once the worker is exited (no matter closed by itself or terminated),
// the channel will be closed by the WasmWebWorkerConn. So no need to close
// the channel themselves.
func (conn *WasmWebWorkerConn) StdoutPipe() (io.ReadCloser, error) {
	if conn.Stdout != nil {
		return nil, errors.New("wasmww: Stdout already set")
	}
	if conn.ww != nil {
		return nil, errors.New("wasmww: StdoutPipe after worker started")
	}
	r, w, err := chanio.Pipe()
	if err != nil {
		return nil, err
	}
	conn.Stdout = w
	conn.pipes = append(conn.pipes, w)
	return r, nil
}

// StderrPipe returns a channel that will be connected to the worker's
// standard error when the worker starts.
//
// Once the worker is exited (no matter closed by itself or terminated),
// the channel will be closed by the WasmWebWorkerConn. So no need to close
// the channel themselves.
func (conn *WasmWebWorkerConn) StderrPipe() (io.ReadCloser, error) {
	if conn.Stderr != nil {
		return nil, errors.New("wasmww: Stderr already set")
	}
	if conn.ww != nil {
		return nil, errors.New("wasmww: StderrPipe after worker started")
	}
	r, w, err := chanio.Pipe()
	if err != nil {
		return nil, err
	}
	conn.Stderr = w
	conn.pipes = append(conn.pipes, w)
	return r, nil
}

// PostMessage sends data in a message to the worker, optionally transferring ownership of all items in transfers.
func (conn *WasmWebWorkerConn) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return conn.ww.PostMessage(data, transfers)
}

// Terminate immediately terminates the Worker. Meanwhile, it stops the internal event loop, which makes the `Wait` to return.
func (conn *WasmWebWorkerConn) Terminate() {
	conn.ww.Terminate()
	conn.ctxCancel()
}

// EventChannel returns the channel that receives events sent from the Web Worker.
func (conn *WasmWebWorkerConn) EventChannel() <-chan types.MessageEvent {
	return conn.eventCh
}
