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

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
	"github.com/magodo/go-webworkers/types"
)

func main() {
	os.Setenv("parent_foo", "parent_bar")
	conn := &wasmww.WasmSharedWebWorkerConn{
		Name: "hello",
		Path: "hello.wasm",
	}
	consoleConn, err := conn.Start()
	if err != nil {
		log.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		io.Copy(&stdout, consoleConn.Stdout())
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		io.Copy(&stderr, consoleConn.Stderr())
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		handle(conn.EventChannel())()
		wg.Done()
	}()
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hello Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hello Bar!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("Hello Baz!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}

	// Wait for the 1st port closed
	conn.Wait()

	// Wait for the worker closed, which closes the console connection
	consoleConn.Wait()

	// Wait until the console output streams are all copied and event handler done
	wg.Wait()

	fmt.Printf(`Worker Stdout (from buffer)
---
%s
---
`, stdout.String())
	fmt.Printf(`Worker Stderr (from buffer)
---
%s
---
`, stderr.String())
}

func handle(ch <-chan types.MessageEventMessage) func() {
	return func() {
		for evt := range ch {
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
	}
}
