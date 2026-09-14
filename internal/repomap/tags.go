// Package repomap builds a structural map of a repository — ranked files with
// their key signatures — so an agent can see a codebase's shape before it
// starts grepping through it.
//
// The ranking follows the design used by peer coding agents: parse each source
// file into symbol definitions and references, build a cross-file reference
// graph whose edges are weighted by symbol rarity (IDF), and rank files with
// PageRank. Files that many others depend on come out on top.
//
// The parsing differs from those implementations on purpose. They use
// tree-sitter, a C library; aikota is a Go tool and the Go standard library
// already ships a full Go parser, so the Go half of this costs no dependency
// and no CGO. Other languages can be added behind the same tag model later.
package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

type tagKind uint8

const (
	tagDef tagKind = iota
	tagRef
)

// A tag is one symbol occurrence in a file: either a definition (with the
// signature we render) or a reference (a name used, which becomes a graph edge
// when some other file defines it).
type tag struct {
	Kind      tagKind
	Name      string
	Line      int
	Signature string // definitions only
}

// fileTags is one parsed file's tags plus the package it belongs to, which the
// graph uses to resolve import edges.
type fileTags struct {
	Path    string // repo-relative, slash-separated
	Tags    []tag
	Imports []string // import paths, verbatim
	Package string
}

// extractGoTags parses one Go source file into definitions and references.
//
// A parse failure is not an error for the caller: a repository can legitimately
// contain files that do not compile on their own (build-tagged variants, files
// mid-edit), and a partial map beats no map. The caller records the file as
// unparsed and moves on.
func extractGoTags(relPath string, src []byte) (fileTags, error) {
	fset := token.NewFileSet()
	// SkipObjectResolution drops the deprecated object-identity pass; we only
	// need syntax, and skipping it is meaningfully faster on large trees.
	f, err := parser.ParseFile(fset, relPath, src, parser.SkipObjectResolution)
	if err != nil {
		return fileTags{}, err
	}

	ft := fileTags{Path: relPath, Package: f.Name.Name}
	if f.Name != nil {
		ft.Package = f.Name.Name
	}
	for _, imp := range f.Imports {
		if p := strings.Trim(imp.Path.Value, `"`); p != "" {
			ft.Imports = append(ft.Imports, p)
		}
	}

	line := func(p token.Pos) int { return fset.Position(p).Line }

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			ft.Tags = append(ft.Tags, tag{
				Kind:      tagDef,
				Name:      d.Name.Name,
				Line:      line(d.Pos()),
				Signature: funcSignature(d),
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					ft.Tags = append(ft.Tags, tag{
						Kind:      tagDef,
						Name:      s.Name.Name,
						Line:      line(s.Pos()),
						Signature: "type " + typeSignature(s),
					})
				case *ast.ValueSpec:
					// Grouped const/var blocks share one GenDecl; emit one def
					// per name so each is referenceable.
					for _, n := range s.Names {
						if n.Name == "_" {
							continue
						}
						ft.Tags = append(ft.Tags, tag{
							Kind:      tagDef,
							Name:      n.Name,
							Line:      line(n.Pos()),
							Signature: d.Tok.String() + " " + n.Name,
						})
					}
				}
			}
		}
	}

	// References: every identifier used anywhere in the file. This deliberately
	// over-collects — local variables and field names land here too — because
	// the ranking filters them downstream: a name defined in many files is
	// either dropped by the fan-out cap or weighted down by IDF, and the common
	// identifier list catches the rest.
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name != "_" {
			ft.Tags = append(ft.Tags, tag{Kind: tagRef, Name: id.Name, Line: line(id.Pos())})
		}
		return true
	})

	return ft, nil
}

// funcSignature renders a function or method declaration down to its header,
// with the body omitted.
func funcSignature(d *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if d.Recv != nil && len(d.Recv.List) > 0 {
		// The receiver variable name is noise in a map; keep only its type.
		b.WriteString("(")
		b.WriteString(fieldList(d.Recv.List, true))
		b.WriteString(") ")
	}
	b.WriteString(d.Name.Name)
	b.WriteString(paramsSignature(d.Type))
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		b.WriteString(" ")
		b.WriteString(resultsSignature(d.Type.Results))
	}
	return b.String()
}

// paramsSignature renders a func type's parameter list, collapsing parameter
// names (which add noise without structure) while keeping the types.
func paramsSignature(ft *ast.FuncType) string {
	open, close := "(", ")"
	if ft.Params == nil {
		return open + close
	}
	return open + fieldList(ft.Params.List, true) + close
}

func resultsSignature(fl *ast.FieldList) string {
	if len(fl.List) == 1 && len(fl.List[0].Names) == 0 {
		return typeString(fl.List[0].Type)
	}
	return "(" + fieldList(fl.List, true) + ")"
}

// fieldList renders fields; when typesOnly is set, names are dropped so a
// signature line stays short and structural.
//
// A single field can carry several names (`a, b int`), so dropping the names
// must emit the type once per name — otherwise `func Multi(a, b int)` renders
// as `func Multi(int)` and the arity, which is the only thing a caller needs
// from the signature, is lost.
func fieldList(fields []*ast.Field, typesOnly bool) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		t := typeString(f.Type)
		if typesOnly {
			n := len(f.Names)
			if n == 0 {
				n = 1
			}
			for i := 0; i < n; i++ {
				parts = append(parts, t)
			}
			continue
		}
		if len(f.Names) == 0 {
			parts = append(parts, t)
			continue
		}
		names := make([]string, len(f.Names))
		for i, n := range f.Names {
			names[i] = n.Name
		}
		parts = append(parts, strings.Join(names, ", ")+" "+t)
	}
	return strings.Join(parts, ", ")
}

// typeSignature renders a type declaration's header: "Name struct", "Name
// interface", "Name = int", or "Name []string".
func typeSignature(s *ast.TypeSpec) string {
	switch s.Type.(type) {
	case *ast.StructType:
		return s.Name.Name + " struct"
	case *ast.InterfaceType:
		return s.Name.Name + " interface"
	}
	assign := ""
	if s.Assign.IsValid() {
		assign = " ="
	}
	return s.Name.Name + assign + " " + typeString(s.Type)
}

// typeString renders a type expression, keeping it shallow: composite literals
// are summarised rather than expanded, so one signature line never grows into a
// paragraph.
func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		return "[...]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.StructType:
		return "struct{...}"
	case *ast.InterfaceType:
		return "interface{...}"
	case *ast.FuncType:
		return "func" + paramsSignature(t)
	default:
		return "?"
	}
}

// isGoSource reports whether the path names a Go source file worth parsing.
func isGoSource(relPath string) bool {
	base := filepath.Base(relPath)
	if strings.HasSuffix(base, "_test.go") {
		// Test files are included: they carry real structural signal (they tell
		// you which package a concept belongs to), and the ranking decides
		// whether they matter.
		return true
	}
	return strings.HasSuffix(base, ".go")
}
