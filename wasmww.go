//go:build js && wasm

package wasmww

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/hack-pad/safejs"
	"github.com/magodo/go-webworkers/types"
	"github.com/magodo/go-webworkers/worker"
)

//go:embed worker.js.tpl
var WorkerJSTpl []byte

type WasmWebWorker struct {
	// Name specifies an identifying name for the DedicatedWorkerGlobalScope representing the scope of the worker, which is mainly useful for debugging purposes.
	// If this is not specified, `Start` will create a UUIDv4 for it and populate back.
	Name string

	// Path is the path of the WASM to run as the Web Worker.
	Path string

	// Args holds command line arguments, including the WASM as Args[0].
	// If the Args field is empty or nil, Run uses {Path}.
	Args []string

	// Env specifies the environment of the process.
	// Each entry is of the form "key=value".
	// If Env is nil, the new Web Worker uses the current context's
	// environment.
	// If Env contains duplicate environment keys, only the last
	// value in the slice for each duplicate key is used.
	Env []string

	worker *worker.Worker
}

type templateData struct {
	Path string
	Args []string
	Env  []string
}

func (d templateData) ArgsToJS() string {
	el := []string{}
	for _, e := range d.Args {
		el = append(el, `"`+e+`"`)
	}
	return "[" + strings.Join(el, ",") + "]"
}

func (d templateData) EnvToJS() string {
	el := []string{}
	for _, entry := range d.Env {
		if k, v, ok := strings.Cut(entry, "="); ok {
			el = append(el, fmt.Sprintf(`"%s":"%s"`, k, v))
		}
	}
	return "{" + strings.Join(el, ",") + "}"
}

func (ww *WasmWebWorker) Start() error {
	var workerJS bytes.Buffer

	args := ww.Args
	if len(args) == 0 {
		args = []string{ww.Path}
	}

	env := ww.Env
	if len(env) == 0 {
		env = os.Environ()
	}

	data := templateData{
		Path: ww.Path,
		Args: args,
		Env:  env,
	}
	if err := template.Must(template.New("js").Parse(string(WorkerJSTpl))).Execute(&workerJS, data); err != nil {
		return err
	}

	if ww.Name == "" {
		ww.Name = uuid.New().String()
	}

	wk, err := worker.NewFromScript(workerJS.String(), worker.Options{Name: ww.Name})
	if err != nil {
		return err
	}

	ww.worker = wk

	return nil
}

// PostMessage sends data in a message to the worker, optionally transferring ownership of all items in transfers.
func (ww *WasmWebWorker) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return ww.worker.PostMessage(data, transfers)
}

// Terminate immediately terminates the Worker.
func (ww *WasmWebWorker) Terminate() {
	ww.worker.Terminate()
}

// Listen sends message events on a channel for events fired by self.postMessage() calls inside the Worker's global scope.
// Stops the listener and closes the channel when ctx is canceled.
func (ww *WasmWebWorker) Listen(ctx context.Context) (<-chan types.MessageEvent, error) {
	return ww.worker.Listen(ctx)
}
