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

// OutputOnlyProcessor implements the recommended behavior of
// aip.dev/203#output-only, namely that fields marked like:
//
//	type name = <tag> [(google.api.field_behavior) = OUTPUT_ONLY];
//
// Should be cleared by the server (without raising an error) if
// a user request includes a value for them.
type OutputOnlyProcessor struct{}

var _ FieldProcessor = (*OutputOnlyProcessor)(nil)

// Process implements FieldProcessor.
//
// This will clear (zero) the field in `msg`.
func (OutputOnlyProcessor) Process(_ DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool) {
	msg.Clear(field)
	return ResultData{Message: "cleared OUTPUT_ONLY field"}, true
}

func (OutputOnlyProcessor) ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		for _, fb := range proto.GetExtension(fo, annotations.E_FieldBehavior).([]annotations.FieldBehavior) {
			if fb == annotations.FieldBehavior_OUTPUT_ONLY {
				return ProcessIfSet
			}
		}
	}
	return ProcessNever
}
