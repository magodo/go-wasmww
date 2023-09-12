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
	"github.com/magodo/go-webworkers/types"
)

func main() {
	os.Setenv("parent_foo", "parent_bar")
	conn1 := &wasmww.WasmSharedWebWorkerConn{
		Name: "hello",
		Path: "hello.wasm",
	}
	mgmtConn, err := conn1.Start()
	if err != nil {
		log.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		io.Copy(&stdout, mgmtConn.Stdout())
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		io.Copy(&stderr, mgmtConn.Stderr())
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
	conn2, err := mgmtConn.Connect()
	if err != nil {
		log.Fatal(err)
	}
	wg.Add(1)
	go func() {
		handle(conn2.EventChannel())()
		wg.Done()
	}()

	// Close and wait for the 1st port closed when the 2nd connection is active, to avoid the worker to close itself.
	// Since on the worker side, we will close it if there is no active connection.
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}
	conn1.Wait()

	if err := conn2.PostMessage(safejs.Safe(js.ValueOf("Hi Foo!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn2.PostMessage(safejs.Safe(js.ValueOf("Hi Bar!")), nil); err != nil {
		log.Fatal(err)
	}

	// Connect conn1 back (to test a closed connection can connect again)
	conn1, err = mgmtConn.Connect()
	if err != nil {
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

	if err := mgmtConn.SetWriteToConsole(); err != nil {
		log.Fatal(err)
	}
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hey Console!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("WriteToNull")), nil); err != nil {
		log.Fatal(err)
	}
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hey Secret!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := mgmtConn.SetWriteToController(); err != nil {
		log.Fatal(err)
	}
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("Hey Controller!")), nil); err != nil {
		log.Fatal(err)
	}

	// Close and wait for the 1st port closed again, this will close the worker as it is the last connection
	if err := conn1.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}
	conn1.Wait()

	// The close of the worker will close the console connection
	mgmtConn.Wait()

	// Wait until the console output streams are all copied and event handler done
	wg.Wait()
	printWorker(stdout, stderr)

	// Re-spawn
	conn1.Env = []string{
		"foo=bar",
	}
	conn1.Args = []string{"wasm", "arg"}
	mgmtConn, err = conn1.Start()
	if err != nil {
		log.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		io.Copy(&stdout, mgmtConn.Stdout())
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		io.Copy(&stderr, mgmtConn.Stderr())
		wg.Done()
	}()

	conn2, err = mgmtConn.Connect()
	if err != nil {
		log.Fatal(err)
	}

	conn3, err := mgmtConn.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Close the conn3 from within the worker
	if err := conn3.PostMessage(safejs.Safe(js.ValueOf("ClosePort")), nil); err != nil {
		log.Fatal(err)
	}
	conn3.Wait()

	// CLose the conn2 from the outside
	if err := conn2.Close(); err != nil {
		log.Fatal(err)
	}
	conn2.Wait()

	// Give some time to the worker to finish printing...
	time.Sleep(time.Millisecond * 100)

	// Terminate the worker
	if err := mgmtConn.Close(); err != nil {
		log.Fatal(err)
	}

	conn1.Wait()
	mgmtConn.Wait()
	wg.Wait()

	printWorker(stdout, stderr)

	mgmtConn, err = conn1.Start()
	if err != nil {
		log.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	wg = sync.WaitGroup{}
	wg.Add(1)
	go func() {
		io.Copy(&stdout, mgmtConn.Stdout())
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		io.Copy(&stderr, mgmtConn.Stderr())
		wg.Done()
	}()

	// Connect via mgmt connection
	conn2, err = mgmtConn.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Connect via newly constructing a conn
	conn3 = &wasmww.WasmSharedWebWorkerConn{
		Name: conn1.Name,
		URL:  conn1.URL,
	}
	if err := conn3.Connect(); err != nil {
		log.Fatal(err)
	}

	// Close the worker from a random connection with other connections active
	if err := conn2.PostMessage(safejs.Safe(js.ValueOf("CloseWorker")), nil); err != nil {
		log.Fatal(err)
	}

	conn1.Wait()
	conn2.Wait()
	conn3.Wait()
	mgmtConn.Wait()
	wg.Wait()

	printWorker(stdout, stderr)
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

func printWorker(stdout, stderr bytes.Buffer) {
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
}
