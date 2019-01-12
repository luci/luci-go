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

package model

import (
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/lucicfg/docgen/ast"
)

// Loader knows how to load a starlark file and all files it references
// (recursively), and resolve symbols in them to their final definition.
//
// As a result it builds a symbol tree. Intermediate nodes in this tree are
// struct-like definitions (which define namespaces), and leafs hold pointers
// to ast.Nodes with concrete definitions of these symbols (after following
// all possible aliases).
//
// Consider this module.star Starlark code, for example:
//
//     def _func():
//       """Doc string."""
//     exported = struct(func = _func, const = 123)
//
// It will produce the following symbol tree:
//
//    Struct('module.star', *ast.Module, [
//      Term('_func', *ast.Function _func),
//      Struct('exported', *ast.Namespace exported, [
//        Term('func', *ast.Function _func),
//        Term('const', *ast.Var const),
//      ]),
//    ])
//
// Notice that both '_func' and 'exported.func' point to exact same AST node
// where the function was actually defined.
//
// This allows to collection the documentation for all exported symbols even
// if they are gathered from many internal modules via load(...) statements,
// assignments and structs.
type Loader struct {
	// Source loads module's source code.
	Source func(module string) (src string, err error)

	loading stringset.Set      // set of modules being recursively loaded now
	sources map[string]string  // all loaded source code, keyed by module name
	symbols map[string]*Struct // symbols defined in the corresponding module
}

// init lazily initializes loader's guts.
func (l *Loader) init() {
	if l.loading == nil {
		l.loading = stringset.New(1)
		l.sources = make(map[string]string, 1)
		l.symbols = make(map[string]*Struct, 1)
	}
}

// Loads loads the module and all modules it references, populating the
// loader's state with information about exported symbols.
//
// Returns a struct with a list of symbols defined in the module.
//
// Can be called multiple times with different modules.
func (l *Loader) Load(module string) (syms *Struct, err error) {
	defer func() {
		err = errors.Annotate(err, "in %s", module).Err()
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
	mod, err := ast.ParseModule(module, src)
	if err != nil {
		return nil, err
	}

	// Recursively resolve all references in 'mod' to their concrete definitions
	// (perhaps in another modules). This returns a struct with a list of all
	// symbols defined in the module, fully resolved to their definitions.
	var top *Struct
	if top, err = l.resolveRefs(&mod.Namespace, nil); err != nil {
		return nil, err
	}
	l.symbols[module] = top
	return top, nil
}

// resolveRefs visits nodes in the namespace and follows References and
// ExternalReferences to get the final definition of all symbols.
//
// If puts them in a struct and returns it.
//
// 'top' struct represents the top module scope and it is used to lookup
// referenced symbols. It is nil when resolveRefs is used to resolve the module
// scope itself.
//
// When resolving symbols in a struct(k=v, ...), 'top' contains symbols from the
// top-level module scope. There's NO chaining of scopes, because the following
// is NOT a valid definition:
//
//    struct(
//        k1 = v,
//        nested = struct(k2 = k1),  # k1 is undefined!
//    )
//
// Only symbols defined at the module scope (e.g. variables) can be referenced
// from inside struct definitions.
func (l *Loader) resolveRefs(ns *ast.Namespace, top *Struct) (*Struct, error) {
	cur := NewStruct(ns.Name(), ns)
	defer cur.Freeze()

	// When parsing the module scope, 'cur' IS the top-level scope. All symbols
	// defined in 'cur' become immediately visible to all later definitions.
	if top == nil {
		top = cur
	}

	for _, n := range ns.Nodes {
		switch val := n.(type) {
		case *ast.Reference:
			// A reference to a symbol defined elsewhere. Follow it.
			pointsTo := Lookup(top, val.Path[0])
			for i := 1; i < len(val.Path) && !IsBroken(pointsTo); i++ {
				pointsTo = Lookup(pointsTo, val.Path[i])
			}
			cur.AddSymbol(NewAlias(val.Name(), pointsTo))

		case *ast.ExternalReference:
			// A reference to a symbol in another module. Load the module and follow
			// the reference.
			external, err := l.Load(val.Module)
			if err != nil {
				return nil, err
			}
			cur.AddSymbol(NewAlias(val.Name(), Lookup(external, val.ExternalName)))

		case *ast.Namespace:
			// A struct(...) definition. Recursively resolve what's inside it. Allow
			// it to reference the symbols in the top scope only. When one struct
			// nests another, the inner struct doesn't have access to symbols defined
			// in an outer struct. Only what's in the top-level scope.
			inner, err := l.resolveRefs(val, top)
			if err != nil {
				return nil, err
			}
			cur.AddSymbol(inner)

		default:
			// Something defined right in this namespace.
			cur.AddSymbol(NewTerm(n.Name(), n))
		}
	}

	return cur, nil
}
