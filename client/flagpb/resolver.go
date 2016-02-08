// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flagpb

import "github.com/luci/luci-go/common/proto/google/descriptor"

// Resolver resolves type names.
type Resolver interface {
	// Resolve resolves a type name to
	// *descriptor.DescriptorProto or *descriptor.EnumDescriptorProto.
	Resolve(name string) interface{}
}

// NewResolver creates a resolver for all types in a file descriptor set.
// Resolving time complexity is linear.
func NewResolver(set *descriptor.FileDescriptorSet) Resolver {
	return &descriptorSetResolver{set}
}

type descriptorSetResolver struct {
	set *descriptor.FileDescriptorSet
}

func (r *descriptorSetResolver) Resolve(name string) interface{} {
	_, o, _ := r.set.Resolve(name)
	switch o := o.(type) {
	case *descriptor.DescriptorProto, *descriptor.EnumDescriptorProto:
		return o
	default:
		return nil
	}
}
