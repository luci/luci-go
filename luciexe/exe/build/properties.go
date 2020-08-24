// Copyright 2020 The LUCI Authors.
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

package build

import (
	"context"

	structpb "github.com/golang/protobuf/ptypes/struct"
)

// ModifyProperties allows you to atomically read and write the output
// properties object within the current Build.
//
// See WriteProperties and ParseProperties, as well as the AsMap helper method
// on *Struct for assistance dealing with the Struct proto message.
func ModifyProperties(ctx context.Context, cb func(props *structpb.Struct) error) error {
	state := getState(ctx)

	return state.modLock(func() error {
		state.propsMu.Lock()
		defer state.propsMu.Unlock()

		node := state.build.Output.Properties
		if node == nil {
			state.build.Output.Properties = mkEmptyStruct().GetStructValue()
			node = state.build.Output.Properties
		}

		return cb(node)
	})
}

func mkEmptyStruct() *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
		Fields: map[string]*structpb.Value{},
	}}}
}
