package model

import (
	"go/ast"
	"go/token"

	"github.com/n-valchev/goalie/internal/extract"
)

const (
	kindStruct    = "struct"
	kindInterface = "interface"
	kindOther     = "other"
)

type Type struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Fields  []Field  `json:"fields,omitempty"`
	Methods []Method `json:"methods,omitempty"`
}

// Field is one name of a struct field. An embedded field has the name Go gives it, the bare type name.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Method is one line of an interface. An embedded interface or a constraint such as `~int | ~string`
// has no name of its own, so its source text is the Name and it has no Signature.
type Method struct {
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
}

type Op struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	EndLine  int    `json:"endLine"`
	Exported bool   `json:"exported"`
}

// fileReader turns one parsed file into model entries.
type fileReader struct {
	fset   *token.FileSet
	file   string
	source []byte
}

// line ignores //line directives, because gocrap and godry rows match on the line in the file.
func (r fileReader) line(pos token.Pos) int {
	return r.fset.PositionFor(pos, false).Line
}

// text slices the file instead of printing the node, so that a card shows what the author wrote.
func (r fileReader) text(node ast.Node) string {
	return string(r.source[r.offset(node.Pos()):r.offset(node.End())])
}

func (r fileReader) offset(pos token.Pos) int {
	return r.fset.PositionFor(pos, false).Offset
}

func (r fileReader) ops(parsed *ast.File) []Op {
	var ops []Op
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			ops = append(ops, Op{
				Name:     opName(fn),
				File:     r.file,
				Line:     r.line(fn.Pos()),
				EndLine:  r.line(fn.End() - 1),
				Exported: fn.Name.IsExported(),
			})
		}
	}
	return ops
}

func opName(fn *ast.FuncDecl) string {
	if receiver := extract.ReceiverType(fn); receiver != "" {
		return receiver + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func (r fileReader) types(parsed *ast.File) []Type {
	var types []Type
	for _, spec := range typeSpecs(parsed) {
		entry := Type{Name: spec.Name.Name, Kind: kindOther, File: r.file, Line: r.line(spec.Pos())}
		switch typ := ast.Unparen(spec.Type).(type) {
		case *ast.StructType:
			entry.Kind, entry.Fields = kindStruct, r.fields(typ)
		case *ast.InterfaceType:
			entry.Kind, entry.Methods = kindInterface, r.methods(typ)
		}
		types = append(types, entry)
	}
	return types
}

// typeSpecs leaves out types declared inside a function, which are no part of a package's shape.
func typeSpecs(parsed *ast.File) []*ast.TypeSpec {
	var specs []*ast.TypeSpec
	for _, decl := range parsed.Decls {
		if group, ok := decl.(*ast.GenDecl); ok && group.Tok == token.TYPE {
			for _, spec := range group.Specs {
				specs = append(specs, spec.(*ast.TypeSpec))
			}
		}
	}
	return specs
}

func (r fileReader) fields(typ *ast.StructType) []Field {
	var fields []Field
	for _, field := range typ.Fields.List {
		text := r.text(field.Type)
		if len(field.Names) == 0 {
			fields = append(fields, Field{embeddedName(field.Type), text})
		}
		for _, name := range field.Names {
			fields = append(fields, Field{name.Name, text})
		}
	}
	return fields
}

func embeddedName(expr ast.Expr) string {
	switch typ := embeddedBase(expr).(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		return typ.Sel.Name
	}
	return ""
}

// embeddedBase strips what may wrap the type name of an embedded field: `*pkg.Type[K, V]`.
func embeddedBase(expr ast.Expr) ast.Expr {
	for {
		switch typ := expr.(type) {
		case *ast.StarExpr:
			expr = typ.X
		case *ast.IndexExpr:
			expr = typ.X
		case *ast.IndexListExpr:
			expr = typ.X
		default:
			return expr
		}
	}
}

func (r fileReader) methods(typ *ast.InterfaceType) []Method {
	var methods []Method
	for _, field := range typ.Methods.List {
		if len(field.Names) == 0 {
			methods = append(methods, Method{Name: r.text(field.Type)})
			continue
		}
		methods = append(methods, Method{field.Names[0].Name, r.text(field.Type)})
	}
	return methods
}
