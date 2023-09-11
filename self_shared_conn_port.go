package wasmww

import (
	"context"
	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/types"
	"slices"
)

type SelfSharedConnPort struct {
	conn      *SelfSharedConn
	closeFunc WebWorkerCloseFunc
	port      *types.MessagePort
}

// SetupConn set up the worker port for working with the peering WasmSharedWebWorkerConn.
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

// Close closes this port, and close the event channel on the controller side.
func (p *SelfSharedConnPort) Close() error {
	return p.closeFunc()
}
