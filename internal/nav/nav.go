// Package nav implements source navigation: `where` scans the project with
// go/parser (no session needed); def/hover/refs shell out to gopls.
package nav

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
)

// Where finds declarations (funcs, methods, types, consts, vars) matching
// name anywhere under root.
func Where(root, name string) (string, error) {
	fset := token.NewFileSet()
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base != "." && (strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" || base == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, decl := range f.Decls {
			switch dd := decl.(type) {
			case *ast.FuncDecl:
				if dd.Name.Name != name {
					continue
				}
				pos := fset.Position(dd.Pos())
				recv := ""
				if dd.Recv != nil && len(dd.Recv.List) == 1 {
					recv = "(" + typeString(dd.Recv.List[0].Type) + ") "
				}
				hits = append(hits, fmt.Sprintf("%s:%d  func %s%s%s", rel, pos.Line, recv, name, signature(dd)))
			case *ast.GenDecl:
				for _, spec := range dd.Specs {
					switch sp := spec.(type) {
					case *ast.TypeSpec:
						if sp.Name.Name == name {
							pos := fset.Position(sp.Pos())
							hits = append(hits, fmt.Sprintf("%s:%d  type %s", rel, pos.Line, name))
						}
					case *ast.ValueSpec:
						for _, id := range sp.Names {
							if id.Name == name {
								pos := fset.Position(id.Pos())
								kind := "var"
								if dd.Tok == token.CONST {
									kind = "const"
								}
								hits = append(hits, fmt.Sprintf("%s:%d  %s %s", rel, pos.Line, kind, name))
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return fmt.Sprintf("no declaration of %q found under %s", name, root), nil
	}
	return strings.Join(hits, "\n"), nil
}

func signature(fd *ast.FuncDecl) string {
	var params, results []string
	if fd.Type.Params != nil {
		for _, p := range fd.Type.Params.List {
			params = append(params, fieldString(p))
		}
	}
	sig := "(" + strings.Join(params, ", ") + ")"
	if fd.Type.Results != nil {
		for _, r := range fd.Type.Results.List {
			results = append(results, fieldString(r))
		}
		if len(results) == 1 && !strings.Contains(results[0], " ") {
			sig += " " + results[0]
		} else {
			sig += " (" + strings.Join(results, ", ") + ")"
		}
	}
	return sig
}

func fieldString(f *ast.Field) string {
	t := typeString(f.Type)
	if len(f.Names) == 0 {
		return t
	}
	names := make([]string, len(f.Names))
	for i, n := range f.Names {
		names[i] = n.Name
	}
	return strings.Join(names, ", ") + " " + t
}

func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.FuncType:
		return "func(…)"
	case *ast.InterfaceType:
		return "interface{…}"
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	case *ast.IndexExpr:
		return typeString(t.X) + "[…]"
	default:
		return "?"
	}
}

// Gopls runs a gopls query (definition|references|hover) at file:line:col.
func Gopls(root, verb, file string, line, col int) (string, error) {
	if _, err := exec.LookPath("gopls"); err != nil {
		return "", fmt.Errorf("gopls not found on PATH; install it with: go install golang.org/x/tools/gopls@latest")
	}
	abs := file
	if !filepath.IsAbs(file) {
		abs = filepath.Join(root, file)
	}
	loc := fmt.Sprintf("%s:%d:%d", abs, line, col)
	if verb == "hover" {
		// gopls has no `hover` subcommand; `signature` is the closest.
		verb = "signature"
	}
	cmd := exec.Command("gopls", verb, loc)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gopls %s: %v\n%s", verb, err, out)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "no result", nil
	}
	return text, nil
}
