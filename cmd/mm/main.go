package main

import (
	"fmt"
	"os"

	"github.com/adamavenir/mini-msg/internal/command"
)

func main() {
	if err := command.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
