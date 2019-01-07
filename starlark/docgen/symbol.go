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

package docgen

import (
	"fmt"
)

// Symbol is a referable symbol exported by a Starlark module.
//
// A symbol is either a real declaration (like "def func"), or an alias for
// the real declaration (like "exported = _impl.private").
type Symbol struct {
	Name string // name of the symbol or the alias
	Decl Node   // node with the symbol or the alias declaration

	// Only one of the below is set.

	Term   Node    // node with the actual implementation
	Nested Symbols // nested symbols for namespace-like symbols, e.g. structs
	Broken bool    // true if the symbol refers to something we can't find
}

// String is used to debug-print symbols.
func (s *Symbol) String() string {
	decl := ""
	if s.Decl != nil {
		start, _ := s.Decl.Span()
		decl = fmt.Sprintf(" at %s", start)
	}

	val := ""
	switch {
	case s.Term != nil:
		start, _ := s.Term.Span()
		val = fmt.Sprintf("%s at %s", s.Term.Name(), start)

	case s.Nested != nil:
		val = "{...}"

	case s.Broken:
		val = "???"
	}

	return fmt.Sprintf("%s%s = %s", s.Name, decl, val)
}

// setFrom is used when a symbol is a reference to another symbol.
func (s *Symbol) setFrom(another *Symbol) *Symbol {
	s.Term = another.Term
	s.Nested = another.Nested
	s.Broken = another.Broken
	return s
}

// Symbols is a list of symbols.
type Symbols []*Symbol

// Lookup finds a symbol with given name or returns a broken symbol.
func (s Symbols) Lookup(name string) *Symbol {
	for _, sym := range s {
		if sym.Name == name {
			return sym
		}
	}
	return &Symbol{Name: name, Broken: true}
}
