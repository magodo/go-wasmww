//go:build js && wasm

package main

import (
	"fmt"
	"io"
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

	ch, closeFn, err := self.SetupConn()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Worker (%s): Args: %v\n", name, os.Args)
	log.Printf("Worker (%s): Env: %v\n", name, os.Environ())

	defer func() {
		if err := closeFn(); err != nil {
			log.Fatal(err)
		}
	}()

	null := io.Discard
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

		switch str {
		case "Close":
			// cancel the context will close the channel from within the Listen()
			closeFn()
			fmt.Printf("Worker (%s): Close\n", name)
			continue
		case "WriteToConsole":
			self.ResetWriteSync()
			continue
		case "WriteToNull":
			self.SetWriteSync(
				[]wasmww.MsgWriter{self.NewMsgWriterToIoWriter(null)},
				[]wasmww.MsgWriter{self.NewMsgWriterToIoWriter(null)},
			)
			continue
		case "WriteToController":
			self.SetWriteSync(
				[]wasmww.MsgWriter{self.NewMsgWriterToControllerStdout()},
				[]wasmww.MsgWriter{self.NewMsgWriterToControllerStderr()},
			)
			continue
		}

		fmt.Printf("Worker (%s): Received message %q\n", name, str)

		// Echo back the message
		if err := self.PostMessage(safejs.Safe(js.ValueOf(str)), nil); err != nil {
			fmt.Printf("Worker (%s): Error: %v\n", name, err)
			continue
		}
	}

	fmt.Printf("Worker (%s): Exit\n", name)
}
