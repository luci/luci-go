// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements .go code transformation.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"
	"text/template"
	"unicode/utf8"

	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
)

const (
	serverPrpcPackagePath = `github.com/luci/luci-go/server/prpc`
	commonPrpcPackagePath = `github.com/luci/luci-go/common/prpc`
)

var (
	serverPrpcPkg = ast.NewIdent("prpc")
	commonPrpcPkg = ast.NewIdent("prpccommon")
)

type transformer struct {
	fset          *token.FileSet
	inPRPCPackage bool
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
	t.inPRPCPackage, err = isInPackage(filename, serverPrpcPackagePath)
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

	return ioutil.WriteFile(filename, formatted, 0666)
}

func (t *transformer) transformFile(file *ast.File) error {
	if t.transformRegisterServerFuncs(file) && !t.inPRPCPackage {
		t.insertImport(file, serverPrpcPkg, serverPrpcPackagePath)
	}
	changed, err := t.generateClients(file)
	if err != nil {
		return err
	}
	if changed {
		t.insertImport(file, commonPrpcPkg, commonPrpcPackagePath)
	}
	return nil
}

// transformRegisterServerFuncs finds RegisterXXXServer functions and
// checks its first parameter type to prpc.Registrar.
// Returns true if modified ast.
func (t *transformer) transformRegisterServerFuncs(file *ast.File) bool {
	registrarName := ast.NewIdent("Registrar")
	var registrarType ast.Expr = registrarName
	if !t.inPRPCPackage {
		registrarType = &ast.SelectorExpr{serverPrpcPkg, registrarName}
	}

	changed := false
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := funcDecl.Name.Name
		if !strings.HasPrefix(name, "Register") || !strings.HasSuffix(name, "Server") {
			continue
		}
		params := funcDecl.Type.Params
		if params == nil || len(params.List) != 2 {
			continue
		}

		params.List[0].Type = registrarType
		changed = true
	}
	return changed
}

// generateClients finds client interface declarations
// and inserts pRPC implementations after them.
func (t *transformer) generateClients(file *ast.File) (bool, error) {
	changed := false
	for i := len(file.Decls) - 1; i >= 0; i-- {
		genDecl, ok := file.Decls[i].(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			spec := spec.(*ast.TypeSpec)
			const suffix = "Client"
			if !strings.HasSuffix(spec.Name.Name, suffix) {
				continue
			}
			serviceName := strings.TrimSuffix(spec.Name.Name, suffix)

			iface, ok := spec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}

			newDecls, err := t.generateClient(file.Name.Name, serviceName, iface)
			if err != nil {
				return false, err
			}
			file.Decls = append(file.Decls[:i+1], append(newDecls, file.Decls[i+1:]...)...)
			changed = true
		}
	}
	return changed, nil
}

var clientCodeTemplate = template.Must(template.New("").Parse(`
package template

type {{$.StructName}} struct {
	client *prpccommon.Client
}

func New{{.Service}}PRPCClient(client *prpccommon.Client) {{.Service}}Client {
	return &{{$.StructName}}{client}
}

{{range .Methods}}
func (c *{{$.StructName}}) {{.Name}}(ctx context.Context, in *{{.InputMessage}}, opts ...grpc.CallOption) (*{{.OutputMessage}}, error) {
	out := new({{.OutputMessage}})
	err := c.client.Call(ctx, "{{$.Pkg}}.{{$.Service}}", "{{.Name}}", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
{{end}}
`))

// generateClient generates pRPC implementation of a client interface.
func (t *transformer) generateClient(packageName, serviceName string, iface *ast.InterfaceType) ([]ast.Decl, error) {
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

	err := clientCodeTemplate.Execute(&buf, map[string]interface{}{
		"Pkg":        packageName,
		"Service":    serviceName,
		"StructName": firstLower(serviceName) + "PRPCClient",
		"Methods":    methods,
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
