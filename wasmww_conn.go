//go:build js && wasm

package wasmww

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"

	"github.com/hack-pad/go-webworkers/worker"
	"github.com/hack-pad/safejs"
	"github.com/magodo/chanio"
)

const CLOSE_EVENT = "__WASMWW_CLOSE__"
const STDOUT_EVENT = "__WASMWW_STDOUT__"
const STDERR_EVENT = "__WASMWW_STDERR__"

// WasmWebWorkerConn is a high level wrapper around the WasmWebWorker, which
// provides a full duplex connection between the web worker.
// On the web worker, it is expected to call the GlobalSelfConn.SetupConn() to build up the connection.
type WasmWebWorkerConn struct {
	Name string
	Path string
	Args []string
	Env  map[string]string

	Stdout io.Writer
	Stderr io.Writer

	pipes []io.Closer

	ww        *WasmWebWorker
	ctx       context.Context
	ctxCancel context.CancelFunc
	eventCh   chan worker.MessageEvent
}

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
	eventCh := make(chan worker.MessageEvent)
	go func() {
		for event := range rawCh {
			if data, err := event.Data(); err == nil {
				if str, err := data.String(); err == nil {
					if str == CLOSE_EVENT {
						conn.ctxCancel()
						continue
					}
					if conn.Stdout != nil {
						if strings.HasPrefix(str, STDOUT_EVENT) {
							if _, err := conn.Stdout.Write([]byte(str[len(STDOUT_EVENT):])); err != nil {
								log.Fatalf("Controller writing to stdout: %v", err)
							}
							continue
						}
					}
					if conn.Stderr != nil {
						if strings.HasPrefix(str, STDERR_EVENT) {
							if _, err := conn.Stderr.Write([]byte(str[len(STDERR_EVENT):])); err != nil {
								log.Fatalf("Controller writing to stderr: %v", err)
							}
							continue
						}
					}
				}
			}
			eventCh <- event
		}
		close(eventCh)

		for _, closer := range conn.pipes {
			closer.Close()
		}

		conn.ww = nil
	}()

	conn.eventCh = eventCh
	return nil
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

func (conn *WasmWebWorkerConn) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return conn.ww.PostMessage(data, transfers)
}

func (conn *WasmWebWorkerConn) Terminate() {
	conn.ww.Terminate()
	conn.ctxCancel()
}

func (conn *WasmWebWorkerConn) EventChannel() <-chan worker.MessageEvent {
	return conn.eventCh
}
