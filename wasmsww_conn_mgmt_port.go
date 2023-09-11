package wasmww

import (
	"context"
	"fmt"
	"github.com/hack-pad/safejs"
	"io"
	"log"
	"strings"
	"syscall/js"

	"github.com/magodo/chanio"
)

// WasmSharedWebWorkerConnMgmtPort is a connection to a newly started Shared Web Worker.
// It is only meant to:
// - Receive stdout/stderr from the worker, in form of the message event.
// - Send mgmt message events to the worker, including:
//   - Close event to let it close itself
//   - SetWriteToConsole event to let it write to console
//   - SetWriteToController event to let it write to this port back to the controller
type WasmSharedWebWorkerConnMgmtPort struct {
	name string
	path string
	args []string
	env  []string
	url  string

	stdout io.ReadCloser
	stderr io.ReadCloser

	ww        *WasmSharedWebWorker
	closeFunc WebWorkerCloseFunc
	closeCh   chan any
}

func (port *WasmSharedWebWorkerConnMgmtPort) start() (err error) {
	ww := &WasmSharedWebWorker{
		Name: port.name,
		Path: port.path,
		Args: port.args,
		Env:  port.env,
	}
	if err := ww.startForConn(); err != nil {
		return err
	}
	if port.name == "" {
		port.name = ww.Name
	}
	port.url = ww.URL
	port.ww = ww

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			ctxCancel()
		}
	}()

	initCh, err := ww.Listen(ctx)
	if err != nil {
		return err
	}

	// Wait for the worker's initial sync event, which indicates the worker is ready to receive connect events.
	if _, ok := <-initCh; !ok {
		return fmt.Errorf("message event channel closed (due to ctx canceled)")
	}

	// Connect again when the worker is ready for connection
	if err := ww.Connect(); err != nil {
		return err
	}

	stdoutR, stdoutW, err := chanio.Pipe()
	if err != nil {
		return err
	}
	stderrR, stderrW, err := chanio.Pipe()
	if err != nil {
		return err
	}

	closeCh := make(chan any)

	port.stdout = stdoutR
	port.stderr = stderrR
	port.closeCh = closeCh

	mgmtCh, err := ww.Listen(ctx)
	if err != nil {
		return err
	}

	closeFunc := func() error {
		ctxCancel()
		stdoutW.Close()
		stderrW.Close()
		return nil
	}
	port.closeFunc = closeFunc

	// Wait for the worker's console msg ready event, which is non-null only to indicate the console message channel is ready.
	readyMsg, ok := <-mgmtCh
	if !ok {
		err = fmt.Errorf("message event channel closed (due to ctx canceled)")
		return
	}
	data, err := readyMsg.Data()
	if err != nil {
		return err
	}

	if data.IsNull() {
		return fmt.Errorf("the Shared Web Worker already exists")
	}

	// Consume the message that represents the stdout/stderr of the web worker.
	// It will cancel the listening context and close the channel when the worker closes.
	go func() {
		for event := range mgmtCh {
			if data, err := event.Data(); err == nil {
				if str, err := data.String(); err == nil {
					if str == CLOSE_EVENT {
						closeFunc()
						continue
					}
					if strings.HasPrefix(str, STDOUT_EVENT) {
						if _, err := stdoutW.Write([]byte(str[len(STDOUT_EVENT):])); err != nil {
							log.Fatalf("Controller writing to stdout: %v", err)
						}
						continue
					}
					if strings.HasPrefix(str, STDERR_EVENT) {
						if _, err := stderrW.Write([]byte(str[len(STDERR_EVENT):])); err != nil {
							log.Fatalf("Controller writing to stderr: %v", err)
						}
						continue
					}
					log.Fatalf("Only expected {STDOUT|STDERR|CLOSE}_EVENT, got=%q", str)
				}
			}
		}
		close(closeCh)
	}()
	return nil
}

// Terminate mimics the terminate method of the DedicatedWorkerGlobalScope, by sending a close message to the shared worker, which will close itself on receive.
func (port *WasmSharedWebWorkerConnMgmtPort) Terminate() error {
	if err := port.ww.PostMessage(safejs.Safe(js.ValueOf(CLOSE_EVENT)), nil); err != nil {
		return err
	}
	return port.closeFunc()
}

// SetWriteToConsole instructs the worker to write its stdout/stderr to console
func (port *WasmSharedWebWorkerConnMgmtPort) SetWriteToConsole() error {
	return port.ww.PostMessage(safejs.Safe(js.ValueOf(WRITE_TO_CONSOLE_EVENT)), nil)
}

// SetWriteToController instructs the worker to write its stdout/stderr to controller, which can be retrieved by Stdout(), Stderr().
func (port *WasmSharedWebWorkerConnMgmtPort) SetWriteToController() error {
	return port.ww.PostMessage(safejs.Safe(js.ValueOf(WRITE_TO_CONTROLLER_EVENT)), nil)

}

// Wait waits for the controller's internal event loop to quit. This can be caused by the worker closes itself.
func (port *WasmSharedWebWorkerConnMgmtPort) Wait() {
	<-port.closeCh
}

// Stdout returns an io.ReadCloser that streams out the stdout of the web worker as long as its target write destination is not modified to redirect to other sinks
func (port *WasmSharedWebWorkerConnMgmtPort) Stdout() io.ReadCloser {
	return port.stdout
}

// Stderr returns an io.ReadCloser that streams out the stderr of the web worker as long as its target write destination implementation is not modified to redirect to other sinks
func (port *WasmSharedWebWorkerConnMgmtPort) Stderr() io.ReadCloser {
	return port.stderr
}
