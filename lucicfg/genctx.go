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
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/lucicfg/graph"
)

// genCtx is a starlark struct that represents the state passed to generator
// callbacks (as first and only argument).
type genCtx struct {
	starlarkstruct.Struct

	graph     *graph.Graph // frozen graph with all the state
	configSet *configSet   // output config set
}

func newGenCtx(g *graph.Graph) *genCtx {
	ctx := &genCtx{
		graph:     g,
		configSet: newConfigSet(),
	}

	graphAttr := func(key string) starlark.Value {
		val, err := g.Attr(key)
		if err != nil {
			panic(err)
		}
		if val == nil {
			panic("expectedly missing attribute")
		}
		return val
	}

	ctx.Struct = *starlarkstruct.FromStringDict(starlark.String("gen_ctx"), starlark.StringDict{
		"config_set": ctx.configSet,

		// Limited read-only view of the graph API, to make sure generators don't
		// have access to the graph mutation API.
		"graph": starlarkstruct.FromStringDict(starlark.String("graph"), starlark.StringDict{
			"node": graphAttr("node"),
		}),
	})
	return ctx
}

func init() {
	// new_gen_ctx() makes a new empty generator context object.
	declNative("new_gen_ctx", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return newGenCtx(&call.State.graph), nil
	})
}
