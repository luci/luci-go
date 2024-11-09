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

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// service is the set of data extracted from a generated protobuf file (.pb.go)
// for a single service.
type service struct {
	name string

	protoPackageName string

	clientIfaceDecl ast.Decl
	clientIface     *ast.InterfaceType

	registerServerFunc *ast.FuncType
}

func getServices(file *ast.File) ([]*service, error) {
	svcs := map[string]*service{}
	var serviceNames []string
	get := func(name string) *service {
		s := svcs[name]
		if s == nil {
			s = &service{name: name}
			svcs[name] = s
			serviceNames = append(serviceNames, name)
		}
		return s
	}

	for _, decl := range file.Decls {
		switch dt := decl.(type) {
		case *ast.FuncDecl:
			// Identify server types by scanning for Register<NAME>Server functions.
			name := trimPhrase(dt.Name.Name, "Register", "Server")
			if name == "" {
				break
			}
			s := get(name)
			s.registerServerFunc = dt.Type

		case *ast.GenDecl:
			// Look for:
			// 1) The client interface type: type ...Client
			// 2) The service descriptor:
			//    var ... = grpc.ServiceDesc
			for _, spec := range dt.Specs {
				switch st := spec.(type) {
				case *ast.TypeSpec:
					name := trimPhrase(st.Name.Name, "", "Client")
					if name == "" {
						break
					}

					iface, ok := st.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}

					s := get(name)
					s.clientIfaceDecl = decl
					s.clientIface = iface

				case *ast.ValueSpec:
					if len(st.Values) != 1 {
						continue
					}
					compLit, ok := st.Values[0].(*ast.CompositeLit)
					if !ok {
						continue
					}

					// Is the assigned type "grpc.ServiceDesc"?
					tsel, ok := compLit.Type.(*ast.SelectorExpr)
					if !ok || tsel.Sel.Name != "ServiceDesc" {
						continue
					}
					pkg, ok := tsel.X.(*ast.Ident)
					if !ok || pkg.Name != "grpc" {
						continue
					}

					// Get the "ServiceName" struct field and parse it.
					var serviceNameExpr *ast.KeyValueExpr
					for _, e := range compLit.Elts {
						kv, ok := e.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						kident, ok := kv.Key.(*ast.Ident)
						if !ok {
							continue
						}
						if kident.Name == "ServiceName" {
							serviceNameExpr = kv
							break
						}
					}
					if serviceNameExpr == nil {
						return nil, errors.New("could not find ServiceName member")
					}
					// Get string value.
					serviceNameLit, ok := serviceNameExpr.Value.(*ast.BasicLit)
					if !ok || serviceNameLit.Kind != token.STRING {
						return nil, errors.New("ServiceDesc.ServiceName not a string literal")
					}
					value := trimPhrase(serviceNameLit.Value, `"`, `"`)
					if value == "" {
						return nil, errors.New("ServiceDesc.ServiceName is not properly quoted")
					}
					protoPackage, service, err := parseServiceName(value)
					if err != nil {
						return nil, err
					}

					s := get(service)
					s.protoPackageName = protoPackage
				}
			}
			break
		}
	}

	// Export our services as a slice, ordered by when the service was first
	// encountered in the source file.
	// Verify each service is complete.
	services := make([]*service, len(serviceNames))
	for i, k := range serviceNames {
		s := svcs[k]
		if err := s.complete(); err != nil {
			return nil, fmt.Errorf("incomplete service %q: %s", s.name, err)
		}
		services[i] = s
	}
	return services, nil
}

func (s *service) complete() error {
	if s.protoPackageName == "" {
		return errors.New("missing protobuf package name")
	}
	if s.clientIface == nil {
		return errors.New("missing client iface")
	}
	if s.registerServerFunc == nil {
		return errors.New("missing server registration function")
	}
	return nil
}

// trimPhrase removes the specified prefix and suffix strings from the supplied
// v. If either prefix is missing, suffix is missing, or v consists entirely of
// prefix and suffix, the empty string is returned.
func trimPhrase(v, prefix, suffix string) string {
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	v = strings.TrimPrefix(v, prefix)

	if !strings.HasSuffix(v, suffix) {
		return ""
	}
	return strings.TrimSuffix(v, suffix)
}

func parseServiceName(v string) (string, string, error) {
	idx := strings.LastIndex(v, ".")
	if idx <= 0 {
		return "", "", errors.New("malformed service name")
	}
	return v[:idx], v[idx+1:], nil
}
