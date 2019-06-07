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
	"regexp"
	"strings"
	"text/template"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/lucicfg/docgen/ast"
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

	loader *symbols.Loader    // knows how to load symbols from starlark modules
	links  map[string]*symbol // full name -> symbol we can link to
}

// Render renders the given text template in an environment with access to
// parsed structured Starlark comments.
func (g *Generator) Render(templ string) ([]byte, error) {
	if g.loader == nil {
		g.loader = &symbols.Loader{Source: g.Starlark}
		g.links = map[string]*symbol{}
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
		"EscapeMD":       escapeMD,
		"Symbol":         g.symbol,
		"LinkifySymbols": g.linkifySymbols,
	}
}

// escapeMD makes sure 's' gets rendered as is in markdown.
func escapeMD(s string) string {
	return strings.Replace(s, "*", "\\*", -1)
}

func (g *Generator) load(module string) (*symbols.Struct, error) {
	mod, err := g.loader.Load(module)
	if err != nil {
		return nil, err
	}

	// Transform lucicfg.rule(...) definitions to pick up docstrings and arguments
	// of the rule implementation. We replace `var = lucicfg.rule(impl = f)` with
	// `var = f`.
	return mod.Transform(func(s symbols.Symbol) (symbols.Symbol, error) {
		inv, ok := s.(*symbols.Invocation)
		if !ok {
			return s, nil
		}
		// Rule constructor symbols are marked with RuleCtor tag.
		targetTags := inv.Func().Doc().RemarkBlock("DocTags").Body
		if !strings.Contains(targetTags, "RuleCtor") {
			return s, nil
		}
		// Find a symbol assigned to 'impl' kwarg and return it, so that it is
		// used instead of lucicfg.rule(...) invocation. Give it the name of 's'.
		for _, arg := range inv.Args() {
			if arg.Name() == "impl" {
				return symbols.NewAlias(s.Name(), arg), nil
			}
		}
		return nil, fmt.Errorf("cannot resolve rule constructor call in %s, no `impl` kwarg", s)
	})
}

// symbol returns a symbol from the given module.
//
// lookup is a field path, e.g. "a.b.c". "a" will be searched for in the
// top-level dict of the module. If empty, the module itself will be returned.
//
// If the requested symbol can't be found, returns a broken symbol.
func (g *Generator) symbol(module, lookup string) (*symbol, error) {
	mod, err := g.load(module)
	if err != nil {
		return nil, err
	}

	var lookupPath []string
	if lookup != "" {
		lookupPath = strings.Split(lookup, ".")
	}

	sym := &symbol{
		Symbol:   symbols.Lookup(mod, lookupPath...),
		Module:   module,
		FullName: lookup,
	}

	// Let automatic linkifier know about the loaded symbols so it can start
	// generating links to them if it encounters them in the text.
	syms, _ := sym.Symbols()
	for _, s := range syms {
		g.links[s.FullName] = s
	}

	return sym, nil
}

// This matches a.b.c(...). '(...)' part is important, otherwise there is a ton
// of undesired matches in various code snippets.
var symRefRe = regexp.MustCompile(`\w+(\.\w+)*(\(\.\.\.\))`)

// linkifySymbols replaces recognized symbol names with markdown links to
// symbols.
func (g *Generator) linkifySymbols(text string) string {
	return symRefRe.ReplaceAllStringFunc(text, func(match string) string {
		if sym := g.links[strings.TrimSuffix(match, "(...)")]; sym != nil {
			return fmt.Sprintf("[%s](#%s)", match, sym.Anchor())
		}
		return match
	})
}

////////////////////////////////////////////////////////////////////////////////

// symbol is what we expose to the templates.
//
// It is mostly symbols.Symbol, except we add few useful utility fields and
// methods.
type symbol struct {
	symbols.Symbol

	Module   string // module name used to load this symbol
	FullName string // field path from module's top dict to this symbol

	tags stringset.Set // lazily extracted from "DocTags" remarks section
}

