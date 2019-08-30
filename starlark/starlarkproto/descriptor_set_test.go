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

package starlarkproto

import (
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"
)

func fdp(name string, deps ...string) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       &name,
		Dependency: deps,
	}
}

func fdps(d ...*descriptorpb.FileDescriptorProto) []*descriptorpb.FileDescriptorProto { return d }
func deps(d ...*DescriptorSet) []*DescriptorSet                                       { return d }

func TestDescriptorSetDiamond(t *testing.T) {
	t.Parallel()

	base, err := NewDescriptorSet("base", fdps(fdp("base.proto")), nil)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	left, err := NewDescriptorSet("left", fdps(fdp("left.proto", "base.proto")), deps(base))
	if err != nil {
		t.Fatalf("left: %v", err)
	}
	right, err := NewDescriptorSet("right", fdps(fdp("right.proto", "base.proto")), deps(base))
	if err != nil {
		t.Fatalf("right: %v", err)
	}
	_, err = NewDescriptorSet("top", fdps(fdp("top.proto", "left.proto", "right.proto")), deps(left, right))
	if err != nil {
		t.Fatalf("top: %v", err)
	}
}

func TestDescriptorSetDupFile(t *testing.T) {
	t.Parallel()

	a, _ := NewDescriptorSet("a", fdps(fdp("file.proto")), nil)
	b, _ := NewDescriptorSet("b", fdps(fdp("file.proto")), nil)

	if _, err := NewDescriptorSet("c", nil, deps(a, b)); err == nil {
		t.Fatalf("unexpectedly succeeded")
	}
}

func TestDescriptorSetUnknownFile(t *testing.T) {
	t.Parallel()

	a, _ := NewDescriptorSet("a", fdps(fdp("a.proto")), nil)
	if _, err := NewDescriptorSet("b", fdps(fdp("b.proto", "a.proto", "unknown.proto")), deps(a)); err == nil {
		t.Fatalf("unexpectedly succeeded")
	}
}
