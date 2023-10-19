//go:build js && wasm

package wasmww

import (
	"context"
	"fmt"
	"sync"
	"syscall/js"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/sharedworker"
	"github.com/magodo/go-webworkers/types"
)

type SelfSharedConn struct {
	self      *sharedworker.GlobalSelf
	closeFunc WebWorkerCloseFunc

	ports []*SelfSharedConnPort

	// mgmtPort is a special port that only used for sending stdout/stderr to the peer, and only receive the mgmt message from there.
	// It is guaranteed to be set on the first message port setup.
	mgmtPort *types.MessagePort

	// originWriteSync stores the original js.Func of the "writeSync" from the Go glue file.
	originWriteSync js.Value
}

func NewSelfSharedConn() (*SelfSharedConn, error) {
	self, err := sharedworker.Self()
	if err != nil {
		return nil, err
	}
	return &SelfSharedConn{
		self:            self,
		originWriteSync: js.Global().Get("fs").Get("writeSync"),
	}, nil
}

// SetupConn set up the worker for working with the peering WasmSharedWebWorkerConn.
// The returned eventCh sends the SelfSharedConnPort connected with the peering WasmSharedWebWorkerConn, until the closeFn is called.
func (s *SelfSharedConn) SetupConn() (_ <-chan *SelfSharedConnPort, err error) {
	recentPort, err := safejs.Global().Get("recent_port")
	if err != nil {
		return nil, err
	}
	initMsgPort, err := types.WrapMessagePort(recentPort)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	connCh, err := s.self.Listen(ctx)
	if err != nil {
		return nil, err
	}

	// Notify controller the initial sync message to indicate the worker is ready for connection request
	if err := initMsgPort.PostMessage(safejs.Null(), nil); err != nil {
		return nil, err
	}

	// The first connection is the mgmt port
	event := <-connCh
	ports, err := event.Ports()
	if err != nil {
		return nil, err
	}
	if pl := len(ports); pl != 1 {
		return nil, fmt.Errorf("the initial connection expects to have one port, got=%d", pl)
	}
	mgmtPort := ports[0]
	s.mgmtPort = mgmtPort

	//Redirect the stdout/stderr to this port
	SetWriteSync(
		[]MsgWriter{
			s.NewMsgWriterToControllerStdout(),
		},
		[]MsgWriter{
			s.NewMsgWriterToControllerStderr(),
		},
	)

	// Listening on mgmt message from this port, currently, only close event will be sent through it.
	mgmtCh, err := mgmtPort.Listen(ctx)
	if err != nil {
		return nil, err
	}

	var wg1, wg2 sync.WaitGroup
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		for event := range mgmtCh {
			data, err := event.Data()
			if err != nil {
				continue
			}
			str, err := data.String()
			if err != nil {
				continue
			}
			switch str {
			case CLOSE_EVENT:
				ports := make([]*SelfSharedConnPort, len(s.ports))
				copy(ports, s.ports)
				for _, port := range ports {
					port.closeFunc()
				}

				cancel()
				wg2.Wait()

				// Close this web worker
				s.self.Close()

			case WRITE_TO_CONSOLE_EVENT:
				s.ResetWriteSync()

			case WRITE_TO_CONTROLLER_EVENT:
				SetWriteSync(
					[]MsgWriter{s.NewMsgWriterToControllerStdout()},
					[]MsgWriter{s.NewMsgWriterToControllerStderr()},
				)
			}
		}
	}()

	// Notify the controller with a non-null msg to indicate the mgmt port connection is set up
	if err := mgmtPort.PostMessage(safejs.Safe(js.ValueOf(true)), nil); err != nil {
		return nil, err
	}

	// Create a channel to relay the event from the onmessage channel to the consuming channel,
	// except it will close the scope itself when the parent sends a close event.
	ch := make(chan *SelfSharedConnPort)
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		for event := range connCh {
			if ports, err := event.Ports(); err == nil {
				if len(ports) == 1 {
					port := ports[0]
					select {
					case ch <- &SelfSharedConnPort{
						conn: s,
						port: port,
					}:
					case <-ctx.Done():
						break
					}
				}
			}
		}
		close(ch)
	}()

	s.closeFunc = func() error {
		ports := make([]*SelfSharedConnPort, len(s.ports))
		copy(ports, s.ports)
		for _, port := range ports {
			port.closeFunc()
		}

		if s.mgmtPort != nil {
			s.mgmtPort.PostMessage(safejs.Safe(js.ValueOf(CLOSE_EVENT)), nil)
		}

		// This must comes after calling the closeFunc of ports, otherwise, those CLOSE events
		// will fail to be sent to outside (not sure why).
		cancel()
		wg1.Wait()
		wg2.Wait()

		// Close this web worker
		return s.self.Close()
	}

	return ch, nil
}

func (s *SelfSharedConn) Name() (string, error) {
	return s.self.Name()
}

func (s *SelfSharedConn) Location() (*types.WorkerLocation, error) {
	return s.self.Location()
}

func (s *SelfSharedConn) ResetWriteSync() {
	js.Global().Get("fs").Set("writeSync", s.originWriteSync)
}

func (s *SelfSharedConn) NewMsgWriterToControllerStdout() MsgWriter {
	return &msgWriterController{poster: s.mgmtPort, prefix: STDOUT_EVENT}
}

func (s *SelfSharedConn) NewMsgWriterToControllerStderr() MsgWriter {
	return &msgWriterController{poster: s.mgmtPort, prefix: STDERR_EVENT}
}

// Close closes the web worker, and close the event channels on all the controllers side.
func (s *SelfSharedConn) Close() error {
	return s.closeFunc()
}

// Idle tells whether this Shared Web Worker has no connected port at this point
func (s *SelfSharedConn) Idle() bool {
	return len(s.ports) == 0
}
