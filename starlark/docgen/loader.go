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

// Package docgen is an opinionated documentation generator for Starlark code.
package docgen

import (
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

// Loader knows how to load a bunch of starlark files and resolve symbols in
// them.
type Loader struct {
	// Source knows how to load module's source code.
	Source func(module string) (src string, err error)

	loading stringset.Set      // set of modules being recursively explored now
	sources map[string]string  // all loaded source code, keyed by module name
	symbols map[string]Symbols // symbols defined in a module
}

// init lazily initializes loader's guts.
func (l *Loader) init() {
	if l.loading == nil {
		l.loading = stringset.New(1)
		l.sources = make(map[string]string, 1)
		l.symbols = make(map[string]Symbols, 1)
	}
}

// Loads loads the module and all modules it references, populating the
// loader's state with information about exported symbols.
//
// Returns a list of symbols defined in the module.
func (l *Loader) Load(module string) (syms Symbols, err error) {
	defer func() {
		err = errors.Annotate(err, "when loading %s", module).Err()
	}()

	l.init()
	if !l.loading.Add(module) {
		return nil, errors.New("recursive dependency")
	}
	defer l.loading.Del(module)

	// Already processed it?
	if syms, ok := l.symbols[module]; ok {
		return syms, nil
	}

	// Load and parse the source code into a distilled AST.
	src, err := l.Source(module)
	if err != nil {
		return nil, err
	}
	l.sources[module] = src
	mod, err := ParseModule(module, src)
	if err != nil {
		return nil, err
	}

	// Recursively resolve all references in 'mod' to their concrete definitions
	// (perhaps in another modules). The produces a list of symbols defined in
	// the module.
	if syms, err = l.resolveNamespaceRefs(&mod.Namespace, nil); err != nil {
		return nil, err
	}
	l.symbols[module] = syms
	return syms, nil
}

func (l *Loader) resolveNamespaceRefs(ns *Namespace, toplevel *Symbols) (syms Symbols, err error) {
	// 'toplevel' is nil when resolving the module-level namespace. It IS the top
	// level scope. It will be inherited by all nested namespaces, so definitions
	// there are allowed to reference variable defined at the module-scope.
	if toplevel == nil {
		toplevel = &syms
	}

	for _, n := range ns.Nodes {
		switch val := n.(type) {
		case *Reference:
			// A reference to a symbol defined elsewhere. Follow it. Now that trying
			// to lookup  a field in a non-namespace symbol results in a broken
			// symbol, which aborts the cycle.
			pointsTo := toplevel.Lookup(val.Path[0])
			for i := 1; i < len(val.Path) && !pointsTo.Broken; i++ {
				pointsTo = pointsTo.Nested.Lookup(val.Path[i])
			}
			syms = append(syms, (&Symbol{
				Name: val.Name(),
				Decl: val,
			}).setFrom(pointsTo))

		case *ExternalReference:
			// A reference to a symbol in another module. Load the module and follow.
			external, err := l.Load(val.Module)
			if err != nil {
				return nil, err
			}
			syms = append(syms, (&Symbol{
				Name: val.Name(),
				Decl: val,
			}).setFrom(external.Lookup(val.ExternalName)))

		case *Namespace:
			// A struct(...) definition. Recursively resolve what's inside it. Allow
			// it to reference the symbols in the top scope only. When one struct
			// nests another, the inner struct doesn't have access to symbols defined
			// in an outer struct. Only what's in the top-level scope.
			namespace, err := l.resolveNamespaceRefs(val, toplevel)
			if err != nil {
				return nil, err
			}
			syms = append(syms, &Symbol{
				Name:   val.Name(),
				Decl:   val,
				Nested: namespace,
			})

		default:
			// Something (a term like a function) defined right in this namespace.
			syms = append(syms, &Symbol{
				Name: n.Name(),
				Decl: n,
				Term: n,
			})
		}
	}

	return
}
