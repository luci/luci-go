// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This file implements .go code transformation.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"text/template"
	"unicode/utf8"
)

const (
	prpcPackagePath = `go.chromium.org/luci/grpc/prpc`
)

var (
	serverPrpcPkg = ast.NewIdent("prpc")
)

type transformer struct {
	fset          *token.FileSet
	inPRPCPackage bool
	services      []*service
	PackageName   string
}

// transformGoFile rewrites a .go file to work with prpc.
func (t *transformer) transformGoFile(filename string) error {
	t.fset = token.NewFileSet()
	file, err := parser.ParseFile(t.fset, filename, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	t.PackageName = file.Name.Name
	t.services, err = getServices(file)
	if err != nil {
		return err
	}
	if len(t.services) == 0 {
		return nil
	}

	t.inPRPCPackage, err = isInPackage(filename, prpcPackagePath)
	if err != nil {
		return err
	}

	if err := t.transformFile(file); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, t.fset, file); err != nil {
		return err
	}
	formatted, err := gofmt(buf.Bytes())
	if err != nil {
		return err
	}

	return os.WriteFile(filename, formatted, 0666)
}

func (t *transformer) transformFile(file *ast.File) error {
	var includePrpc bool
	for _, s := range t.services {
		t.transformRegisterServerFuncs(s)
		if !t.inPRPCPackage {
			includePrpc = true
		}

		if err := t.generateClients(file, s); err != nil {
			return err
		}
	}
	if includePrpc {
		t.insertImport(file, serverPrpcPkg, prpcPackagePath)
	}
	return nil
}

// transformRegisterServerFuncs finds RegisterXXXServer functions and
// checks its first parameter type to prpc.Registrar.
// Returns true if modified ast.
func (t *transformer) transformRegisterServerFuncs(s *service) {
	registrarName := ast.NewIdent("Registrar")
	var registrarType ast.Expr = registrarName
	if !t.inPRPCPackage {
		registrarType = &ast.SelectorExpr{X: serverPrpcPkg, Sel: registrarName}
	}
	s.registerServerFunc.Params.List[0].Type = registrarType
}

// generateClients finds client interface declarations
// and inserts pRPC implementations after them.
func (t *transformer) generateClients(file *ast.File, s *service) error {
	switch newDecls, err := t.generateClient(s.protoPackageName, s.name, s.clientIface); {
	case err != nil:
		return err
	case len(newDecls) > 0:
		insertAST(file, s.clientIfaceDecl, newDecls)
		return nil
	default:
		return nil
	}
}

func insertAST(file *ast.File, after ast.Decl, newDecls []ast.Decl) {
	for i, d := range file.Decls {
		if d == after {
			file.Decls = append(file.Decls[:i+1], append(newDecls, file.Decls[i+1:]...)...)
			return
		}
	}
	panic("unable to find after node")
}

var clientCodeTemplate = template.Must(template.New("").Parse(`
package template

type {{$.StructName}} struct {
	client *{{.PRPCSymbolPrefix}}Client
}

func New{{.Service}}PRPCClient(client *{{.PRPCSymbolPrefix}}Client) {{.Service}}Client {
	return &{{$.StructName}}{client}
}

{{range .Methods}}
func (c *{{$.StructName}}) {{.Name}}(ctx context.Context, in *{{.InputMessage}}, opts ...grpc.CallOption) (*{{.OutputMessage}}, error) {
	out := new({{.OutputMessage}})
	err := c.client.Call(ctx, "{{$.ProtoPkg}}.{{$.Service}}", "{{.Name}}", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
{{end}}
`))

// generateClient generates pRPC implementation of a client interface.
func (t *transformer) generateClient(protoPackage, serviceName string, iface *ast.InterfaceType) ([]ast.Decl, error) {
	// This function used to construct an AST. It was a lot of code.
	// Now it generates code via a template and parses back to AST.
	// Slower, but saner and easier to make changes.

	type Method struct {
		Name          string
		InputMessage  string
		OutputMessage string
	}
	methods := make([]Method, 0, len(iface.Methods.List))

	var buf bytes.Buffer
	toGoCode := func(n ast.Node) (string, error) {
		defer buf.Reset()
		err := format.Node(&buf, t.fset, n)
		if err != nil {
			return "", err
		}
		return buf.String(), nil
	}

	for _, m := range iface.Methods.List {
		signature, ok := m.Type.(*ast.FuncType)
		if !ok {
			return nil, fmt.Errorf("unexpected embedded interface in %sClient", serviceName)
		}

		inStructPtr := signature.Params.List[1].Type.(*ast.StarExpr)
		inStruct, err := toGoCode(inStructPtr.X)
		if err != nil {
			return nil, err
		}

		outStructPtr := signature.Results.List[0].Type.(*ast.StarExpr)
		outStruct, err := toGoCode(outStructPtr.X)
		if err != nil {
			return nil, err
		}

		methods = append(methods, Method{
			Name:          m.Names[0].Name,
			InputMessage:  inStruct,
			OutputMessage: outStruct,
		})
	}

	prpcSymbolPrefix := "prpc."
	if t.inPRPCPackage {
		prpcSymbolPrefix = ""
	}
	err := clientCodeTemplate.Execute(&buf, map[string]any{
		"Service":          serviceName,
		"ProtoPkg":         protoPackage,
		"StructName":       firstLower(serviceName) + "PRPCClient",
		"Methods":          methods,
		"PRPCSymbolPrefix": prpcSymbolPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("client template execution: %s", err)
	}

	f, err := parser.ParseFile(t.fset, "", buf.String(), 0)
	if err != nil {
		return nil, fmt.Errorf("client template result parsing: %s. Code: %#v", err, buf.String())
	}
	return f.Decls, nil
}

func (t *transformer) insertImport(file *ast.File, name *ast.Ident, path string) {
	spec := &ast.ImportSpec{
		Name: name,
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + path + `"`,
		},
	}
	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{spec},
	}
	file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
}

func firstLower(s string) string {
	_, w := utf8.DecodeRuneInString(s)
	return strings.ToLower(s[:w]) + s[w:]
}

func transformGoFile(name string, input []byte, filter func(*ast.File) error) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, input, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	if err := filter(file); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return nil, err
	}
	return gofmt(buf.Bytes())
}
