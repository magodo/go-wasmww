//go:build js && wasm

package wasmww

import (
	"github.com/hack-pad/go-webworkers/worker"
	"github.com/hack-pad/safejs"
)

type GlobalSelf struct {
	gs *worker.GlobalSelf
}

func Self() (*GlobalSelf, error) {
	gs, err := worker.Self()
	if err != nil {
		return nil, err
	}
	return &GlobalSelf{gs: gs}, nil
}

func (s *GlobalSelf) PostMessage(message safejs.Value, transfers []safejs.Value) error {
	return s.gs.PostMessage(message, transfers)
}

func (s *GlobalSelf) Close() error {
	if err := s.gs.Close(); err != nil {
		return err
	}
	msg, err := safejs.ValueOf(CLOSE_EVENT)
	if err != nil {
		return err
	}
	return s.PostMessage(msg, nil)
}

func (s *GlobalSelf) Name() (string, error) {
	return s.gs.Name()
}
