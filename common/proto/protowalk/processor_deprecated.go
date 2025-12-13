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
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// DeprecatedProcessor will find, and produce a Result for every
// deprecated field which is set. Deprecated fields are marked like:
//
//	type name = <tag> [deprecated = true];
//
// You may also optionally clear the data in deprecated fields by providing
// DeprecatedProcessorClearField.Value(true) to Execute.
type DeprecatedProcessor struct{}

var DeprecatedProcessorClearField = NewDataHandle[bool]()

var _ FieldProcessor = DeprecatedProcessor{}

// Process implements [FieldProcessor].
//
// This will optionally clear (zero) the field in `msg` to avoid accidental
// usage of deprecated fields
func (d DeprecatedProcessor) Process(data DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (ResultData, bool) {
	if DeprecatedProcessorClearField.Get(data) {
		msg.Clear(field)
	}
	return ResultData{Message: "deprecated"}, true
}

// ShouldProcess implements [FieldProcessor].
//
// This flags all fields which have `[deprecated = true]` and ignores all other
// fields.
func (d DeprecatedProcessor) ShouldProcess(field protoreflect.FieldDescriptor) ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		if fo.Deprecated != nil && *fo.Deprecated {
			return ProcessIfSet
		}
	}
	return ProcessNever
}
