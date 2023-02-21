// Copyright 2016 The LUCI Authors.
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

package flagpb

import (
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/proto/google/descutil"
)

// Resolver resolves type names.
type Resolver interface {
	// Resolve resolves a type name to
	// *descriptor.DescriptorProto or *descriptor.EnumDescriptorProto.
	Resolve(name string) any
}

// NewResolver creates a resolver for all types in a file descriptor set.
// Resolving time complexity is linear.
func NewResolver(set *descriptorpb.FileDescriptorSet) Resolver {
	return &descriptorSetResolver{set}
}

type descriptorSetResolver struct {
	set *descriptorpb.FileDescriptorSet
}

func (r *descriptorSetResolver) Resolve(name string) any {
	_, o, _ := descutil.Resolve(r.set, name)
	switch o := o.(type) {
	case *descriptorpb.DescriptorProto, *descriptorpb.EnumDescriptorProto:
		return o
	default:
		return nil
	}
}
