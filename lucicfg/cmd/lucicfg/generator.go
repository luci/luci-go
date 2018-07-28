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

package main

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/google/skylark"

	"go.chromium.org/luci/skylark/skylarkproto"
)

// Generator knows how to generate LUCI configs.
//
// Its internal state is populated during the execution of skylark scripts. At
// the end this state is used to generate actual config files.
type Generator struct {
	protos []stagedProto
}

type stagedProto struct {
	configSet string // either 'project' or 'ref'
	name      string // name of the file to drop proto to
	msg       *skylarkproto.Message
}

// Report just prints generated configs to stdout for debugging.
func (g *Generator) Report() {
	for i, p := range g.protos {
		if i != 0 {
			fmt.Println()
		}
		fmt.Printf("-----------------------------------------\n")
		fmt.Printf("%s/%s:\n", p.configSet, p.name)
		fmt.Printf("-----------------------------------------\n")
		msg, err := p.msg.ToProto()
		if err != nil {
			fmt.Printf("<error: %s>\n", err)
		} else {
			fmt.Printf("%s", proto.MarshalTextString(msg))
		}
		fmt.Printf("-----------------------------------------\n")
	}
}

// ExportedBuiltins is builtins that became available to the skylark scripts.
//
// They hold reference back to the generator, to modify its state during
// scripts' execution.
func (g *Generator) ExportedBuiltins() skylark.StringDict {
	all := skylark.StringDict{}
	add := func(name string, fn func(*skylark.Thread, *skylark.Builtin, skylark.Tuple, []skylark.Tuple) (skylark.Value, error)) {
		all[name] = skylark.NewBuiltin(name, fn)
	}
	add("__write_proto_later", g.expWriteProtoLater)
	return all
}

// expWriteProtoLater stages a proto object for serialization at the end of
// the script execution.
func (g *Generator) expWriteProtoLater(_ *skylark.Thread, fn *skylark.Builtin, args skylark.Tuple, kwargs []skylark.Tuple) (skylark.Value, error) {
	var configSet skylark.String
	var path skylark.String
	var msg *skylarkproto.Message
	if err := skylark.UnpackPositionalArgs(fn.Name(), args, kwargs, 3, &configSet, &path, &msg); err != nil {
		return nil, err
	}
	g.protos = append(g.protos, stagedProto{string(configSet), string(path), msg})
	return skylark.None, nil
}
