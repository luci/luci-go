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

// Package symbols defines a data model representing Starlark symbols.
//
// A symbol is a like a variable: it has a name and points to some object
// somewhere. This package allows to load symbols defined in a starlark module,
// following references. For example, if "a = b", then symbol 'a' points to the
// same object as symbol 'b'.
//
// The loader understands how to follow references across module boundaries and
// struct()s.
package symbols

import (
	"fmt"

	"go.chromium.org/luci/lucicfg/docgen/ast"
	"go.chromium.org/luci/lucicfg/docgen/docstring"
)

// Symbol is something defined in a Starlark module.
//
// It has a name and it points to some declaration.
type Symbol interface {
	// Name is a name of this symbol within its parent namespace.
	//
	// E.g. this is just "a", not "parent.a".
	Name() string

	// Def is an AST node where the object this symbol points to was defined.
	//
	// Nil for broken symbols.
	Def() ast.Node

	// Doc is a parsed docstring for this symbol.
	Doc() *docstring.Parsed
}

// symbol is common base for different types of symbols.
//
// Implements Symbol interface for them.
type symbol struct {
	name string
	def  ast.Node
	doc  *docstring.Parsed
}

func (s *symbol) Name() string  { return s.name }
func (s *symbol) Def() ast.Node { return s.def }

func (s *symbol) Doc() *docstring.Parsed {
	if s.doc == nil {
		if s.def != nil {
			s.doc = docstring.Parse(s.def.Doc())
		} else {
			s.doc = &docstring.Parsed{Description: "broken"}
		}
	}
	return s.doc
}

// BrokenSymbol is a symbol that refers to something we can't resolve.
//
// For example, if "b" is undefined in "a = b", then "a" becomes BrokenSymbol.
type BrokenSymbol struct {
	symbol
}

// newBrokenSymbol returns a new broken symbol with the given name.
func newBrokenSymbol(name string) *BrokenSymbol {
	return &BrokenSymbol{
		symbol: symbol{
			name: name,
		},
	}
}

// Term is a symbol that represents some single terminal definition, not a
// struct.
type Term struct {
	symbol
}

// newTerm returns a new Term symbol.
func newTerm(name string, def ast.Node) *Term {
	return &Term{
		symbol: symbol{
			name: name,
			def:  def,
		},
	}
}

// Struct is a symbol that represents a struct (or equivalent) that has more
// symbols inside it.
//
// Basically, a struct symbol is something that supports "." operation to
// "look" inside it.
type Struct struct {
	symbol

	// symbols is a list of symbols inside this struct.
	symbols []Symbol
	// frozen is true if 'symbols' must not be modified anymore.
	frozen bool
}

// newStruct returns a new struct with empty list of symbols inside.
//
// The caller then can populate it via AddSymbol and finalize with Freeze when
// done.
func newStruct(name string, def ast.Node) *Struct {
	return &Struct{
		symbol: symbol{
			name: name,
			def:  def,
		},
	}
}

// addSymbol appends a symbol to the symbols list in the struct.
//
// Note that starlark forbids reassigning variables in the module scope, so we
// don't check that 'sym' wasn't added before.
//
// Panics if the struct is frozen.
func (s *Struct) addSymbol(sym Symbol) {
	if s.frozen {
		panic("frozen")
	}
	s.symbols = append(s.symbols, sym)
}

// freeze makes the struct immutable.
func (s *Struct) freeze() {
	s.frozen = true
}

// Symbols returns all symbols in the struct.
//
// The caller must not modify the returned slice.
func (s *Struct) Symbols() []Symbol {
	return s.symbols
}

// Lookup essentially does "ns.p0.p1.p2" operation.
//
// Returns a broken symbol if this lookup is not possible, e.g. some field path
// element is not a struct or doesn't exist at all.
func Lookup(ns Symbol, path ...string) Symbol {
	cur := ns
	for _, p := range path {
		var next Symbol
		if strct, _ := cur.(*Struct); strct != nil {
			for _, sym := range strct.symbols {
				if sym.Name() == p {
					next = sym
					break
				}
			}
		}
		if next == nil {
			return newBrokenSymbol(p)
		}
		cur = next
	}
	return cur
}

// newAlias handles definitions like "a = <symbol>".
//
// It returns a new symbol of the same type as the RHS and new name ('a'). It
// points to the same definition the symbol on the RHS points to.
func newAlias(name string, symbol Symbol) Symbol {
	switch s := symbol.(type) {
	case *BrokenSymbol:
		return newBrokenSymbol(name)
	case *Term:
		return newTerm(name, s.Def())
	case *Struct:
		// Structs are copied by value too, but we check they are frozen at this
		// point, so it should be fine. It is possible the struct is not frozen yet
		// in the following self-referential case: 'a = {}; a = struct(k = a)'. But
		// it is not allowed by starlark (redefinitions are forbidden). We still
		// cautiously treat this case as broken.
		if !s.frozen {
			return newBrokenSymbol(name)
		}
		strct := newStruct(name, s.Def())
		strct.symbols = s.symbols // it is immutable, copying the pointer is fine
		strct.freeze()
		return strct
	default:
		panic(fmt.Sprintf("unrecognized symbol type %T", s))
	}
}
