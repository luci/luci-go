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
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ruleImpl is a callable representing some concrete rule.
//
// It is a result of `lucicfg.rule(impl = cb)` call. It wraps `cb`, adding
// an additional positional argument to it (in front of other arguments).
//
// TODO(vadimsh): Add attributes too, such as 'defaults' and 'key'.
type ruleImpl struct {
	starlark.Callable
}

// String lets caller know this is a rule now.
func (r *ruleImpl) String() string {
	name := r.Callable.Name()
	return fmt.Sprintf("<rule %s>", strings.TrimPrefix(name, "_"))
}

// CallInternal prepends `ctx` argument and validates the return value.
//
// Note that we intentionally do not start a new stack frame to declutter stack
// traces: lucicfg.rule(...) wrapping is "transparent" from Starlark's point of
// view.
func (r *ruleImpl) CallInternal(th *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	newArgs := make(starlark.Tuple, 0, len(args)+1)
	newArgs = append(newArgs, starlarkstruct.FromStringDict(
		starlark.String("lucicfg.rule_ctx"),
		starlark.StringDict{
			// TODO(vadimsh): Add fields.
		},
	))
	newArgs = append(newArgs, args...)
	switch res, err := r.Callable.CallInternal(th, newArgs, kwargs); {
	case err != nil:
		return nil, err
	case !isKeyset(res):
		return nil, fmt.Errorf("bad rule implementation %s: must return graph.keyset, got %s", r.Callable, res.Type())
	default:
		return res, nil
	}
}

func isKeyset(v starlark.Value) bool {
	if st, ok := v.(*starlarkstruct.Struct); ok {
		return st.Constructor().String() == "graph.keyset"
	}
	return false
}

func init() {
	// See //internal/lucicfg.star.
	declNative("declare_rule", func(call nativeCall) (starlark.Value, error) {
		var impl starlark.Callable
		if err := call.unpack(1, &impl); err != nil {
			return nil, err
		}
		return &ruleImpl{impl}, nil
	})
}
