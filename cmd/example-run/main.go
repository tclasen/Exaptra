package main

import (
	"fmt"
	"os"

	"github.com/tclasen/Exaptra/cmd/example-run/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
