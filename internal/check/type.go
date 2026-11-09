package check

import (
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/n-valchev/goalie/internal/extract"
	"github.com/n-valchev/goalie/internal/gitchange"
	"github.com/n-valchev/goalie/internal/rules"
)

type TypeMeasure struct {
	Name         string
	DefaultLimit float64
	Of           func(typ extract.Type, methods []extract.Function) rules.TypeMeasurement
}

var LowCohesion = TypeMeasure{Name: "low-cohesion", DefaultLimit: 1, Of: rules.LowCohesion}

type Type struct {
	Limited
	Measure TypeMeasure
}

func NewType(scope Scope, measure TypeMeasure) *Type {
	return &Type{Limited: Limited{Scope: scope, Name: measure.Name, Limit: measure.DefaultLimit}, Measure: measure}
}

type typeMeasured struct {
	File   string     `json:"file"`
	Line   int        `json:"line"`
	Type   string     `json:"type"`
	Groups [][]string `json:"groups"`
}

// Results reports a type in scope changes when the change touches its declaration or any of its
// methods, because a new method can split a type that the change never declared.
func (t Type) Results(dir string, stderr io.Writer) ([]Result, error) {
	paths, changed, err := t.changedFiles(dir, stderr)
	if err != nil {
		return nil, err
	}
	if t.Base != "" {
		// The methods in the unchanged files of a package still decide its groups.
		if paths, err = t.packageFiles(dir, paths); err != nil {
			return nil, err
		}
	}
	types, err := t.Parser.Types(dir, paths, t.Measure.Name)
	if err != nil {
		return nil, err
	}
	methods, err := t.methods(dir, paths)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, typ := range types {
		own := methods[typ.Package+"."+typ.Name]
		if t.Base != "" && !touches(changed, typ, own) {
			continue
		}
		if measured := t.Measure.Of(typ, own); float64(measured.Value) > t.Limit {
			results = append(results, t.result(typ, measured))
		}
	}
	return results, nil
}

// packageFiles returns every file outside tests in the folders of changed.
func (t Type) packageFiles(dir string, changed []string) ([]string, error) {
	folders := map[string]bool{}
	for _, file := range changed {
		folders[path.Dir(file)] = true
	}
	all, err := t.Files.find(t.Repo, dir, false)
	return slices.DeleteFunc(all, func(file string) bool { return !folders[path.Dir(file)] }), err
}

// methods passes no rule name, because a directive above a method doesn't take it out of the
// groups. A call to it must still name a method, not an embedded field.
func (t Type) methods(dir string, paths []string) (map[string][]extract.Function, error) {
	functions, err := t.Parser.Parse(dir, paths, "")
	byType := map[string][]extract.Function{}
	for _, fn := range functions {
		if fn.Receiver != "" {
			key := fn.Package + "." + fn.Receiver
			byType[key] = append(byType[key], fn)
		}
	}
	return byType, err
}

func touches(changed gitchange.Changes, typ extract.Type, methods []extract.Function) bool {
	return changed.Touches(typ.File, typ.StartLine, typ.EndLine) ||
		slices.ContainsFunc(methods, func(fn extract.Function) bool {
			return changed.Touches(fn.File, fn.StartLine, fn.EndLine)
		})
}

func (t Type) result(typ extract.Type, measured rules.TypeMeasurement) Result {
	var groups []string
	for _, group := range measured.Groups {
		groups = append(groups, strings.Join(group, ", "))
	}
	return Result{
		Rule: t.Measure.Name, Value: float64(measured.Value), Limit: t.Limit, File: typ.File, Line: typ.StartLine,
		Message: fmt.Sprintf("%s has %d groups of methods: %s (limit %g)", typ.Name, measured.Value,
			strings.Join(groups, "; "), t.Limit),
		Details: typeMeasured{File: typ.File, Line: typ.StartLine, Type: typ.Name, Groups: measured.Groups},
		About:   []Place{{typ.File, typ.Name}},
	}
}
