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
	self, err := wasmww.NewSelfSharedConn()
	if err != nil {
		log.Fatal(err)
	}
	name, err := self.Name()
	if err != nil {
		log.Fatal(err)
	}

	connCh, err := self.SetupConn()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Worker (%s): Args: %v\n", name, os.Args)
	log.Printf("Worker (%s): Env: %v\n", name, os.Environ())

	null := io.Discard
	for port := range connCh {
		port := port
		go func() {
			eventCh, err := port.SetupConn()
			if err != nil {
				log.Fatal(err)
			}
			for event := range eventCh {
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
				case "ClosePort":
					log.Printf("Worker (%s): Close port\n", name)
					if err := port.Close(); err != nil {
						log.Fatal(err)
					}
					if self.Idle() {
						log.Printf("Worker (%s): Close the worker for idle\n", name)
						if err := self.Close(); err != nil {
							log.Fatal(err)
						}
					}
					continue
				case "WriteToConsole":
					self.ResetWriteSync()
					continue
				case "WriteToNull":
					wasmww.SetWriteSync(
						[]wasmww.MsgWriter{wasmww.NewMsgWriterToIoWriter(null)},
						[]wasmww.MsgWriter{wasmww.NewMsgWriterToIoWriter(null)},
					)
					continue
				case "WriteToController":
					wasmww.SetWriteSync(
						[]wasmww.MsgWriter{self.NewMsgWriterToControllerStdout()},
						[]wasmww.MsgWriter{self.NewMsgWriterToControllerStderr()},
					)
					continue
				}

				fmt.Printf("Worker (%s): Received message %q\n", name, str)

				// Echo back the message
				if err := port.PostMessage(safejs.Safe(js.ValueOf(str)), nil); err != nil {
					fmt.Printf("Worker (%s): Error: %v\n", name, err)
					continue
				}
			}
		}()
	}
	log.Printf("Worker (%s): Exit\n", name)
}
