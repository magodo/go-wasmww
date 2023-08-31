//go:build js && wasm

package wasmww

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/hack-pad/go-webworkers/worker"
	"github.com/hack-pad/safejs"
)

//go:embed worker.js.tpl
var WorkerJSTpl []byte

type WasmWebWorker struct {
	Name string
	Path string
	Args []string
	Env  map[string]string

	worker *worker.Worker
}

func (ww *WasmWebWorker) Start() error {
	var workerJS bytes.Buffer
	if err := template.Must(template.New("js").Funcs(template.FuncMap{
		"toArray": func(l []string) string {
			el := []string{}
			for _, e := range l {
				el = append(el, `"`+e+`"`)
			}
			return "[" + strings.Join(el, ",") + "]"
		},
		"toObject": func(m map[string]string) string {
			el := []string{}
			for k, v := range m {
				el = append(el, fmt.Sprintf(`"%s":"%s"`, k, v))
			}
			return "{" + strings.Join(el, ",") + "}"
		},
	}).Parse(string(WorkerJSTpl))).Execute(&workerJS, ww); err != nil {
		return err
	}

	wk, err := worker.NewFromScript(workerJS.String(), worker.Options{Name: ww.Name})
	if err != nil {
		return err
	}

	ww.worker = wk

	return nil
}

func (ww *WasmWebWorker) PostMessage(data safejs.Value, transfers []safejs.Value) error {
	return ww.worker.PostMessage(data, transfers)
}

func (ww *WasmWebWorker) Terminate() {
	ww.worker.Terminate()
}

func (ww *WasmWebWorker) Listen(ctx context.Context) (<-chan worker.MessageEvent, error) {
	return ww.worker.Listen(ctx)
}
