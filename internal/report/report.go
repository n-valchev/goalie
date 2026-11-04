// Package report writes findings.
package report

import (
	"fmt"
	"io"

	"github.com/n-valchev/godry/internal/compare"
)

// Text writes one line per pair.
func Text(w io.Writer, pairs []compare.Pair) {
	if len(pairs) == 0 {
		fmt.Fprintln(w, "No duplicates found.")
		return
	}
	for _, pair := range pairs {
		fmt.Fprintf(w, "%s:%d: duplicates %s:%d (score %.2f)\n",
			pair.Left.File, pair.Left.StartLine, pair.Right.File, pair.Right.StartLine, pair.Score)
	}
}
