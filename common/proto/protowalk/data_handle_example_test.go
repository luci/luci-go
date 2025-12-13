// Copyright 2025 The LUCI Authors.
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

package protowalk_test

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/proto/prototest"
	"go.chromium.org/luci/common/proto/protowalk"
)

// ExampleProcessor resets all string fields of a given value to a different
// value given by ExampleProcessorHandle.
type ExampleProcessor struct{}

type ExampleProcessorPair struct {
	Search  string
	Replace string
}

var ExampleProcessorHandle = protowalk.NewDataHandle[ExampleProcessorPair]()

var _ protowalk.FieldProcessor = ExampleProcessor{}

func (ExampleProcessor) ShouldProcess(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
	if field.Kind() == protoreflect.StringKind {
		return protowalk.ProcessIfSet
	}
	return protowalk.ProcessNever
}

func (ExampleProcessor) Process(data protowalk.DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (result protowalk.ResultData, applied bool) {
	instruction := ExampleProcessorHandle.Get(data)
	if instruction.Search != msg.Get(field).String() {
		return
	}
	msg.Set(field, protoreflect.ValueOfString(instruction.Replace))
	applied = true
	return
}

var ExampleProcessorWalker = protowalk.NewWalker[*protowalk.Outer](ExampleProcessor{})

func ExampleDataMap() {
	msg := &protowalk.Outer{
		Regular: "needle",

		MultiInner: []*protowalk.Inner{
			{
				Regular: "avoid",
				Custom:  "needle",
			},
		},
	}

	ExampleProcessorWalker.Execute(msg, ExampleProcessorHandle.Value(ExampleProcessorPair{
		Search:  "needle",
		Replace: "balloon",
	}))

	prototest.Print(msg, nil)
	// Output:
	// {
	//   "regular": "balloon",
	//   "multiInner": [
	//     {
	//       "regular": "avoid",
	//       "custom": "balloon"
	//     }
	//   ]
	// }
}
