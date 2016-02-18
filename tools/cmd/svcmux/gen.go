// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/svcmux"
)

func (g *generator) genMuxType(c context.Context, typeName string) (*muxType, error) {
	typeSpec := g.findType(typeName)
	if typeSpec == nil {
		return nil, fmt.Errorf("type %s not found", typeName)
	}
	iface, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		return nil, fmt.Errorf("%s is not an interface", typeName)
	}

	const suffix = "Server"
	if !strings.HasSuffix(typeName, suffix) {
		return nil, fmt.Errorf("expected type name %q to end with %q", typeName, suffix)
	}

	result := &muxType{
		StructName:         "Versioned" + strings.TrimSuffix(typeName, "Server"),
		InterfaceName:      typeName,
		VersionMetadataKey: svcmux.VersionMetadataKey,
	}

	if iface.Methods == nil {
		return result, nil
	}
	for _, m := range iface.Methods.List {
		signature, ok := m.Type.(*ast.FuncType)
		if !ok {
			ifaceName, err := g.exprString(m.Type)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%s embeds %s; not supported", typeName, ifaceName)
		}

		name := m.Names[0].Name
		if signature.Params == nil {
			logging.Warningf(c, "%s.%s: no params", typeName, name)
			continue
		}
		params := signature.Params.List
		if len(params) != 2 {
			logging.Warningf(c,
				"%s.%s: param count is %d; expected 2",
				typeName, name, len(params))
			continue
		}
		if signature.Results == nil {
			logging.Warningf(c, "%s.%s: returns nothing", typeName, name)
			continue
		}
		results := signature.Results.List
		if len(results) != 2 {
			logging.Warningf(c,
				"%s.%s: return value count is %d; expected 2",
				typeName, name, len(results))
			continue
		}

		method := &muxMethod{
			Name: m.Names[0].Name,
		}

		var err error
		method.InputType, err = g.exprString(params[1].Type)
		if err != nil {
			return nil, err
		}
		method.OutputType, err = g.exprString(results[0].Type)
		if err != nil {
			return nil, err
		}
		result.Methods = append(result.Methods, method)
	}
	return result, nil
}

func (g *generator) findType(name string) *ast.TypeSpec {
	if g.typeCache == nil {
		g.typeCache = map[string]*ast.TypeSpec{}
	}
	if typeSpec, ok := g.typeCache[name]; ok {
		return typeSpec
	}

	for _, f := range g.files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				g.typeCache[typeSpec.Name.Name] = typeSpec
				if typeSpec.Name.Name == name {
					return typeSpec
				}
			}
		}
	}

	return nil
}

// exprString renders expr to string.
// Goroutine-unsafe.
func (g *generator) exprString(expr ast.Expr) (string, error) {
	g.exprStrBuf.Reset()
	err := printer.Fprint(&g.exprStrBuf, g.fset, expr)
	return g.exprStrBuf.String(), err
}
