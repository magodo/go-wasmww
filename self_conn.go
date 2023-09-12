//go:build js && wasm

package wasmww

import (
	"context"
	"syscall/js"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/types"
	"github.com/magodo/go-webworkers/worker"
)

type WebWorkerCloseFunc func() error

type SelfConn struct {
	self      *worker.GlobalSelf
	closeFunc WebWorkerCloseFunc

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

func NewSelfConn() (*SelfConn, error) {
	self, err := worker.Self()
	if err != nil {
		return nil, err
	}
	return &SelfConn{
		self:            self,
		originWriteSync: js.Global().Get("fs").Get("writeSync"),
	}, nil
}

// SetupConn setup the worker for working with the peering WasmWebWorkerConn.
// The returned eventCh receives the event sent from the peering WasmWebWorkerConn, until the closeFn is called.
// The closeFn is used to instruct the peering to stop listening to this web worker, and close this web worker.
func (s *SelfConn) SetupConn() (_ <-chan types.MessageEventMessage, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	ch, err := s.self.Listen(ctx)
	if err != nil {
		return nil, err
	}

	s.closeFunc = func() error {
		cancel()
		for range ch {
		}
		if err := s.self.PostMessage(safejs.Safe(js.ValueOf(CLOSE_EVENT)), nil); err != nil {
			return err
		}
		return s.self.Close()
	}

	//Redirect stdout/stderr to the controller, instead of printing to the JS console.
	SetWriteSync(
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
		return nil, err
	}

	return ch, nil
}

func (s *SelfConn) Name() (string, error) {
	return s.self.Name()
}

func (s *SelfConn) PostMessage(message safejs.Value, transfers []safejs.Value) error {
	return s.self.PostMessage(message, transfers)
}

func (s *SelfConn) ResetWriteSync() {
	js.Global().Get("fs").Set("writeSync", s.originWriteSync)
}

func (s *SelfConn) NewMsgWriterToControllerStdout() MsgWriter {
	return &msgWriterController{poster: s.self, prefix: STDOUT_EVENT}
}

func (s *SelfConn) NewMsgWriterToControllerStderr() MsgWriter {
	return &msgWriterController{poster: s.self, prefix: STDERR_EVENT}
}

// Close closes the web worker, and close the event channel on the controller side.
func (s *SelfConn) Close() error {
	return s.closeFunc()
}
