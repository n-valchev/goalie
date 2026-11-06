// Command goalie checks Go code against one set of rules.
package main

import (
	"fmt"
	"os"

	"github.com/n-valchev/goalie/internal/goalie"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "goalie: %v\n", err)
		os.Exit(2)
	}
	os.Exit(goalie.Run(os.Args[1:], dir, os.Stdin, os.Stdout, os.Stderr))
}
