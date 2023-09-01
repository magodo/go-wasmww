//go:build js && wasm

package wasmww

import (
	"context"
	"strings"
	"syscall/js"

	"github.com/hack-pad/go-webworkers/worker"
	"github.com/hack-pad/safejs"
)

type GlobalSelfConn struct {
	self *worker.GlobalSelf
}

func SelfConn() (*GlobalSelfConn, error) {
	self, err := worker.Self()
	if err != nil {
		return nil, err
	}
	return &GlobalSelfConn{self: self}, nil
}

type WebWorkerCloseFunc func() error

// SetupConn setup the worker for working with the peering WasmWebWorkerConn.
// The returned eventCh receives the event sent from the peering WasmWebWorkerConn, until the closeFn is called.
// The closeFn is used to instruct the web worker to stop listening the peering, and close the eventCh.
func (s *GlobalSelfConn) SetupConn() (eventCh <-chan worker.MessageEvent, closeFn WebWorkerCloseFunc, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	eventCh, err = s.self.Listen(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	// Rewrite the writeSync JS function to make it not only log to console, but also postMessage to the controller.
	writeSync := js.FuncOf(func(this js.Value, args []js.Value) any {
		jsConsole := js.Global().Get("console")
		decoder := js.Global().Get("TextDecoder").New("utf-8")
		fd, buf := args[0], args[1]
		var outputBuffer string
		tmpBuf := decoder.Call("decode", buf).String()
		outputBuffer += tmpBuf
		nl := strings.LastIndex(outputBuffer, "\n")
		if nl != -1 {
			msg := outputBuffer[:nl]
			jsConsole.Call("log", js.ValueOf(msg))
			switch fd.Int() {
			case 1:
				s.PostMessage(safejs.Safe(js.ValueOf(STDOUT_EVENT+msg+"\n")), nil)
			case 2:
				s.PostMessage(safejs.Safe(js.ValueOf(STDERR_EVENT+msg+"\n")), nil)
			}
			outputBuffer = outputBuffer[nl+1:]
		}
		return buf.Get("length")
	})
	jsFS := js.Global().Get("fs")
	jsFS.Set("writeSync", writeSync)

	// Notify the controller that this worker has started listening
	if err := s.self.PostMessage(safejs.Null(), nil); err != nil {
		cancel()
		return nil, nil, err
	}

	closeFunc := func() error {
		cancel()
		msg, err := safejs.ValueOf(CLOSE_EVENT)
		if err != nil {
			return err
		}
		if err := s.self.PostMessage(msg, nil); err != nil {
			return err
		}
		return s.self.Close()
	}

	return eventCh, closeFunc, nil
}

func (s *GlobalSelfConn) Name() (string, error) {
	return s.self.Name()
}

func (s *GlobalSelfConn) PostMessage(message safejs.Value, transfers []safejs.Value) error {
	return s.self.PostMessage(message, transfers)
}
