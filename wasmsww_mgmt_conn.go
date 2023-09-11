package wasmww

import (
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"
	"syscall/js"

	"github.com/hack-pad/safejs"

	"github.com/magodo/chanio"
	"github.com/magodo/go-webworkers/types"
)

// WasmSharedWebWorkerMgmtConn is a connection to a newly started Shared Web Worker.
// It is only meant to:
// - Receive stdout/stderr from the worker, in form of the message event.
// - Send mgmt message events to the worker, including:
//   - Close event to let it close itself
//   - SetWriteToConsole event to let it write to console
//   - SetWriteToController event to let it write to this port back to the controller
type WasmSharedWebWorkerMgmtConn struct {
	name string
	path string
	args []string
	env  []string
	url  string

	stdout io.ReadCloser
	stderr io.ReadCloser

	ww        *WasmSharedWebWorker
	conns     []*WasmSharedWebWorkerConn
	closeFunc WebWorkerCloseFunc
	closeCh   chan any
}

func (c *WasmSharedWebWorkerMgmtConn) start() (err error) {
	ww := &WasmSharedWebWorker{
		Name: c.name,
		Path: c.path,
		Args: c.args,
		Env:  c.env,
	}
	if err := ww.startForConn(); err != nil {
		return err
	}
	if c.name == "" {
		c.name = ww.Name
	}
	c.url = ww.URL
	c.ww = ww

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
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

	c.stdout = stdoutR
	c.stderr = stderrR
	c.closeCh = closeCh

	mgmtCh, err := ww.Listen(ctx)
	if err != nil {
		return err
	}

	closeFunc := func() error {
		cancel()
		for range mgmtCh {
		}
		stdoutW.Close()
		stderrW.Close()
		return nil
	}
	c.closeFunc = closeFunc

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

// Connect creates a new WasmSharedWebWorkerConn to an active Shared Web Worker.
func (c *WasmSharedWebWorkerMgmtConn) Connect() (conn *WasmSharedWebWorkerConn, err error) {
	ww := &WasmSharedWebWorker{
		Name: c.name,
		URL:  c.url,
	}

	if err := ww.Connect(); err != nil {
		return nil, err
	}

	conn = &WasmSharedWebWorkerConn{
		Name:     c.name,
		Path:     c.path,
		Env:      c.env,
		Args:     c.args,
		url:      c.url,
		ww:       ww,
		mgmtPort: c,
	}

	c.conns = append(c.conns, conn)

	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		if err != nil {
			cancel()
		}
	}()

	rawCh, err := ww.Listen(ctx)
	if err != nil {
		return nil, err
	}

	// Wait for the sync message
	if _, ok := <-rawCh; !ok {
		return nil, fmt.Errorf("channel closed (due to ctx canceled)")
	}

	closeFunc := func() error {
		cancel()
		for range rawCh {
		}
		c.conns = slices.DeleteFunc(c.conns, func(c *WasmSharedWebWorkerConn) bool {
			return conn == c
		})
		return nil
	}
	conn.closeFunc = closeFunc

	// Create a channel to relay the event from the onmessage channel to the consuming channel,
	// except it will cancel the listening context and close the channel when the worker closes.
	eventCh := make(chan types.MessageEventMessage)
	closeCh := make(chan any)
	go func() {
		for event := range rawCh {
			if data, err := event.Data(); err == nil {
				if str, err := data.String(); err == nil {
					if str == CLOSE_EVENT {
						closeFunc()
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

	return conn, nil
}

// Terminate mimics the terminate method of the DedicatedWorkerGlobalScope, by sending a close message to the shared worker, which will close itself on receive.
// Meanwhile, it will close the event channels of all the WasmSharedWebWorkerConn connected to this shared worker.
func (c *WasmSharedWebWorkerMgmtConn) Terminate() error {
	if err := c.ww.PostMessage(safejs.Safe(js.ValueOf(CLOSE_EVENT)), nil); err != nil {
		return err
	}
	for _, conn := range c.conns {
		conn.closeFunc()
	}
	return c.closeFunc()
}

// SetWriteToConsole instructs the worker to write its stdout/stderr to console
func (c *WasmSharedWebWorkerMgmtConn) SetWriteToConsole() error {
	return c.ww.PostMessage(safejs.Safe(js.ValueOf(WRITE_TO_CONSOLE_EVENT)), nil)
}

// SetWriteToController instructs the worker to write its stdout/stderr to controller, which can be retrieved by Stdout(), Stderr().
func (c *WasmSharedWebWorkerMgmtConn) SetWriteToController() error {
	return c.ww.PostMessage(safejs.Safe(js.ValueOf(WRITE_TO_CONTROLLER_EVENT)), nil)

}

// Wait waits for the controller's internal event loop to quit. This can be caused by the worker closes itself.
func (c *WasmSharedWebWorkerMgmtConn) Wait() {
	<-c.closeCh
}

// Stdout returns an io.ReadCloser that streams out the stdout of the web worker as long as its target write destination is not modified to redirect to other sinks
func (c *WasmSharedWebWorkerMgmtConn) Stdout() io.ReadCloser {
	return c.stdout
}

// Stderr returns an io.ReadCloser that streams out the stderr of the web worker as long as its target write destination implementation is not modified to redirect to other sinks
func (c *WasmSharedWebWorkerMgmtConn) Stderr() io.ReadCloser {
	return c.stderr
}
