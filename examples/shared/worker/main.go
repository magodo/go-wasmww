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

	logger := log.New(os.Stdout, fmt.Sprintf("Worker (%s): ", name), 0)

	logger.Printf("Args: %v\n", os.Args)
	logger.Printf("Env: %v\n", os.Environ())

	location, err := self.Location()
	if err != nil {
		log.Fatal(err)
	}
	logger.Printf("Location: %s\n", location)

	null := io.Discard
	i := 0
	for port := range connCh {
		port := port
		logger := log.New(os.Stdout, fmt.Sprintf("Worker (%s-%d): ", name, i), 0)
		i++
		go func() {
			logger.Printf("started\n")
			eventCh, err := port.SetupConn()
			if err != nil {
				log.Fatal(err)
			}
			for event := range eventCh {
				data, err := event.Data()
				if err != nil {
					logger.Printf("Error: %v\n", err)
					continue
				}
				str, err := data.String()
				if err != nil {
					logger.Printf("Error: %v\n", err)
					continue
				}

				switch str {
				case "ClosePort":
					logger.Printf("Close port\n")
					if err := port.Close(); err != nil {
						logger.Fatal(err)
					}
					if self.Idle() {
						logger.Printf("Close the worker for idle\n")
						if err := self.Close(); err != nil {
							log.Fatal(err)
						}
					}
					continue
				case "CloseWorker":
					logger.Printf("Close the worker\n")
					if err := self.Close(); err != nil {
						log.Fatal(err)
					}
				case "WriteToNull":
					wasmww.SetWriteSync(
						[]wasmww.MsgWriter{wasmww.NewMsgWriterToIoWriter(null)},
						[]wasmww.MsgWriter{wasmww.NewMsgWriterToIoWriter(null)},
					)
					continue
				}

				logger.Printf("Received message %q\n", str)

				// Echo back the message
				if err := port.PostMessage(safejs.Safe(js.ValueOf(str)), nil); err != nil {
					logger.Printf("Error: %v\n", err)
					continue
				}
			}
		}()
	}
	select {}
}
