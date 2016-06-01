// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	var includeServerPkg, includeCommonPkg bool
	for _, s := range t.services {
		if err := s.complete(); err != nil {
			return fmt.Errorf("incomplete service %q: %s", s.name, err)
		}

		t.transformRegisterServerFuncs(s)
		if !t.inPRPCPackage {
			includeServerPkg = true
		}

		needsCommonPkg, err := t.generateClients(file, s)
		if err != nil {
			return err
		}
		if needsCommonPkg {
			includeCommonPkg = true
		}
	}
	if includeServerPkg {
		t.insertImport(file, serverPrpcPkg, serverPrpcPackagePath)
	}
	if includeCommonPkg {
		t.insertImport(file, commonPrpcPkg, commonPrpcPackagePath)
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
		registrarType = &ast.SelectorExpr{serverPrpcPkg, registrarName}
	}
	s.registerServerFunc.Params.List[0].Type = registrarType
}

// generateClients finds client interface declarations
// and inserts pRPC implementations after them.
func (t *transformer) generateClients(file *ast.File, s *service) (bool, error) {
	switch newDecls, err := t.generateClient(s.protoPackageName, s.name, s.clientIface); {
	case err != nil:
		return false, err
	case len(newDecls) > 0:
		insertAST(file, s.clientIfaceDecl, newDecls)
		return true, nil
	default:
		return false, nil
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
	client *prpccommon.Client
}

func New{{.Service}}PRPCClient(client *prpccommon.Client) {{.Service}}Client {
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

	err := clientCodeTemplate.Execute(&buf, map[string]interface{}{
		"Service":    serviceName,
		"ProtoPkg":   protoPackage,
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
