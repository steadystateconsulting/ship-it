package main

import (
	"fmt"
	"os"

	"shipit/internal/shipit"
)

func main() {
	if err := shipit.Main(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
