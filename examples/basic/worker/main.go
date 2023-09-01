//go:build js && wasm

package main

import (
	"fmt"
	"log"
	"os"
	"syscall/js"

	"github.com/hack-pad/safejs"
	"github.com/magodo/go-wasmww"
)

func main() {
	self, err := wasmww.SelfConn()
	if err != nil {
		log.Fatal(err)
	}
	name, err := self.Name()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Worker (%s): Args: %v\n", name, os.Args)
	fmt.Printf("Worker (%s): Env: %v\n", name, os.Environ())

	ch, closeFn, err := self.SetupConn()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			log.Fatal(err)
		}
	}()

	for event := range ch {
		data, err := event.Data()
		if err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			continue
		}
		str, err := data.String()
		if err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			continue
		}
		fmt.Printf("Worker (%s): Received %q\n", name, str)
		if str == "Close" {
			// cancel the context will close the channel from within the Listen()
			closeFn()
			fmt.Printf("Worker (%s): Close\n", name)
			continue
		}

		// Echo back the message
		if err := self.PostMessage(safejs.Safe(js.ValueOf(str)), nil); err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			continue
		}
	}
	fmt.Printf("Worker (%s): Exit\n", name)
}
