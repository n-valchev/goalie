package godeps

import (
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/model"
	"github.com/n-valchev/goalie/internal/toolreport"
)

// joinDry warns and does not fail on a side with no function, because a report can be older than
// the code, or come from a run with --include-tests.
func joinDry(dir, file string, result *model.Model, source model.Source, stderr io.Writer) error {
	if file == "" {
		return nil
	}
	pairs, err := toolreport.ReadDry(dir, file)
	if err != nil {
		return err
	}
	if unmatched := model.JoinDuplicates(result, source, pairs); unmatched > 0 {
		fmt.Fprintf(stderr, "godeps: godry report sides with no function: %d\n", unmatched)
	}
	return nil
}
