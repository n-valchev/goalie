package godeps

import (
	"fmt"
	"io"

	"github.com/n-valchev/goalie/internal/model"
	"github.com/n-valchev/goalie/internal/toolreport"
)

// joinReports warns and does not fail on entries it can't place. A report can be older than the
// code or cover code that this run left out, and the rest of the map is still right. arch-go stays
// the gate, and its counts show a change in its message text.
func joinReports(dir string, opts options, result *model.Model, source model.Source, stderr io.Writer) error {
	warn := func(what string, count int) {
		if count > 0 {
			fmt.Fprintf(stderr, "godeps: %s: %d\n", what, count)
		}
	}
	err := joinReport(dir, opts.archGoReport, toolreport.ReadArchGo, func(report toolreport.ArchGo) {
		warn("arch-go lines with no imported package", report.Unparsed)
		warn("arch-go breaks with no edge", model.MarkBreaks(result, source, report.Breaks))
	})
	if err != nil {
		return err
	}
	err = joinReport(dir, opts.dupReport, toolreport.ReadDry, func(pairs []toolreport.DryPair) {
		warn("dup report sides with no function", model.JoinDuplicates(result, source, pairs))
	})
	if err != nil {
		return err
	}
	return joinReport(dir, opts.crapReport, toolreport.ReadCrap, func(rows []toolreport.CrapRow) {
		warn("crap rows with no function", model.JoinCrap(result, rows, opts.limits))
	})
}

func joinReport[R any](dir, file string, read func(dir, file string) (R, error), join func(R)) error {
	if file == "" {
		return nil
	}
	report, err := read(dir, file)
	if err != nil {
		return err
	}
	join(report)
	return nil
}
