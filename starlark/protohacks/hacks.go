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

// Package protohacks is temporary.
//
// It will disappear once we stop vendoring "google.golang.org/protobuf".
//
// As of now, only code "close by" to the "vendor" folder can directly import
// types from "google.golang.org/protobuf".
package protohacks

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// FileOptions returns proto-serialized descriptorpb.FileOptions.
//
// Return (nil, nil) if the file has no options.
func FileOptions(d protoreflect.FileDescriptor) ([]byte, error) {
	opts := d.Options().(*descriptorpb.FileOptions)
	if opts == nil {
		return nil, nil
	}
	return proto.Marshal(opts)
}
