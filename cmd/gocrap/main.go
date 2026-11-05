// Command gocrap finds Go functions that are complex and poorly tested.
package main

import (
	"fmt"
	"os"

	"github.com/n-valchev/goalie/internal/gocrap"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gocrap: %v\n", err)
		os.Exit(2)
	}
	os.Exit(gocrap.Run(os.Args[1:], dir, os.Stdout, os.Stderr))
}
