package wasmww

import (
	"fmt"
	"io"
	"strings"
	"syscall/js"

	"github.com/hack-pad/safejs"
)

type MessagePoster interface {
	PostMessage(message safejs.Value, transfers []safejs.Value) error
}

type MsgWriter interface {
	Write(p []byte) (n int, err error)
	sealed()
}

type msgWriterController struct {
	poster MessagePoster
	prefix string
}

func (msgWriterController) sealed() {}

func (w *msgWriterController) Write(p []byte) (int, error) {
	if err := w.poster.PostMessage(safejs.Safe(js.ValueOf(w.prefix+string(p)+"\n")), nil); err != nil {
		return 0, err
	}
	return len(p), nil
}

type msgWriterIoWriter struct {
	w io.Writer
}

func (msgWriterIoWriter) sealed() {}

func (w *msgWriterIoWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func NewMsgWriterToIoWriter(w io.Writer) MsgWriter {
	return &msgWriterIoWriter{w: w}
}

type msgWriterConsole struct{}

func (msgWriterConsole) sealed() {}

func (msgWriterConsole) Write(p []byte) (int, error) {
	js.Global().Get("console").Call("log", js.ValueOf(string(p)))
	return len(p), nil
}

func NewMsgWriterToConsole() MsgWriter {
	return msgWriterConsole{}
}

// SetWriteSync overrides the "writeSync" implementation that will be called by Go.
// It redirects the message to a slice of `MsgWriterFunc` functions for both the stdout and stderr.
func SetWriteSync(stdoutWriters, stderrWriters []MsgWriter) {
	writeSync := func() js.Func {
		jsConsole := js.Global().Get("console")
		var outputBuffer string
		return js.FuncOf(func(this js.Value, args []js.Value) any {
			fd, buf := args[0], args[1]
			outputBuffer += js.Global().Get("TextDecoder").New("utf-8").Call("decode", buf).String()
			nl := strings.LastIndex(outputBuffer, "\n")
			if nl != -1 {
				msg := outputBuffer[:nl]
				switch fd.Int() {
				case 1:
					for i, w := range stdoutWriters {
						if _, err := w.Write([]byte(msg)); err != nil {
							jsConsole.Call("log", js.ValueOf(fmt.Sprintf("%d-th writeSync for stdout error: %v", i, err)))
						}
					}
				case 2:
					for i, w := range stderrWriters {
						if _, err := w.Write([]byte(msg)); err != nil {
							jsConsole.Call("log", js.ValueOf(fmt.Sprintf("%d-th writeSync for stdout error: %v", i, err)))
						}
					}
				}
				outputBuffer = outputBuffer[nl+1:]
			}
			return buf.Get("length")
		})
	}()
	js.Global().Get("fs").Set("writeSync", writeSync)
}
