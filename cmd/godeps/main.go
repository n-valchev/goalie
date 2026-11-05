// Command godeps maps the packages of a Go module and the imports between them.
package main

import (
	"fmt"
	"os"

	"github.com/n-valchev/goalie/internal/godeps"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "godeps: %v\n", err)
		os.Exit(2)
	}
	os.Exit(godeps.Run(os.Args[1:], dir, os.Stdout, os.Stderr))
}
