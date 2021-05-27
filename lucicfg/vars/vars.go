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

	"go.chromium.org/luci/common/data/stringset"
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
	// DeclaredExposeAsAliases is a set of all `exposeAs` passed to Declare.
	DeclaredExposeAsAliases stringset.Set

	next   ID       // index of the next variable to produce
	scopes []*scope // stack of scopes, the "active" is last
	bottom scope    // fixed bottom of the stack, with preset var values (see Declare)
}

// scope holds values that were set within the currently exec-ing module.
type scope struct {
	values map[ID]varValue  // var -> (value, trace)
	thread *starlark.Thread // just for asserts
}

// assign mutates 'values' map, auto-initializing it if necessary.
//
// Finish populating 'val' and returns it.
func (s *scope) assign(th *starlark.Thread, id ID, val varValue) (varValue, error) {
	// Forbid sneaky cross-scope communication through in-place mutations of the
	// variable's value.
	val.value.Freeze()

	// Remember where assignment happened, for the error messages in Set().
	var err error
	if val.trace, err = builtins.CaptureStacktrace(th, 1); err != nil {
		return varValue{}, nil
	}

	// Remember the value.
	if s.values == nil {
		s.values = make(map[ID]varValue, 1)
	}
	s.values[id] = val
	return val, nil
}

// varValue is a var value plus a stacktrace where it was set, for errors.
type varValue struct {
	value    starlark.Value               // the frozen value set in the scope
	trace    *builtins.CapturedStacktrace // where it was set
	exposeAs string                       // exposeAs as passed to Declare(...)
	auto     bool                         // true if the value was auto-initialized in get()
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
	v.DeclaredExposeAsAliases = nil
	for _, s := range v.scopes {
		s.values = nil
	}
	v.bottom.values = nil
}

// Declare allocates a new variable, setting it right away to 'presetVal' value
// if 'exposeAs' not empty.
//
// Variables that were pre-set during declaration are essentially immutable
// throughout lifetime of the interpreter.
//
// Returns an error if a variable with the same `exposeAs` name has already been
// exposed.
func (v *Vars) Declare(th *starlark.Thread, exposeAs string, presetVal starlark.Value) (ID, error) {
	// Pick a slot for the variable.
	id := v.next
	v.next++

	if exposeAs != "" {
		// Put a preset value of the var at the bottom of the scope stack, to
		// indicate it was set "before" Starlark execution. v.lookup() will find it
		// there, thus forbidding reassignments.
		_, err := v.bottom.assign(th, id, varValue{value: presetVal, exposeAs: exposeAs})
		if err != nil {
			return 0, err
		}

		// Remember that we exposed a var under this 'exposeAs' alias. This is
		// eventually used to verify all passed '-var' flag were "consumed".
		if v.DeclaredExposeAsAliases == nil {
			v.DeclaredExposeAsAliases = stringset.New(1)
		}
		v.DeclaredExposeAsAliases.Add(exposeAs)
	}

	return id, nil
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
		switch {
		case cur.auto:
			return fmt.Errorf("variable reassignment is forbidden, the previous value was auto-set to the default as a side effect of 'get' at: %s", cur.trace)
		case cur.exposeAs != "":
			return fmt.Errorf("the value of the variable is controlled through CLI flag \"-var %s=...\" and can't be changed from Starlark side", cur.exposeAs)
		default:
			return fmt.Errorf("variable reassignment is forbidden, the previous value was set at: %s", cur.trace)
		}
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
	// The stack has a fixed bottom with vars whose values were set right when
	// they were declared (which may happen outside of EXEC context, so we can't
	// use regular Set and v.scopes there).
	if val, ok := v.bottom.values[id]; ok {
		return &val
	}
	return nil
}

// assign sets the variable's value in the innermost scope.
func (v *Vars) assign(th *starlark.Thread, id ID, value starlark.Value, auto bool) error {
	_, err := v.scopes[len(v.scopes)-1].assign(th, id, varValue{value: value, auto: auto})
	return err
}
