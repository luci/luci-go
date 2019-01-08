// Copyright 2018 The LUCI Authors.
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

// Functions for working with config sets and generator callbacks that modify
// them.

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/starlarkproto"
)

// configSet is a map-like value that has file names as keys and strings or
// protobuf messages as values.
//
// At the end of the execution all protos are serialized to strings too, using
// textpb encoding.
type configSet struct {
	starlark.Dict
}

func newConfigSet() *configSet {
	return &configSet{}
}

func (cs *configSet) Type() string { return "config_set" }

func (cs *configSet) SetKey(k, v starlark.Value) error {
	if _, ok := k.(starlark.String); !ok {
		return fmt.Errorf("config set key should be a string, not %s", k.Type())
	}

	_, str := v.(starlark.String)
	_, msg := v.(*starlarkproto.Message)
	if !str && !msg {
		return fmt.Errorf("config set value should be either a string or a proto message, not %s", v.Type())
	}

	return cs.Dict.SetKey(k, v)
}

// asTextProto returns a config set with all protos serialized to text proto
// format (with added header).
//
// Non-proto configs are returned as is.
func (cs *configSet) asTextProto(header string) (map[string]string, error) {
	out := make(map[string]string, cs.Len())

	for _, kv := range cs.Items() {
		k, v := kv[0].(starlark.String), kv[1]

		text := ""
		if s, ok := v.(starlark.String); ok {
			text = s.GoString()
		} else {
			msg, err := v.(*starlarkproto.Message).ToProto()
			if err != nil {
				return nil, err
			}
			text = header + proto.MarshalTextString(msg)
		}

		out[k.GoString()] = text
	}

	return out, nil
}

// generators is a list of registered generator callbacks.
//
// It lives in State. Generators are executed sequentially after all Starlark
// code is loaded. They examine the state and generate configs based on it.
type generators struct {
	gen    []starlark.Callable
	frozen bool // true while iterating over the list
}

// add registers a new generator callback.
func (g *generators) add(cb starlark.Callable) error {
	if g.frozen {
		return fmt.Errorf("generators list is frozen during iteration")
	}
	g.gen = append(g.gen, cb)
	return nil
}

// call calls all registered callbacks sequentially, collecting all errors.
func (g *generators) call(th *starlark.Thread, ctx *genCtx) (errs errors.MultiError) {
	if g.frozen {
		return errors.MultiError{
			fmt.Errorf("generators list is frozen, nested iteration?"),
		}
	}
	g.frozen = true
	defer func() { g.frozen = false }()

	fc := builtins.GetFailureCollector(th)

	for _, cb := range g.gen {
		if fc != nil {
			fc.Clear()
		}
		if _, err := starlark.Call(th, cb, starlark.Tuple{ctx}, nil); err != nil {
			if fc != nil && fc.LatestFailure() != nil {
				// Prefer this error, it has custom stack trace.
				errs = append(errs, fc.LatestFailure())
			} else {
				errs = append(errs, err)
			}
		}
	}
	return
}

func init() {
	// new_config_set() makes a new empty config set, useful in tests.
	declNative("new_config_set", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return newConfigSet(), nil
	})

	// add_generator(cb) registers a callback that is called at the end of the
	// execution to generate or mutate produced configs.
	declNative("add_generator", func(call nativeCall) (starlark.Value, error) {
		var cb starlark.Callable
		if err := call.unpack(1, &cb); err != nil {
			return nil, err
		}
		return starlark.None, call.State.generators.add(cb)
	})

	// call_generators(ctx) calls all registered generators, useful in tests.
	declNative("call_generators", func(call nativeCall) (starlark.Value, error) {
		var ctx *genCtx
		if err := call.unpack(1, &ctx); err != nil {
			return nil, err
		}
		switch errs := call.State.generators.call(call.Thread, ctx); {
		case len(errs) == 0:
			return starlark.None, nil
		case len(errs) == 1:
			return starlark.None, errs[0]
		default:
			return starlark.None, errs
		}
	})
}
