// Copyright 2019 The LUCI Authors.
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

// Package docgen generates documentation from Starlark code.
package docgen

import (
	"bytes"
	"text/template"

	"go.chromium.org/luci/lucicfg/docgen/model"
)

// Generator renders text templates that have access to parsed structured
// representation of Starlark modules.
//
// The templates use them to inject documentation extracted from Starlark into
// appropriate places.
type Generator struct {
	// Starlark produces Starlark module's source code.
	//
	// It is then parsed by the generator to extract documentation from it.
	Starlark func(module string) (src string, err error)

	loader *model.Loader // knows how to load starlark modules
}

// Render renders the given text template in an environment with access to
// parsed structured Starlark comments.
func (g *Generator) Render(templ string) ([]byte, error) {
	if g.loader == nil {
		g.loader = &model.Loader{Source: g.Starlark}
	}

	t, err := template.New("main").Funcs(g.funcMap()).Parse(templ)
	if err != nil {
		return nil, err
	}

	buf := bytes.Buffer{}
	if err := t.Execute(&buf, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// funcMap are functions available to templates.
func (g *Generator) funcMap() template.FuncMap {
	return template.FuncMap{
		"symbol": g.symbol,
	}
}

// symbol returns model.Symbol from the given module.
//
// lookup is a field path, e.g. "a.b.c". "a" will be searched for in the
// top-level dict of the module. If empty, the module itself will be returned.
//
// If the requested symbol can't be found, returns a broken symbol.
func (g *Generator) symbol(module, lookup string) (model.Symbol, error) {
	mod, err := g.loader.Load(module)
	if err != nil {
		return nil, err
	}
	return model.Lookup(mod, lookup), nil
}
