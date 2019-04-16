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
	"path"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// genCtx is a starlark struct that represents the state passed to generator
// callbacks (as first and only argument).
type genCtx struct {
	starlarkstruct.Struct

	output *outputBuilder
	roots  map[string]string // name of the set => its root directory
}

func newGenCtx() *genCtx {
	ctx := &genCtx{
		output: newOutputBuilder(),
		roots:  map[string]string{},
	}
	ctx.Struct = *starlarkstruct.FromStringDict(starlark.String("gen_ctx"), starlark.StringDict{
		"output":             ctx.output,
		"declare_config_set": starlark.NewBuiltin("declare_config_set", ctx.declareConfigSetImpl),
	})
	return ctx
}

func (c *genCtx) declareConfigSetImpl(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name starlark.String
	var root starlark.String
	err := starlark.UnpackArgs("declare_config_set", args, kwargs,
		"name", &name,
		"root", &root)
	if err != nil {
		return nil, err
	}
	nameS := name.GoString()
	rootS := path.Clean(root.GoString())

	// It is OK to redeclare exact same root. It is not OK to change it.
	if existing, ok := c.roots[nameS]; ok && existing != rootS {
		return nil, fmt.Errorf("declare_config_set: set %q has already been declared", nameS)
	}
	c.roots[nameS] = rootS

	return starlark.None, nil
}

func (c *genCtx) assembleOutput(textPBHeader string) (Output, error) {
	files, err := c.output.renderWithTextProto(textPBHeader)
	return Output{
		Data:  files,
		Roots: c.roots,
	}, err
}

func init() {
	// new_gen_ctx() makes a new empty generator context object.
	declNative("new_gen_ctx", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return newGenCtx(), nil
	})
}
