package godeps

import (
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/model"
	"github.com/n-valchev/goalie/internal/toolreport"
)

// joinArchGo warns and does not fail on lines it can't use, because arch-go stays the gate and a
// map with one red arrow fewer is still useful. The count shows a change in arch-go's message text.
func joinArchGo(dir, file string, result *model.Model, source model.Source, stderr io.Writer) error {
	if file == "" {
		return nil
	}
	report, err := toolreport.ReadArchGo(dir, file)
	if err != nil {
		return err
	}
	if report.Unparsed > 0 {
		fmt.Fprintf(stderr, "godeps: arch-go lines with no imported package: %d\n", report.Unparsed)
	}
	if unplaced := model.MarkBreaks(result, source, report.Breaks); unplaced > 0 {
		fmt.Fprintf(stderr, "godeps: arch-go breaks with no edge: %d\n", unplaced)
	}
	return nil
}
