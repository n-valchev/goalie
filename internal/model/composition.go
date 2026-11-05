package model

import (
	"cmp"
	"go/ast"
)

const kindComposition = "composition"

// fileEmbeds is held until every file is read, because an import with no alias resolves on the
// package clause of its target, and that file may not be parsed yet.
type fileEmbeds struct {
	from       string
	qualifiers []string // The pkg of each embedded pkg.Type.
	imports    []*ast.ImportSpec
}

func readEmbeds(from string, parsed *ast.File) fileEmbeds {
	embeds := fileEmbeds{from: from, imports: parsed.Imports}
	for _, spec := range typeSpecs(parsed) {
		ast.Inspect(spec, func(node ast.Node) bool {
			if typ, ok := node.(*ast.StructType); ok {
				embeds.qualifiers = append(embeds.qualifiers, embeddedQualifiers(typ)...)
			}
			return true
		})
	}
	return embeds
}

func embeddedQualifiers(typ *ast.StructType) []string {
	var qualifiers []string
	for _, field := range typ.Fields.List {
		if len(field.Names) > 0 {
			continue
		}
		if selector, ok := embeddedBase(field.Type).(*ast.SelectorExpr); ok {
			if qualifier, ok := selector.X.(*ast.Ident); ok {
				qualifiers = append(qualifiers, qualifier.Name)
			}
		}
	}
	return qualifiers
}

func compositionEdges(files []fileEmbeds, classes []Class) []Edge {
	clauses := make(map[string]string, len(classes))
	for _, class := range classes {
		clauses[class.ID] = class.Name
	}
	var edges []Edge
	for _, file := range files {
		targets := importedClasses(file.imports, clauses)
		for _, qualifier := range file.qualifiers {
			if target, ok := targets[qualifier]; ok && target != file.from {
				edges = append(edges, Edge{file.from, target, kindComposition})
			}
		}
	}
	return edges
}

// importedClasses maps the name a file uses for an import to the class it reaches. Imports that
// reach no class drop out here, which also keeps a same-named outside package from matching.
func importedClasses(imports []*ast.ImportSpec, clauses map[string]string) map[string]string {
	targets := make(map[string]string, len(imports))
	for _, spec := range imports {
		target := importPath(spec)
		name, scanned := clauses[target]
		if !scanned {
			continue
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		targets[name] = target
	}
	return targets
}

func compareEdges(a, b Edge) int {
	return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To), cmp.Compare(a.Kind, b.Kind))
}
