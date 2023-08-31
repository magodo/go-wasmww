package main

import (
	"fmt"
	"log"
	"syscall/js"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
)

func main() {
	closeCh := make(chan any)
	self, err := wasmww.Self()
	if err != nil {
		log.Fatal(err)
	}
	name, err := self.Name()
	if err != nil {
		log.Fatal(err)
	}
	f, err := safejs.FuncOf(func(this safejs.Value, args []safejs.Value) any {
		v, err := args[0].Get("data")
		if err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			return nil
		}
		str, err := v.String()
		if err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			return nil
		}
		fmt.Printf("Worker (%s): Received %q\n", name, str)
		if str == "Close" {
			close(closeCh)
			return nil
		}
		if err := self.PostMessage(safejs.Safe(js.ValueOf(str)), nil); err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			return nil
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := safejs.Global().Set("HandleMessage", f); err != nil {
		log.Fatal(err)
	}

	<-closeCh

	self.PostMessage(safejs.Safe(js.ValueOf(wasmww.CLOSE_EVENT)), nil)
	fmt.Printf("Worker (%s): Close\n", name)
	self.Close()
}
