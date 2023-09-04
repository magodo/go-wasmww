//go:build js && wasm

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"syscall/js"
	"time"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
)

func main() {
	os.Setenv("parent_foo", "parent_bar")
	var stdout, stderr bytes.Buffer
	conn := &wasmww.WasmWebWorkerConn{
		Name:   "hello",
		Path:   "hello.wasm",
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
	conn.Env = []string{
		"foo=bar",
	}
	conn.Args = []string{"wasm", "arg"}
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
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("WriteToConsole")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hey Console!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("WriteToController")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hey Controller!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("WriteToNull")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hey Secret!")), nil); err != nil {
		log.Fatal(err)
	}
	// Reset the writer before close
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("WriteToController")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Close")), nil); err != nil {
		log.Fatal(err)
	}
	conn.Wait()
	fmt.Printf("Control: Worker closed\n")

	fmt.Printf(`Worker Stdout (from ctrl buffer)
---
%s
---
`, stdout.String())
	fmt.Printf(`Worker Stderr (from ctrl buffer)
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
	fmt.Printf(`Worker Stdout (from ctrl pipe)
---
%s
---
`, stdout.String())
	fmt.Printf(`Worker Stderr (from ctrl pipe)
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
