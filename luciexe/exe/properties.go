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

package exe

import (
	"context"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"go.chromium.org/luci/common/logging"
)

// NamespaceProperties adjusts the 'root' property key given one or more
// namespace tokens.
//
// This is useful to allow some library code access to properties, but at
// a location of your choosing, and to be sure that that library can only see
// and manipulate data within this scope.
//
// Namespaced properties are lazy; They will be populated in the build's output
// properties only when ModifyProperties is called.
//
// For example, given the properties:
//
//    {
//      "foo": {"bar": {"some": "things"}},
//      "numeric": 100,
//      "other": ...
//    }
//
// If you create a context with namespace=["foo", "bar"], then ModifyProperties
// using this contex will be able to see/mutate only {"some": "things"}.
//
// If you create a context with namespace=["numeric"], then ModifyProperties
// will overwrite the "numeric" value to `{}` when called, presenting the
// callback with an empty struct.
func NamespaceProperties(ctx context.Context, namespace ...string) context.Context {
	return addPropertyNS(ctx, namespace)
}

// ModifyProperties allows you to atomically read and write the output
// properties object within the current `NamespaceProperties` scope of the
// overall Build.
//
// See WriteProperties and ParseProperties, as well as the AsMap helper method
// on *Struct for assistance dealing with the Struct proto message.
func ModifyProperties(ctx context.Context, cb func(props *structpb.Struct) error) error {
	build := getBuild(ctx)
	propNS := getPropertyNS(ctx)

	return build.modLock(func() error {
		build.propsMu.Lock()
		defer build.propsMu.Unlock()

		node := build.build.Output.Properties
		if node == nil {
			build.build.Output.Properties = mkEmptyStruct().GetStructValue()
			node = build.build.Output.Properties
		}

		for i, tok := range propNS {
			newNode := node.Fields[tok].GetStructValue()
			if newNode == nil {
				if node.Fields[tok] != nil {
					logging.Debugf(ctx, "overwriting properties %q to Struct", propNS[:i])
				}
				node.Fields[tok] = mkEmptyStruct()
				newNode = node.Fields[tok].GetStructValue()
			}
			node = newNode
		}

		return cb(node)
	})
}

func mkEmptyStruct() *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
		Fields: map[string]*structpb.Value{},
	}}}
}
