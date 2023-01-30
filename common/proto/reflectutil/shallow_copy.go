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

package reflectutil

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ShallowCopy returns a new, shallow copy of `msg`.
//
// This is a safe alterative to the "obvious" implementation:
//
//	tmp := *underlyingMsgValue
//	return &tmp
//
// Unfortunately, protobuf holds many internal structures which are
// unsafe to shallow copy, and `go vet` has been hinted to complain
// about this.
//
// If `msg` is invalid (typically a nil *SomeMessage), this returns `msg`
// as-is.
//
// This implementation also copies unknown fields.
func ShallowCopy(msg proto.Message) proto.Message {
	msgr := msg.ProtoReflect()
	if !msgr.IsValid() {
		return msg
	}

	ret := msgr.New()
	msgr.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		ret.Set(field, value)
		return true
	})
	ret.SetUnknown(msgr.GetUnknown())

	return ret.Interface()
}