// Flavor returns one of "func", "var", "struct", "unknown".
func (s *symbol) Flavor() string {
	switch s.Symbol.(type) {
	case *symbols.Term:
		switch s.Symbol.Def().(type) {
		case *ast.Function:
			return "func"
		case *ast.Var:
			return "var"
		default:
			return "unknown"
		}
	case *symbols.Invocation:
		return "inv"
	case *symbols.Struct:
		return "struct"
	default:
		return "unknown"
	}
}

// HasDocTag returns true if the docstring has a section "DocTags" and the
// given tag is listed there.
//
// Used to mark some symbols as advanced, or experimental.
func (s *symbol) HasDocTag(tag string) bool {
	if s.tags == nil {
		s.tags = stringset.Set{}
		for _, word := range strings.Fields(s.Doc().RemarkBlock("DocTags").Body) {
			s.tags.Add(strings.ToLower(strings.Trim(word, ".,")))
		}
	}
	return s.tags.Has(strings.ToLower(tag))
}

// Symbols returns nested symbols.
//
// If `flavors` is not empty, it specifies what kinds of symbols to keep.
// Possible variants: "func", "var", "inv", "struct".
func (s *symbol) Symbols(flavors ...string) (out []*symbol, err error) {
	strct, _ := s.Symbol.(*symbols.Struct)
	if strct == nil {
		return nil, fmt.Errorf("%q is not a struct", s.FullName)
	}

	keepFlavors := stringset.NewFromSlice(flavors...)

	for _, sym := range strct.Symbols() {
		fullName := ""
		if s.FullName != "" {
			fullName = s.FullName + "." + sym.Name()
		} else {
			fullName = sym.Name()
		}

		sym := &symbol{
			Symbol:   sym,
			Module:   s.Module,
			FullName: fullName,
		}

		if keepFlavors.Len() == 0 || keepFlavors.Has(sym.Flavor()) {
			out = append(out, sym)
		}
	}

	return
}

// Anchor returns a markdown anchor name that can be used to link to some part
// of this symbol's documentation from other parts of the doc.
func (s *symbol) Anchor(sub ...string) string {
	return strings.Join(append([]string{s.FullName}, sub...), "-")
}

// InvocationSnippet returns a snippet showing how a function represented by
// this symbol can be called.
//
// Like this:
//
//     luci.recipe(
//         # Required arguments.
//         name,
//         cipd_package,
//
//         # Optional arguments.
//         cipd_version = None,
//         recipe = None,
//
//         **kwargs,
//     )
//
// This is apparently very non-trivial to generate using text/template while
// keeping all spaces and newlines strict.
func (s *symbol) InvocationSnippet() string {
	var req, opt, variadric []string
	for _, f := range s.Doc().Args() {
		switch {
		case strings.HasPrefix(f.Name, "*"):
			variadric = append(variadric, f.Name)
		case isRequiredField(f):
			req = append(req, f.Name)
		default:
			opt = append(opt, fmt.Sprintf("%s = None", f.Name))
		}
	}

	b := &strings.Builder{}

	writeSection := func(section string, args []string) {
		if len(args) != 0 {
			b.WriteString(section)
			for _, a := range args {
				fmt.Fprintf(b, "    %s,\n", a)
			}
		}
	}

	fmt.Fprintf(b, "%s(", s.FullName)
	if all := append(append(req, opt...), variadric...); len(all) <= 3 {
		// Use a compact form when we have only very few arguments.
		b.WriteString(strings.Join(all, ", "))
	} else {
		writeSection("\n    # Required arguments.\n", req)
		writeSection("\n    # Optional arguments.\n", opt)
		writeSection("\n", variadric)
	}
	fmt.Fprintf(b, ")")
	return b.String()
}

// isRequiredField takes a field description and tries to figure out whether
// this field is required.
//
// Does this by searching for "Required." suffix. Very robust.
func isRequiredField(f docstring.Field) bool {
	return strings.HasSuffix(f.Desc, "Required.")
}
