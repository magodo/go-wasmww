//go:build js && wasm

package wasmww

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"

	"github.com/hack-pad/go-webworkers/worker"
	"github.com/hack-pad/safejs"
)

const CLOSE_EVENT = "__WASMWW_CLOSE__"

//go:embed worker.js.tpl
var WorkerJSTpl []byte

type WASMWW struct {
	Name   string
	Path   string
	Args   []string
	Env    map[string]string
	CbName string

	worker    *worker.Worker
	ctx       context.Context
	ctxCancel context.CancelFunc
	eventCh   <-chan worker.MessageEvent
}

func (ww *WASMWW) Spawn() error {
	var workerJS bytes.Buffer
	if err := template.Must(template.New("js").Parse(string(WorkerJSTpl))).Execute(&workerJS, struct{ CbName string }{CbName: ww.CbName}); err != nil {
		return err
	}

	wk, err := worker.NewFromScript(workerJS.String(), worker.Options{Name: ww.Name})
	if err != nil {
		return err
	}

	argv := make([]any, 0, len(ww.Args))
	for _, arg := range ww.Args {
		argv = append(argv, arg)
	}

	env := map[string]any{}
	for k, v := range ww.Env {
		env[k] = v
	}

	msg, err := safejs.ValueOf(map[string]any{
		"argv": argv,
		"env":  env,
		"path": ww.Path,
	})

	if err != nil {
		return err
	}

	// Sending the initial msg to the worker script to boot the WASM module
	if err := wk.PostMessage(msg, nil); err != nil {
		return err
	}

	ww.ctx, ww.ctxCancel = context.WithCancel(context.Background())

	rawCh, err := wk.Listen(ww.ctx)
	if err != nil {
		return err
	}

	// Create a channel to relay the event from the onmessage channel to the consuming channel,
	// except it will cancel the listening context and close the channel when the worker closes.
	eventCh := make(chan worker.MessageEvent)
	go func() {
		for {
			select {
			case <-ww.ctx.Done():
				close(eventCh)
				return
			case event := <-rawCh:
				if data, err := event.Data(); err == nil {
					if str, err := data.String(); err == nil {
						if str == CLOSE_EVENT {
							ww.ctxCancel()
							continue
						}
					}
				}
				eventCh <- event
			}
		}
	}()

	ww.eventCh = eventCh
	ww.worker = wk

	return nil
}

func (ww *WASMWW) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return ww.worker.PostMessage(data, transfers)
}

func (ww *WASMWW) Terminate() {
	ww.ctxCancel()
	ww.worker.Terminate()
}

func (ww *WASMWW) EventCh() <-chan worker.MessageEvent {
	return ww.eventCh
}
