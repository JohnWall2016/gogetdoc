package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

func builtinPackage(fs *token.FileSet) *ast.Package {
	buildPkg, err := build.Import("builtin", "", build.ImportComment)
	// should never fail
	if err != nil {
		panic(err)
	}
	include := func(info os.FileInfo) bool {
		return info.Name() == "builtin.go"
	}
	astPkgs, err := parser.ParseDir(fs, buildPkg.Dir, include, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	return astPkgs["builtin"]
}

func readFuncType(ft *ast.FuncType) string {
	pars := []string{}
	if ft.Params != nil {
		for _, field := range ft.Params.List {
			finfo := readField(field)
			pars = append(pars, finfo)
		}
	}
	ress := []string{}
	if ft.Results != nil {
		for _, field := range ft.Results.List {
			rinfo := readField(field)
			ress = append(ress, rinfo)
		}
	}
	info := fmt.Sprintf("(%s)", strings.Join(pars, ", "))
	if len(ress) == 1 {
		info += " " + ress[0]
	} else if len(ress) > 1 {
		info += fmt.Sprintf(" (%s)", strings.Join(ress, ", "))
	}

	return info
}

func readField(field *ast.Field) string {
	name := ""
	for _, nm := range field.Names {
		name = nm.Name
	}
	typ := ""
	if field.Type != nil {
		switch ft := field.Type.(type) {
		case *ast.FuncType:
			typ = readFuncType(ft)
			return name + typ
		case *ast.Ident:
			typ = ft.Name
		case *ast.ArrayType:
			if ft.Elt != nil {
				if id, ok := ft.Elt.(*ast.Ident); ok {
					typ = "[]" + id.String()
				}
			}
		case *ast.Ellipsis:
			if ft.Elt != nil {
				if id, ok := ft.Elt.(*ast.Ident); ok {
					typ = "..." + id.String()
				}
			}
		case *ast.MapType:
			key := ""
			if ft.Key != nil {
				if id, ok := ft.Key.(*ast.Ident); ok {
					key = id.String()
				}
			}
			value := ""
			if ft.Value != nil {
				if id, ok := ft.Value.(*ast.Ident); ok {
					value = id.String()
				}
			}
			typ = fmt.Sprintf("map[%s]%s", key, value)
		case *ast.StarExpr:
			if ft.X != nil {
				if id, ok := ft.X.(*ast.Ident); ok {
					typ = "*" + id.Name
				}
			}
		case *ast.ChanType:
			if ft.Value != nil {
				if id, ok := ft.Value.(*ast.Ident); ok {
					val := id.Name
					if ft.Dir == ast.SEND {
						typ = "chan<- " + val
					} else if ft.Dir == ast.RECV {
						typ = "<-chan " + val
					} else {
						typ = "chan " + val
					}
				}
			}
		case *ast.InterfaceType:
			typ = readInterfaceType(ft)
		default:
			fmt.Printf("%#v\n", field)
		}
	}
	if name == "" {
		return typ
	} else if typ == "" {
		return name
	} else {
		return name + " " + typ
	}
}

func readInterfaceType(it *ast.InterfaceType) string {
	fields := []string{}
	if it.Methods != nil {
		for _, field := range it.Methods.List {
			fields = append(fields, readField(field))
		}
	}
	if len(fields) == 0 {
		return "interface{}"
	} else {
		return fmt.Sprintf("interface {\n\t%s\n}", strings.Join(fields, "\n\t"))
	}
}

func getValueSpec(name string, v *ast.ValueSpec, vt token.Token, vdoc *ast.CommentGroup, fs *token.FileSet) *Doc {
	for _, nm := range v.Names {
		if name == nm.Name {
			typ := ""
			if v.Type != nil {
				if t, ok := v.Type.(*ast.Ident); ok {
					typ = t.Name
				}
			}

			values := []string{}
			for _, expr := range v.Values {
				if basicLit, ok := expr.(*ast.BasicLit); ok {
					values = append(values, basicLit.Value)
				}
				if binExpr, ok := expr.(*ast.BinaryExpr); ok {
					if x, ok := binExpr.X.(*ast.BasicLit); ok {
						if y, ok := binExpr.X.(*ast.BasicLit); ok {
							values = append(values, fmt.Sprintf("%s %s %s", x.Value, binExpr.Op, y.Value))
						}
					}
				}
			}

			doc := &Doc{}
			doc.Name = name

			decl := fmt.Sprintf("%s %s", vt, doc.Name)
			if typ != "" {
				decl = fmt.Sprintf("%s %s", decl, typ)
			}
			if len(values) > 0 {
				decl = fmt.Sprintf("%s = %s", decl, strings.Join(values, ", "))
			}
			doc.Decl = decl

			pos := nm.Pos()
			if pos.IsValid() {
				doc.Pos = fs.Position(pos).String()
			}

			sdoc := ""
			if v.Doc != nil {
				sdoc = v.Doc.Text()
			} else if vdoc != nil {
				sdoc = vdoc.Text()
			}
			doc.Doc = sdoc

			return doc
		}
	}

	return nil
}

func getTypeSpec(name string, t *ast.TypeSpec, tt token.Token, tdoc *ast.CommentGroup, fs *token.FileSet) *Doc {
	if name != t.Name.Name {
		return nil
	}

	doc := &Doc{}

	doc.Name = name

	decl := fmt.Sprintf("%s %s", tt, doc.Name)
	typ := ""
	if t.Type != nil {
		if ti, ok := t.Type.(*ast.Ident); ok {
			typ = ti.Name
		} else if ti, ok := t.Type.(*ast.InterfaceType); ok {
			typ += readInterfaceType(ti)
		}
	}
	if typ != "" {
		if t.Assign != 0 {
			decl = fmt.Sprintf("%s = %s", decl, typ)
		} else {
			decl = fmt.Sprintf("%s %s", decl, typ)
		}
	}
	doc.Decl = decl

	pos := t.Name.Pos()
	if pos.IsValid() {
		doc.Pos = fs.Position(pos).String()
	}

	sdoc := ""
	if t.Doc != nil {
		sdoc = t.Doc.Text()
	} else if tdoc != nil {
		sdoc = tdoc.Text()
	}
	doc.Doc = sdoc

	return doc
}

func getFuncDecl(name string, f *ast.FuncDecl, fs *token.FileSet) *Doc {
	if f.Name.Name != name {
		return nil
	}

	doc := &Doc{}

	doc.Name = name

	if f.Doc != nil {
		doc.Doc = f.Doc.Text()
	}

	pos := f.Name.Pos()
	if pos.IsValid() {
		doc.Pos = fs.Position(pos).String()
	}

	decl := "func"
	recv := []string{}
	if f.Recv != nil {
		for _, field := range f.Recv.List {
			recv = append(recv, readField(field))
		}
	}
	if len(recv) > 0 {
		decl += " (" + strings.Join(recv, ", ") + ")"
	}
	decl += " " + doc.Name
	decl += readFuncType(f.Type)
	doc.Decl = decl

	return doc
}

func findBuiltinType(name string) *Doc {
	fs := token.NewFileSet()
	pkg := builtinPackage(fs)

	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			var doc *Doc
			switch d := decl.(type) {
			case *ast.GenDecl:
				switch d.Tok {
				case token.CONST, token.VAR:
					for _, spec := range d.Specs {
						doc = getValueSpec(name, spec.(*ast.ValueSpec), d.Tok, d.Doc, fs)
						if doc != nil {
							break
						}
					}
				case token.TYPE:
					for _, spec := range d.Specs {
						doc = getTypeSpec(name, spec.(*ast.TypeSpec), d.Tok, d.Doc, fs)
						if doc != nil {
							break
						}
					}
				}
			case *ast.FuncDecl:
				doc = getFuncDecl(name, d, fs)
			}
			if doc != nil {
				doc.Import = "builtin"
				doc.Pkg = "builtin"
				return doc
			}
		}
	}
	return nil
}
