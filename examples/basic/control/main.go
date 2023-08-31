package main

import (
	"fmt"
	"log"
	"syscall/js"
	"time"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
)

func main() {
	conn := &wasmww.WasmWebWorkerConn{
		Name: "hello",
		Path: "hello.wasm",
		Env: map[string]string{
			"foo": "bar",
		},
		Args: []string{"wasm", "arg"},
	}
	closeCh, err := spawnAndHandle(conn)
	if err != nil {
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
	<-closeCh
	fmt.Printf("Control: Worker closed\n")

	// Re-spwn
	closeCh, err = spawnAndHandle(conn)
	if err != nil {
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
	time.Sleep(time.Millisecond)
	conn.Terminate()
	<-closeCh
	fmt.Printf("Control: Worker terminated\n")

	time.Sleep(time.Millisecond)
}

func spawnAndHandle(conn *wasmww.WasmWebWorkerConn) (chan interface{}, error) {
	if err := conn.Start(); err != nil {
		return nil, err
	}
	ch := make(chan interface{})
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
		close(ch)
	}()
	return ch, nil
}
