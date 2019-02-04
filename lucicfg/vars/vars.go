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

// Package vars implement lucicfg.var() support.
//
// See lucicfg.var() doc for more info.
package vars

import (
	"fmt"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/interpreter"
)

// Note: this package is tested entirely by Starlark tests, in particular
// testdata/misc/vars.star. They are not generating code coverage results
// though, being in a different package.

// ID is an opaque starlark value that represents a lucifg.var() variable.
//
// It is just a unique identifier for this particular variable, used as a key
// in a map with variables values.
type ID int

func (v ID) String() string        { return fmt.Sprintf("var#%d", v) }
func (v ID) Type() string          { return "lucicfg.var" }
func (v ID) Freeze()               {}
func (v ID) Truth() starlark.Bool  { return starlark.True }
func (v ID) Hash() (uint32, error) { return uint32(v), nil }

// Vars holds definitions and current state of lucicfg.var() variables.
//
// It exposes OpenScope/CloseScope that should be connected to PreExec/PostExec
// interpreter hooks. They "open" and "close" scopes for variables: all changes
// made to vars state within a scope (i.e. within single exec-ed module) are
// discarded when the scope is closed.
type Vars struct {
	next   ID       // index of the next variable to produce
	scopes []*scope // stack of scopes, the "active" is last
}

// scope holds values that were set within the currently exec-ing module.
type scope struct {
	values map[ID]varValue  // var -> (value, trace)
	thread *starlark.Thread // just for asserts
}

// varValue is a var value plus a stacktrace where it was set, for errors.
type varValue struct {
	value starlark.Value
	trace *builtins.CapturedStacktrace
	auto  bool // true if the value was auto-initialized in get()
}

// OpenScope opens a new scope for variables.
//
// All changes to variables' values made within this scope are discarded when it
// is closed.
func (v *Vars) OpenScope(th *starlark.Thread) {
	v.scopes = append(v.scopes, &scope{thread: th})
}

// CloseScope discards changes made to variables since the matching OpenScope.
func (v *Vars) CloseScope(th *starlark.Thread) {
	switch {
	case len(v.scopes) == 0:
		panic("unexpected CloseScope call without matching OpenScope")
	case v.scopes[len(v.scopes)-1].thread != th:
		// This check may fail if Interpreter is using multiple goroutines to
		// execute 'exec's. It shouldn't. If this ever happens, we'll need to move
		// 'v.scopes' to Starlark's TLS. OpenScope will need a way to access TLS
		// of a thread that called 'exec', not only TLS of a new thread, as it does
		// now.
		panic("unexpected starlark thread")
	default:
		v.scopes = v.scopes[:len(v.scopes)-1]
	}
}

// ClearValues resets values of all variables, in all scopes.
//
// Should be used only from tests that want a clean slate.
func (v *Vars) ClearValues() {
	for _, s := range v.scopes {
		s.values = nil
	}
}

// Declare allocates a new variable.
func (v *Vars) Declare() ID {
	v.next++
	return v.next - 1
}

// Set sets the value of a variable within the current scope iff the variable
// was unset before (in the current or any parent scopes).
//
// Variable can be assigned or read only from threads that perform 'exec' calls,
// NOT when loading library modules via 'load' (the library modules must not
// have side effects or depend on a transient state in vars), or executing
// callbacks (callbacks do not have enough context to safely use vars).
//
// Freezes the value as a side effect.
func (v *Vars) Set(th *starlark.Thread, id ID, value starlark.Value) error {
	if err := v.checkVarsAccess(th, id); err != nil {
		return err
	}

	// Must be unset.
	if cur := v.lookup(id); cur != nil {
		if cur.auto {
			return fmt.Errorf("variable reassignment is forbidden, the previous value was auto-set to the default as a side effect of 'get' at: %s", cur.trace)
		}
		return fmt.Errorf("variable reassignment is forbidden, the previous value was set at: %s", cur.trace)
	}

	return v.assign(th, id, value, false)
}

// Get returns the value of a variable, auto-initializing it to 'def' if
// it was unset before.
//
// Variable can be assigned or read only from threads that perform 'exec' calls,
// NOT when loading library modules via 'load' (the library modules must not
// have side effects or depend on a transient state in vars), or executing
// callbacks (callbacks do not have enough context to safely use vars).
func (v *Vars) Get(th *starlark.Thread, id ID, def starlark.Value) (starlark.Value, error) {
	if err := v.checkVarsAccess(th, id); err != nil {
		return nil, err
	}

	// Return if set. Otherwise auto-set to the default and return the default.
	if cur := v.lookup(id); cur != nil {
		return cur.value, nil
	}
	if err := v.assign(th, id, def, true); err != nil {
		return nil, err
	}
	return def, nil
}

// checkVarsAccess verifies the variable is being used in an allowed context.
func (v *Vars) checkVarsAccess(th *starlark.Thread, id ID) error {
	switch {
	case interpreter.GetThreadKind(th) != interpreter.ThreadExecing:
		return fmt.Errorf("only code that is being exec'ed is allowed to get or set variables")
	case len(v.scopes) == 0:
		return fmt.Errorf("not inside an exec scope") // shouldn't be possible
	case id >= v.next:
		return fmt.Errorf("unknown variable") // shouldn't be possible
	default:
		return nil
	}
}

// lookup finds the variable's value in the current scope or any of parent
// scopes.
func (v *Vars) lookup(id ID) *varValue {
	for idx := len(v.scopes) - 1; idx >= 0; idx-- {
		if val, ok := v.scopes[idx].values[id]; ok {
			return &val
		}
	}
	return nil
}

// assign sets the variable's value in the innermost scope.
func (v *Vars) assign(th *starlark.Thread, id ID, value starlark.Value, auto bool) error {
	// Forbid sneaky cross-scope communication through in-place mutations of the
	// variable's value.
	value.Freeze()

	// Remember where assignment happened, for the error message in Set().
	trace, err := builtins.CaptureStacktrace(th, 0)
	if err != nil {
		return err
	}

	// Set in the innermost scope.
	scope := v.scopes[len(v.scopes)-1]
	if scope.values == nil {
		scope.values = make(map[ID]varValue, 1)
	}
	scope.values[id] = varValue{value, trace, auto}
	return nil
}
