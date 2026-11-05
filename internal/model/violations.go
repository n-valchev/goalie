package model

import "github.com/n-valchev/goalie/internal/toolreport"

// Violation is one arch-go rule that an edge breaks. It is a list on the edge, because two rules
// can flag one import.
type Violation struct {
	Rule    string   `json:"rule"`
	Details []string `json:"details"`
}

// MarkBreaks returns how many breaks have no edge.
func MarkBreaks(result *Model, source Source, breaks []toolreport.Break) int {
	edges := map[[2]string]*Edge{}
	for i := range result.Edges {
		if edge := &result.Edges[i]; edge.Kind == kindDependency {
			edges[[2]string{edge.From, edge.To}] = edge
		}
	}
	unplaced := 0
	for _, found := range breaks {
		target := found.Import
		if module := foreignModule(found.Import, source); module != "" {
			target = module
		}
		if edge := edges[[2]string{found.Package, target}]; edge != nil {
			edge.addBreak(found)
		} else {
			unplaced++
		}
	}
	return unplaced
}

// addBreak groups by rule, because one rule can flag two imports that share a foreign edge.
func (e *Edge) addBreak(found toolreport.Break) {
	for i := range e.Violations {
		if violation := &e.Violations[i]; violation.Rule == found.Rule {
			violation.Details = append(violation.Details, found.Detail)
			return
		}
	}
	e.Violations = append(e.Violations, Violation{found.Rule, []string{found.Detail}})
}
