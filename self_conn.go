//go:build js && wasm

package wasmww

import (
	"context"
	"fmt"
	"io"
	"strings"
	"syscall/js"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/types"
	"github.com/magodo/go-webworkers/worker"
)

type GlobalSelfConn struct {
	self *worker.GlobalSelf

	// originWriteSync stores the original js.Func of the "writeSync" from the Go glue file.
	//
	// The reason why not just "re-implement" the "same" version in Go when redirecting write to console,
	// is that there is a wierd issue that although the "same" implementation in Go works in most cases,
	// when the goroutine running in the current context panics, it will print the `too much recursion` error,
	// instead of printing the rewind stack.
	// Persumably, this is due to though the implementations are the same logically, they are different that
	// the Go version lives in the Go world. When the Go program panics, any registered function in the Go
	// world won't be accessible. Whilst, the JS one (lives in the glue code) is still accessible.
	originWriteSync js.Value
}

func SelfConn() (*GlobalSelfConn, error) {
	self, err := worker.Self()
	if err != nil {
		return nil, err
	}
	return &GlobalSelfConn{
		self:            self,
		originWriteSync: js.Global().Get("fs").Get("writeSync"),
	}, nil
}

type WebWorkerCloseFunc func() error

// SetupConn setup the worker for working with the peering WasmWebWorkerConn.
// The returned eventCh receives the event sent from the peering WasmWebWorkerConn, until the closeFn is called.
// The closeFn is used to instruct the web worker to stop listening the peering, and close the eventCh.
func (s *GlobalSelfConn) SetupConn() (eventCh <-chan types.MessageEvent, closeFn WebWorkerCloseFunc, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	eventCh, err = s.self.Listen(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	// Redirect stdout/stderr to the controller, instead of printing to the JS console.
	s.SetWriteSync(
		[]MsgWriter{
			s.NewMsgWriterToControllerStdout(),
		},
		[]MsgWriter{
			s.NewMsgWriterToControllerStderr(),
		},
	)

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

type MsgWriter interface {
	Write(p []byte) (n int, err error)
	sealed()
}

type msgWriterController struct {
	self   *GlobalSelfConn
	prefix string
}

func (msgWriterController) sealed() {}

func (w *msgWriterController) Write(p []byte) (int, error) {
	if err := w.self.PostMessage(safejs.Safe(js.ValueOf(w.prefix+string(p)+"\n")), nil); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *GlobalSelfConn) NewMsgWriterToControllerStdout() MsgWriter {
	return &msgWriterController{self: s, prefix: STDOUT_EVENT}
}

func (s *GlobalSelfConn) NewMsgWriterToControllerStderr() MsgWriter {
	return &msgWriterController{self: s, prefix: STDERR_EVENT}
}

type msgWriterIoWriter struct {
	w io.Writer
}

func (msgWriterIoWriter) sealed() {}

func (w *msgWriterIoWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func (s *GlobalSelfConn) NewMsgWriterToIoWriter(w io.Writer) MsgWriter {
	return &msgWriterIoWriter{w: w}
}

type msgWriterConsole struct{}

func (msgWriterConsole) sealed() {}

func (msgWriterConsole) Write(p []byte) (int, error) {
	js.Global().Get("console").Call("log", js.ValueOf(string(p)))
	return len(p), nil
}

func (s *GlobalSelfConn) NewMsgWriterToConsole() MsgWriter {
	return msgWriterConsole{}
}

// SetWriteSync overrides the "writeSync" implementation that will be called by Go.
// It redirects the message to a slice of `MsgWriterFunc` functions for both the stdout and stderr.
func (s *GlobalSelfConn) SetWriteSync(stdoutWriters, stderrWriters []MsgWriter) {
	writeSync := func() js.Func {
		jsConsole := js.Global().Get("console")
		var outputBuffer string
		return js.FuncOf(func(this js.Value, args []js.Value) any {
			fd, buf := args[0], args[1]
			outputBuffer += js.Global().Get("TextDecoder").New("utf-8").Call("decode", buf).String()
			nl := strings.LastIndex(outputBuffer, "\n")
			if nl != -1 {
				msg := outputBuffer[:nl]
				switch fd.Int() {
				case 1:
					for i, w := range stdoutWriters {
						if _, err := w.Write([]byte(msg)); err != nil {
							jsConsole.Call("log", js.ValueOf(fmt.Sprintf("%d-th writeSync for stdout error: %v", i, err)))
						}
					}
				case 2:
					for i, w := range stderrWriters {
						if _, err := w.Write([]byte(msg)); err != nil {
							jsConsole.Call("log", js.ValueOf(fmt.Sprintf("%d-th writeSync for stdout error: %v", i, err)))
						}
					}
				}
				outputBuffer = outputBuffer[nl+1:]
			}
			return buf.Get("length")
		})
	}()
	js.Global().Get("fs").Set("writeSync", writeSync)
}

func (s *GlobalSelfConn) ResetWriteSync() {
	js.Global().Get("fs").Set("writeSync", s.originWriteSync)
}
