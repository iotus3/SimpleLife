// Command simplelife starts the composition root and exits with its error.
package main

import (
	"log"
	"os"

	"github.com/iotus3/simplelife/app/internal/bootstrap"
)

func main() {
	if err := bootstrap.Run(); err != nil {
		log.Println("simplelife:", err)
		os.Exit(1)
	}
}
