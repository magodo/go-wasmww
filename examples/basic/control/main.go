//go:build js && wasm

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"sync"
	"syscall/js"
	"time"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
)

func main() {
	var stdout, stderr bytes.Buffer
	conn := &wasmww.WasmWebWorkerConn{
		Name: "hello",
		Path: "hello.wasm",
		Env: map[string]string{
			"foo": "bar",
		},
		Args:   []string{"wasm", "arg"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if err := startHandle(conn); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hello Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hello Bar!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hello Baz!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Close")), nil); err != nil {
		log.Fatal(err)
	}
	conn.Wait()
	fmt.Printf("Control: Worker closed\n")

	// Re-spwn
	if err := startHandle(conn); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hi Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hi Bar!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hi Baz!")), nil); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Millisecond) // explicit switch point
	conn.Terminate()
	conn.Wait()
	fmt.Printf("Control: Worker terminated\n")

	// re-spawn again
	if err := startHandle(conn); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hey Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hey Bar!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hey Baz!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Close")), nil); err != nil {
		log.Fatal(err)
	}
	conn.Wait()
	fmt.Printf("Control: Worker closed\n")

	fmt.Printf(`Worker Stdout
---
%s
---
`, stdout.String())
	fmt.Printf(`Worker Stderr
---
%s
---
`, stderr.String())

	//re-spawn and use Stdout/errPipe()
	conn.Stdout = nil
	conn.Stderr = nil
	pipeOut, err := conn.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	pipeErr, err := conn.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(&stdout, pipeOut)
		wg.Done()
	}()
	go func() {
		io.Copy(&stderr, pipeErr)
		wg.Done()
	}()

	if err := startHandle(conn); err != nil {
		log.Fatal(err)
	}

	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Gutentag Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Close")), nil); err != nil {
		log.Fatal(err)
	}

	conn.Wait()
	fmt.Printf("Control: Worker closed\n")

	wg.Wait()
	fmt.Printf(`Worker Stdout (pipe)
---
%s
---
`, stdout.String())
	fmt.Printf(`Worker Stderr (pipe)
---
%s
---
`, stderr.String())
}

func startHandle(conn *wasmww.WasmWebWorkerConn) error {
	if err := conn.Start(); err != nil {
		return err
	}
	go func() {
		for evt := range conn.EventChannel() {
			data, err := evt.Data()
			if err != nil {
				log.Fatal(err)
			}
			str, err := data.String()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("Control: Received %q\n", str)
		}
		fmt.Printf("Control: Quit event handler\n")
	}()
	return nil
}
