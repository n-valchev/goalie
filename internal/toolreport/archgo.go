package toolreport

import (
	"fmt"
	"regexp"
)

// importedPackage covers both message forms of arch-go: "contains imported package" and "doesn't
// contains imported package". The report holds the imported path nowhere else.
var importedPackage = regexp.MustCompile(`imported package '([^']+)'`)

type Break struct {
	Rule    string
	Package string
	Import  string
	Detail  string
}

type ArchGo struct {
	Breaks   []Break
	Unparsed int // Lines of breaks that name no import.
}

type archGoFile struct {
	Compliance *struct {
		Details struct {
			DependenciesRules struct {
				Details []struct {
					Rule           string `json:"rule"`
					PackageDetails []struct {
						Package string   `json:"package"`
						Pass    bool     `json:"pass"`
						Details []string `json:"details"`
					} `json:"packageDetails"`
				} `json:"details"`
			} `json:"dependenciesRules"`
		} `json:"details"`
	} `json:"compliance"`
}

func ReadArchGo(dir, file string) (ArchGo, error) {
	var report archGoFile
	if err := readJSON(dir, file, &report); err != nil {
		return ArchGo{}, err
	}
	// Any JSON decodes into archGoFile, so a gocrap or godry report would pass as one with no breaks.
	if report.Compliance == nil {
		return ArchGo{}, fmt.Errorf("%s: not an arch-go report: no compliance", file)
	}
	var result ArchGo
	for _, rule := range report.Compliance.Details.DependenciesRules.Details {
		for _, entry := range rule.PackageDetails {
			if entry.Pass {
				continue
			}
			for _, line := range entry.Details {
				match := importedPackage.FindStringSubmatch(line)
				if match == nil {
					result.Unparsed++
					continue
				}
				result.Breaks = append(result.Breaks, Break{rule.Rule, entry.Package, match[1], line})
			}
		}
	}
	return result, nil
}
