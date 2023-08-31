//go:build js && wasm

package wasmww

import (
	"context"

	"github.com/hack-pad/go-webworkers/worker"
	"github.com/hack-pad/safejs"
)

const CLOSE_EVENT = "__WASMWW_CLOSE__"

// WasmWebWorkerConn is a high level wrapper around the WasmWebWorker, which
// provides a full duplex connection between the web worker.
// On the web worker, it is expected to call the GlobalSelfConn.SetupConn() to build up the connection.
type WasmWebWorkerConn struct {
	Name string
	Path string
	Args []string
	Env  map[string]string

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
				}
			}
			eventCh <- event
		}
		close(eventCh)
	}()

	conn.eventCh = eventCh
	return nil
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
