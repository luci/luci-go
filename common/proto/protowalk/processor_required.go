// Copyright 2022 The LUCI Authors.
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

package protowalk

import (
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// RequiredProcessor implements the recommended behavior of
// aip.dev/203#required, namely that it produces a Report for unset
// fields marked like:
//
//	type name = <tag> [(google.api.field_behavior) = REQUIRED];
//
// NOTE: In LUCI code, it's recommended to use "protoc-gen-validate" (i.e. PGV)
// annotations to indicate required fields. The PGV generated code is easier to
// use and (likely) more efficient than the implementation in this package.
type RequiredProcessor struct{}

var _ FieldProcessor = RequiredProcessor{}

// Process implements FieldProcessor.
func (RequiredProcessor) Process(_ DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool) {
	return ResultData{Message: "required", IsErr: true}, true
}

func (RequiredProcessor) ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		for _, fb := range proto.GetExtension(fo, annotations.E_FieldBehavior).([]annotations.FieldBehavior) {
			if fb == annotations.FieldBehavior_REQUIRED {
				return ProcessIfUnset
			}
		}
	}
	return ProcessNever
}
