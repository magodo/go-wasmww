package wasmww

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/magodo/chanio"
)

// WasmSharedWebWorkerConsoleConn is a connection to a newly started Shared Web Worker, only meant to
// receive stdout/stderr from the worker, in form of the message event.
// It will be closed when the Shared Web Worker closed.
type WasmSharedWebWorkerConsoleConn struct {
	Name string
	Path string
	Args []string
	Env  []string

	URL string

	stdout io.ReadCloser
	stderr io.ReadCloser

	ww        *WasmSharedWebWorker
	ctx       context.Context
	ctxCancel context.CancelFunc
	closeCh   chan any
}

func (conn *WasmSharedWebWorkerConsoleConn) start() (err error) {
	ww := &WasmSharedWebWorker{
		Name: conn.Name,
		Path: conn.Path,
		Args: conn.Args,
		Env:  conn.Env,
	}
	if err := ww.startForConn(); err != nil {
		return err
	}
	if conn.Name == "" {
		conn.Name = ww.Name
	}
	conn.URL = ww.URL
	conn.ww = ww

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			ctxCancel()
		}
	}()

	ch, err := ww.Listen(ctx)
	if err != nil {
		return err
	}

	// Wait for the worker's initial sync event, which indicates the worker is ready to receive connect events.
	if _, ok := <-ch; !ok {
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

	conn.stdout = stdoutR
	conn.stderr = stderrR
	conn.closeCh = closeCh

	consoleCh, err := ww.Listen(ctx)
	if err != nil {
		return err
	}

	// Wait for the worker's console msg ready event, which is non-null only to indicate the console message channel is ready.
	msgReadyMsg, ok := <-consoleCh
	if !ok {
		err = fmt.Errorf("message event channel closed (due to ctx canceled)")
		return
	}
	data, err := msgReadyMsg.Data()
	if err != nil {
		ctxCancel()
		return err
	}

	if data.IsNull() {
		return fmt.Errorf("the Shared Web Worker already exists")
	}

	// Consume the message that represents the stdout/stderr of the web worker.
	// It will cancel the listening context and close the channel when the worker closes.
	go func() {
		for event := range consoleCh {
			if data, err := event.Data(); err == nil {
				if str, err := data.String(); err == nil {
					if str == CLOSE_EVENT {
						ctxCancel()
						stdoutW.Close()
						stderrW.Close()
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

// Wait waits for the controller's internal event loop to quit. This can be caused by the worker closes itself or calling the Close().
func (conn *WasmSharedWebWorkerConsoleConn) Wait() {
	<-conn.closeCh
}

func (conn *WasmSharedWebWorkerConsoleConn) Stdout() io.ReadCloser {
	return conn.stdout
}

func (conn *WasmSharedWebWorkerConsoleConn) Stderr() io.ReadCloser {
	return conn.stderr
}
