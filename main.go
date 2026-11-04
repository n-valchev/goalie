// Command godry finds Go functions with the same structure.
package main

import (
	"fmt"
	"os"

	"github.com/n-valchev/godry/internal/cli"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "godry: %v\n", err)
		os.Exit(2)
	}
	os.Exit(cli.Run(os.Args[1:], dir, os.Stdout, os.Stderr))
}
