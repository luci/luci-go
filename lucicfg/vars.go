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

package lucicfg

import (
	"fmt"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/interpreter"
)

// Key in Starlark TLS for *scope.
const scopeKey = "lucicfg.scope"

// varID is an opaque starlark value that represents a lucifg.var() variable.
type varID int

func (v varID) String() string        { return fmt.Sprintf("var_%d", v) }
func (v varID) Type() string          { return "lucicfg.var" }
func (v varID) Freeze()               {}
func (v varID) Truth() starlark.Bool  { return starlark.True }
func (v varID) Hash() (uint32, error) { return uint32(v), nil }

// scope hold values of variables that were assigned while the scope was active.
type scope struct {
	values map[varID]varValue
}

// varValue is a value assigned to a variable along with the stacktrace where it
// happened, for error messages.
type varValue struct {
	value starlark.Value
	trace *builtins.CapturedStacktrace
}

// scopedVars holds definitions and current state of lucicfg.var() variables.
//
// It also implement PreExec/PostExec interpreter hooks that "open" and "close"
// scopes for variables: all changes made to vars state within a scope are
// discarded when the scope is closed.
//
// Assumes the interpreter is single-threaded, panics otherwise.
type scopedVars struct {
	next   varID    // index of the next variable to produce, see declareVar()
	scopes []*scope // stack of scopes, the most current is last
}

func (v *scopedVars) preExec(th *starlark.Thread, pkg, path string) {
	// Verify Starlark threads aren't reused by 'exec', per Interpreter
	// implementation. If this changes and threads are reused, we'll need to be
	// more careful and preserve the previous value of scopeKey TLS.
	if th.Local(scopeKey) != nil {
		panic("unexpected scopeKey in thread's TLS")
	}

	newScope := &scope{}
	v.scopes = append(v.scopes, newScope)

	// This will be used in postExec to assert 'exec' call stack is indeed a
	// stack, i.e. we aren't switching goroutines between 'exec's and each preExec
	// has matching postExec, in perfectly nested order.
	th.SetLocal(scopeKey, newScope)
}

func (v *scopedVars) postExec(th *starlark.Thread, pkg, path string) {
	// Pop the scope from the stack.
	if len(v.scopes) == 0 {
		panic("unexpected postExec call without matching preExec")
	}
	top := v.scopes[len(v.scopes)-1]
	v.scopes = v.scopes[:len(v.scopes)-1]

	// Verify it's was indeed used by the thread.
	if cur, _ := th.Local(scopeKey).(*scope); cur != top {
		panic("wrong scope in postExec")
	}
}

// clearValues is used in tests that call __native__.clear_state.
//
// It causes scopedVars to forget about values of all vars, but still remember
// the structure of scopes (since it is tied to the interpreter call stack, not
// affected by __native__.clear_state), and definitions of variables (since
// variables are defined in cached loaded modules, they are not reset by
// __native__.clear_state either).
func (v *scopedVars) clearValues() {
	for _, s := range v.scopes {
		s.values = nil
	}
}

// declareVar allocates a new variable.
//
// It returns a unique integer identifier that should be used as a key in
// setVar and getVar.
func (v *scopedVars) declareVar() varID {
	v.next++
	return v.next - 1
}

// checkPreconditions verifies the variable is being used in an allowed context.
//
// Variable can be assigned or read only from threads that perform 'exec' calls,
// NOT when loading library modules via 'load' (the library modules must not
// have side effects or depend on a transient state in vars), or executing
// callbacks (callbacks do not have enough context to safely use vars).
func (v *scopedVars) checkPreconditions(th *starlark.Thread, id varID) error {
	switch {
	case interpreter.GetThreadKind(th) != interpreter.ThreadExecing:
		return fmt.Errorf("only code that is being exec'ed is allowed to set variables")
	case len(v.scopes) == 0:
		return fmt.Errorf("not inside a scope") // shouldn't be possible
	case id >= v.next:
		return fmt.Errorf("unknown variable") // shouldn't be possible
	default:
		return nil
	}
}

// setVar sets the value of a variable within the current scope iff the
// variable was unset before (in the current or any parent scopes).
//
// Freezes the value as a side effect.
//
func (v *scopedVars) setVar(th *starlark.Thread, id varID, value starlark.Value) error {
	if err := v.checkPreconditions(th, id); err != nil {
		return err
	}

	// Must be unset.
	switch cur, err := v.getVar(th, id); {
	case err != nil:
		panic("impossible") // we already called checkPreconditions above
	case cur.value != starlark.None:
		return fmt.Errorf("variable reassignment is forbidden, the previous value is set at: %s", cur.trace)
	}

	// Remember where assignment happened, for error messages when trying to
	// reassign.
	trace, err := builtins.CaptureStacktrace(th, 0)
	if err != nil {
		return err
	}

	value.Freeze()

	// Set in the most nested scope.
	scope := v.scopes[len(v.scopes)-1]
	if scope.values == nil {
		scope.values = make(map[varID]varValue, 1)
	}
	scope.values[id] = varValue{value, trace}
	return nil
}

// getVar returns the value of a variable or None if unset.
//
// Looks it up starting from the most nested scope.
func (v *scopedVars) getVar(th *starlark.Thread, id varID) (varValue, error) {
	if err := v.checkPreconditions(th, id); err != nil {
		return varValue{}, err
	}
	for idx := len(v.scopes) - 1; idx >= 0; idx-- {
		if val, ok := v.scopes[idx].values[id]; ok {
			return val, nil
		}
	}
	return varValue{starlark.None, nil}, nil
}

func init() {
	// var_declare() allocates a new variable.
	declNative("var_declare", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return call.State.vars.declareVar(), nil
	})

	// var_set(var_id, val) sets the value of a variable.
	declNative("var_set", func(call nativeCall) (starlark.Value, error) {
		var id varID
		var val starlark.Value
		if err := call.unpack(2, &id, &val); err != nil {
			return nil, err
		}
		if err := call.State.vars.setVar(call.Thread, id, val); err != nil {
			return nil, err
		}
		return starlark.None, nil
	})

	// var_get(var_id) returns the value of a variable or None if unset.
	declNative("var_get", func(call nativeCall) (starlark.Value, error) {
		var id varID
		if err := call.unpack(1, &id); err != nil {
			return nil, err
		}
		val, err := call.State.vars.getVar(call.Thread, id)
		return val.value, err
	})
}
