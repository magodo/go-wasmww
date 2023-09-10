package wasmww

import (
	"context"
	"fmt"
	"slices"
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

	// consolePort is a special port that only used for sending stdout/stderr to the peer, but won't receive any message from there.
	// It is guaranteed to be set on the first message port setup.
	consolePort *types.MessagePort

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

// SetupConn setup the worker for working with the peering WasmSharedWebWorkerConn.
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

	// The first connection is the port for the stdout/stderr
	event := <-connCh
	ports, err := event.Ports()
	if err != nil {
		return nil, err
	}
	if pl := len(ports); pl != 1 {
		return nil, fmt.Errorf("the initial connection expects to have one port, got=%d", pl)
	}
	consolePort := ports[0]
	s.consolePort = consolePort

	//Redirect the stdout/stderr to this port
	SetWriteSync(
		[]MsgWriter{
			s.NewMsgWriterToControllerStdout(),
		},
		[]MsgWriter{
			s.NewMsgWriterToControllerStderr(),
		},
	)

	// Notify the controller with a non-null msg to indicate the console port connection is setup
	if err := consolePort.PostMessage(safejs.Safe(js.ValueOf(true)), nil); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	s.closeFunc = func() error {
		cancel()
		wg.Wait()

		msg, err := safejs.ValueOf(CLOSE_EVENT)
		if err != nil {
			return err
		}
		for _, port := range s.ports {
			port.closeFunc()
		}
		if s.consolePort != nil {
			s.consolePort.PostMessage(msg, nil)
		}
		// Close this web worker
		return s.self.Close()
	}

	// Create a channel to relay the event from the onmessage channel to the consuming channel,
	// except it will close the scope itself when the parent sends a close event.
	ch := make(chan *SelfSharedConnPort)
	wg.Add(1)
	go func() {
		for event := range connCh {
			if ports, err := event.Ports(); err == nil {
				if len(ports) == 1 {
					port := ports[0]
					ch <- &SelfSharedConnPort{
						conn: s,
						port: port,
					}
				}
			}
		}
		close(ch)
		wg.Done()
	}()

	return ch, nil
}

func (s *SelfSharedConn) Name() (string, error) {
	return s.self.Name()
}

func (s *SelfSharedConn) ResetWriteSync() {
	js.Global().Get("fs").Set("writeSync", s.originWriteSync)
}

func (s *SelfSharedConn) NewMsgWriterToControllerStdout() MsgWriter {
	return &msgWriterController{poster: s.consolePort, prefix: STDOUT_EVENT}
}

func (s *SelfSharedConn) NewMsgWriterToControllerStderr() MsgWriter {
	return &msgWriterController{poster: s.consolePort, prefix: STDERR_EVENT}
}

// Close closes all the message event handlers and their channels, together with the connection event message handler and the channel.
// Then notify the contoller to close its console event handler and channel.
func (s *SelfSharedConn) Close() error {
	return s.closeFunc()
}

// Idle tells whether this Shared Web Worker has no connected port at this point
func (s *SelfSharedConn) Idle() bool {
	return len(s.ports) == 0
}

type SelfSharedConnPort struct {
	conn      *SelfSharedConn
	closeFunc WebWorkerCloseFunc
	port      *types.MessagePort
}

// SetupConn setup the worker port for working with the peering WasmSharedWebWorkerConn.
// The returned eventCh sends the MessageEvent connected with the peering WasmSharedWebWorkerConn, until the closeFn is called.
func (p *SelfSharedConnPort) SetupConn() (_ <-chan types.MessageEventMessage, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.port.Listen(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	// Add this port to the conn's ports array for track
	p.conn.ports = append(p.conn.ports, p)

	p.closeFunc = func() error {
		cancel()
		for range ch {
		}

		msg, err := safejs.ValueOf(CLOSE_EVENT)
		if err != nil {
			return err
		}
		if err := p.port.PostMessage(msg, nil); err != nil {
			return err
		}
		if err := p.port.Close(); err != nil {
			return err
		}
		// Remove this port from the conn's ports array
		p.conn.ports = slices.DeleteFunc(p.conn.ports, func(port *SelfSharedConnPort) bool {
			return port == p
		})
		return nil
	}

	// Notify the controller that this worker has started listening
	if err := p.port.PostMessage(safejs.Null(), nil); err != nil {
		p.closeFunc()
		return nil, err
	}

	return ch, nil
}

func (p *SelfSharedConnPort) PostMessage(message safejs.Value, transfers []safejs.Value) error {
	return p.port.PostMessage(message, transfers)
}

// Close closes the message event handler and the channel, then notify the controller to do the same.
func (p *SelfSharedConnPort) Close() error {
	return p.closeFunc()
}
