// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements .go code transformation.

package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
)

const (
	prpcPackagePath = `"github.com/luci/luci-go/server/prpc"`
)

var (
	prpcPkg       = ast.NewIdent("prpc")
	registrarType = &ast.SelectorExpr{prpcPkg, ast.NewIdent("Registrar")}
)

// transformGoFile rewrites a .go file to work with prpc.
func transformGoFile(filename string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return err
	}

	transformFile(file)

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()
	return printer.Fprint(out, fset, file)
}

func transformFile(file *ast.File) {
	if transformRegisterServerFuncs(file) {
		insertPrpcImport(file)
	}
}

// transformRegisterServerFuncs finds RegisterXXXServer functions and
// checks its first parameter type to prpc.Registrar.
// Returns true if modified ast.
func transformRegisterServerFuncs(file *ast.File) bool {
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

func insertPrpcImport(file *ast.File) {
	spec := &ast.ImportSpec{
		Name: prpcPkg,
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: prpcPackagePath,
		},
	}
	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{spec},
	}
	file.Decls = append([]ast.Decl{importDecl}, file.Decls...)
}
