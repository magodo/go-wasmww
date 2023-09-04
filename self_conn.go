//go:build js && wasm

package wasmww

import (
	"context"
	"io"
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

// RedirectStd redirects the stdout and stderr to the given writers. works as same as `os.stderr=...`
func (*GlobalSelfConn) RedirectStd(stdout_w, stderr_w io.Writer) {
	writeSync := js.FuncOf(func(this js.Value, args []js.Value) any {
		decoder := js.Global().Get("TextDecoder").New("utf-8")
		fd, buf := args[0], args[1]
		var outputBuffer string
		tmpBuf := decoder.Call("decode", buf).String()
		outputBuffer += tmpBuf
		nl := strings.LastIndex(outputBuffer, "\n")
		if nl != -1 {
			msg := outputBuffer[:nl]
			switch fd.Int() {
			case 1:
				_, err := stdout_w.Write([]byte(msg))
				if err != nil {
					panic(err)
				}
			case 2:
				_, err := stderr_w.Write([]byte(msg))
				if err != nil {
					panic(err)
				}
			}
			outputBuffer = outputBuffer[nl+1:]
		}
		return buf.Get("length")
	})
	jsFS := js.Global().Get("fs")
	jsFS.Set("writeSync", writeSync)
}

func (s *GlobalSelfConn) Name() (string, error) {
	return s.self.Name()
}

func (s *GlobalSelfConn) PostMessage(message safejs.Value, transfers []safejs.Value) error {
	return s.self.PostMessage(message, transfers)
}
