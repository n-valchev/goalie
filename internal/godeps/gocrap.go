package godeps

import (
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/crap"
	"github.com/n-valchev/goalie/internal/model"
	"github.com/n-valchev/goalie/internal/toolreport"
)

// joinGocrap warns and does not fail on rows with no op, because a report can cover code that this
// run left out, and the rest of the map is still right.
func joinGocrap(dir, file string, limits crap.Limits, result *model.Model, stderr io.Writer) error {
	if file == "" {
		return nil
	}
	rows, err := toolreport.ReadGocrap(dir, file)
	if err != nil {
		return err
	}
	if unmatched := model.JoinCrap(result, rows, limits); unmatched > 0 {
		fmt.Fprintf(stderr, "godeps: gocrap rows with no function: %d\n", unmatched)
	}
	return nil
}
