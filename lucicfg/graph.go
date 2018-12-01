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

import (
	"fmt"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/lucicfg/graph"
)

// Internal Starlark API for constructing graphs, as defined in 'graph' package.

// configGraph is a graph with config objects, living in State.
type configGraph struct {
	keys graph.KeySet
}

func init() {
	// graph_key(typ1, id2, typ2, id2, ...) returns the corresponding *Key object.
	declNative("graph_key", func(call nativeCall) (starlark.Value, error) {
		if len(call.Kwargs) != 0 {
			return nil, fmt.Errorf("graph_key: not expecting keyword arguments")
		}
		pairs := make([]string, len(call.Args))
		for idx, arg := range call.Args {
			str, ok := arg.(starlark.String)
			if !ok {
				return nil, fmt.Errorf("graph_key: all arguments must be strings, arg #%d was %s", idx, arg.Type())
			}
			pairs[idx] = str.GoString()
		}
		return call.State.graph.keys.Key(pairs...)
	})
}
