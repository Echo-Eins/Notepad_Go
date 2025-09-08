package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os/exec"
	"strings"
)

// extractGoSignature uses the Go AST to parse a single function declaration
// line and returns its FunctionSignature. It returns nil if parsing fails.
func extractGoSignature(line string) *FunctionSignature {
	// Wrap the line into a minimal Go file so the parser can handle it.
	src := fmt.Sprintf("package p\n%s{}", line)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "snippet.go", src, 0)
	if err != nil {
		return nil
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		sig := &FunctionSignature{Name: fn.Name.Name}
		if fn.Type.Params != nil {
			for _, field := range fn.Type.Params.List {
				typ := exprString(field.Type)
				if len(field.Names) == 0 {
					sig.Parameters = append(sig.Parameters, Parameter{Type: typ})
					continue
				}
				for _, name := range field.Names {
					sig.Parameters = append(sig.Parameters, Parameter{Name: name.Name, Type: typ})
				}
			}
		}
		if fn.Type.Results != nil {
			var results []string
			for _, field := range fn.Type.Results.List {
				typ := exprString(field.Type)
				if len(field.Names) > 0 {
					for range field.Names {
						results = append(results, typ)
					}
				} else {
					results = append(results, typ)
				}
			}
			sig.ReturnType = strings.Join(results, ", ")
		}
		return sig
	}
	return nil
}

// extractPythonSignature uses the Python AST (via the system's Python
// interpreter) to parse a single function declaration line. It returns nil
// if parsing fails or Python is unavailable.
func extractPythonSignature(line string) *FunctionSignature {
	// Build a tiny Python module from the line.
	script := fmt.Sprintf(`import ast, json
mod = ast.parse("""%s\n    pass""", mode='exec')
fn = mod.body[0]
print(json.dumps({'name': fn.name, 'params': [a.arg for a in fn.args.args]}))`, line)
	out, err := exec.Command("python3", "-c", script).Output()
	if err != nil {
		return nil
	}
	var data struct {
		Name   string   `json:"name"`
		Params []string `json:"params"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil
	}
	sig := &FunctionSignature{Name: data.Name}
	for _, p := range data.Params {
		sig.Parameters = append(sig.Parameters, Parameter{Name: p})
	}
	return sig
}

func exprString(e ast.Expr) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, token.NewFileSet(), e)
	return buf.String()
}
