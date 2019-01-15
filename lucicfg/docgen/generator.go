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
	"fmt"
	"strings"
	"text/template"

	"go.chromium.org/luci/lucicfg/docgen/docstring"
	"go.chromium.org/luci/lucicfg/docgen/symbols"
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

	loader *symbols.Loader // knows how to load symbols from starlark modules
}

// Render renders the given text template in an environment with access to
// parsed structured Starlark comments.
func (g *Generator) Render(templ string) ([]byte, error) {
	if g.loader == nil {
		g.loader = &symbols.Loader{Source: g.Starlark}
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
		"symbol":            g.symbol,
		"invocationSnippet": invocationSnippet,
		"isRequired":        isRequired,
	}
}

// symbol returns symbols.Symbol from the given module.
//
// lookup is a field path, e.g. "a.b.c". "a" will be searched for in the
// top-level dict of the module. If empty, the module itself will be returned.
//
// If the requested symbol can't be found, returns a broken symbol.
func (g *Generator) symbol(module, lookup string) (symbols.Symbol, error) {
	mod, err := g.loader.Load(module)
	if err != nil {
		return nil, err
	}
	return symbols.Lookup(mod, lookup), nil
}

// invocationSnippet takes a name and a symbol representing a function and
// returns a short snippet showing how this function can be called using the
// given name.
//
// Like this:
//
//     core.recipe(
//         # Required arguments.
//         name,
//         cipd_package,
//
//         # Optional arguments.
//         cipd_version = None,
//         recipe = None,
//     )
//
// This is apparently very non-trivial to generate using text/template while
// keeping all spaces and newlines strict.
func invocationSnippet(name string, f symbols.Symbol) string {
	var req, opt []string
	for _, f := range f.Doc().Args() {
		if isRequired(f) {
			req = append(req, f.Name)
		} else {
			opt = append(opt, f.Name)
		}
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "%s(", name)
	if len(req) != 0 {
		fmt.Fprintf(b, "\n    # Required arguments.\n")
		for _, n := range req {
			fmt.Fprintf(b, "    %s,\n", n)
		}
	}
	if len(opt) != 0 {
		fmt.Fprintf(b, "\n    # Optional arguments.\n")
		for _, n := range opt {
			fmt.Fprintf(b, "    %s = None,\n", n)
		}
	}
	fmt.Fprintf(b, ")")
	return b.String()
}

// isRequired takes a field description and tries to figure out whether this
// field is required.
//
// Does this by searching for "Required." suffix. Very robust.
func isRequired(f docstring.Field) bool {
	return strings.HasSuffix(f.Desc, "Required.")
}
