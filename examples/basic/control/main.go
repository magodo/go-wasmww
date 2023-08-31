package main

import (
	"fmt"
	"log"
	"syscall/js"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
)

func main() {
	ww := &wasmww.WASMWW{
		Name: "hello",
		Path: "hello.wasm",
		Env: map[string]string{
			"foo": "bar",
		},
		Args:   []string{"wasm", "arg"},
		CbName: "HandleMessage",
	}
	closeCh, err := spawnAndHandle(ww)
	if err != nil {
		log.Fatal(err)
	}
	if err := ww.PostMessage(safejs.Safe(js.ValueOf("Hello World!")), nil); err != nil {
		log.Fatal(err)
	}
	if err := ww.PostMessage(safejs.Safe(js.ValueOf("Close")), nil); err != nil {
		log.Fatal(err)
	}
	<-closeCh
	fmt.Printf("Control: Worker closed\n")

	// Re-spwn
	closeCh, err = spawnAndHandle(ww)
	if err != nil {
		log.Fatal(err)
	}
	if err := ww.PostMessage(safejs.Safe(js.ValueOf("Hello World!")), nil); err != nil {
		log.Fatal(err)
	}
	ww.Terminate()
	<-closeCh
	fmt.Printf("Control: Worker terminated\n")
}

func spawnAndHandle(ww *wasmww.WASMWW) (chan interface{}, error) {
	if err := ww.Spawn(); err != nil {
		return nil, err
	}
	ch := make(chan interface{})
	go func() {
		for evt := range ww.EventCh() {
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
