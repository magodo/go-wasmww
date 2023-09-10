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
	conn1 := &wasmww.WasmSharedWebWorkerConn{
		Name: "hello",
		Path: "hello.wasm",
	}
	consoleConn, err := conn1.Start()
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
		handle(conn1.EventChannel())()
		wg.Done()
	}()
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hello Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hello Bar!")), nil); err != nil {
		log.Fatal(err)
	}

	// Create a 2nd connection to the shared worker
	conn2 := &wasmww.WasmSharedWebWorkerConn{
		Name: conn1.Name,
		URL:  conn1.URL,
	}
	if err := conn2.Connect(); err != nil {
		log.Fatal(err)
	}

	// Close and wait for the 1st port closed when the 2nd connection is active, to avoid the worker to close itself.
	// Since on the worker side, we will close it if there is no active connection.
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}
	conn1.Wait()

	wg.Add(1)
	go func() {
		handle(conn2.EventChannel())()
		wg.Done()
	}()

	if err := conn2.PostMessage(safejs.Safe(js.ValueOf("Hi Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn2.PostMessage(safejs.Safe(js.ValueOf("Hi Bar!")), nil); err != nil {
		log.Fatal(err)
	}

	// Connect conn1 back (to test a closed connection can connect again)
	if err := conn1.Connect(); err != nil {
		log.Fatal(err)
	}

	wg.Add(1)
	go func() {
		handle(conn1.EventChannel())()
		wg.Done()
	}()

	// Close and wait for the 2nd port closed, after the conn1 is connected again
	if err := conn2.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}
	conn2.Wait()

	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hey Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hey Bar!")), nil); err != nil {
		log.Fatal(err)
	}

	// Close and wait for the 1st port closed again, this will close the worker as it is the last connection
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}
	conn1.Wait()

	// The close of the worker will close the console connection
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
