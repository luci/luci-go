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
	"fmt"

	"go.chromium.org/luci/lucicfg/docgen/ast"
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
}

// symbol is common base for different types of symbols.
//
// Implements Symbol interface for them.
type symbol struct {
	name string
	def  ast.Node
}

func (s *symbol) Name() string  { return s.name }
func (s *symbol) Def() ast.Node { return s.def }

// BrokenSymbol is a symbol that refers to something we can't resolve.
//
// For example, if "b" is undefined in "a = b", then "a" becomes BrokenSymbol.
type BrokenSymbol struct {
	symbol
}

// NewBrokenSymbol returns a new broken symbol with the given name.
func NewBrokenSymbol(name string) *BrokenSymbol {
	return &BrokenSymbol{
		symbol: symbol{
			name: name,
		},
	}
}

// IsBroken returns true if sym is *BrokenSymbol.
func IsBroken(sym Symbol) bool {
	_, yes := sym.(*BrokenSymbol)
	return yes
}

// Term is a symbol that represents some single terminal definition, not a
// struct.
type Term struct {
	symbol
}

// NewTerm returns a new Term symbol.
func NewTerm(name string, def ast.Node) *Term {
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

// NewStruct returns a new struct with empty list of symbols inside.
//
// The caller then can populate it via AddSymbol and finalize with Freeze when
// done.
func NewStruct(name string, def ast.Node) *Struct {
	return &Struct{
		symbol: symbol{
			name: name,
			def:  def,
		},
	}
}

// AddSymbol appends a symbol to the symbols list in the struct.
//
// Note that starlark forbids reassigning variables in the module scope, so we
// don't check that 'sym' wasn't added before.
//
// Panics if the struct is frozen.
func (s *Struct) AddSymbol(sym Symbol) {
	if s.frozen {
		panic("frozen")
	}
	s.symbols = append(s.symbols, sym)
}

// Freeze makes the struct immutable.
func (s *Struct) Freeze() {
	s.frozen = true
}

// Lookup essentially does "ns.<name>" operation.
//
// Returns a broken symbol if 'ns' doesn't represent a struct or there's no
// symbol named 'name' inside.
func Lookup(ns Symbol, name string) Symbol {
	if strct, _ := ns.(*Struct); strct != nil {
		for _, sym := range strct.symbols {
			if sym.Name() == name {
				return sym
			}
		}
	}
	return NewBrokenSymbol(name)
}

// NewAlias handles definitions like "a = <symbol>".
//
// It returns a new symbol of the same type as the RHS and new name ('a'). It
// points to the same definition the symbol on the RHS points to.
func NewAlias(name string, symbol Symbol) Symbol {
	switch s := symbol.(type) {
	case *BrokenSymbol:
		return NewBrokenSymbol(name)
	case *Term:
		return NewTerm(name, s.Def())
	case *Struct:
		// Structs are copied by value too, but we check they are frozen at this
		// point, so it should be fine. It is possible the struct is not frozen yet
		// in the following self-referential case: 'a = {}; a = struct(k = a)'. But
		// it is not allowed by starlark (redefinitions are forbidden). We still
		// cautiously treat this case as broken.
		if !s.frozen {
			return NewBrokenSymbol(name)
		}
		strct := NewStruct(name, s.Def())
		strct.symbols = s.symbols // it is immutable, copying the pointer is fine
		strct.Freeze()
		return strct
	default:
		panic(fmt.Sprintf("unrecognized symbol type %T", s))
	}
}
