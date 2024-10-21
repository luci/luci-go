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

package exe

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testStruct struct {
	Field string `json:"field"`
}

func TestProperties(t *testing.T) {
	ftt.Run(`test property helpers`, t, func(t *ftt.Test) {
		props := &structpb.Struct{}

		expectedStruct := &testStruct{Field: "hi"}
		expectedProto := &bbpb.Build{SummaryMarkdown: "there"}
		expectedStrings := []string{"not", "a", "struct"}
		assert.Loosely(t, WriteProperties(props, map[string]any{
			"struct":  expectedStruct,
			"proto":   expectedProto,
			"strings": expectedStrings,
			"null":    Null,
		}), should.BeNil)
		assert.Loosely(t, props, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"struct": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"field": {Kind: &structpb.Value_StringValue{
							StringValue: "hi",
						}},
					}},
				}},
				"proto": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"summary_markdown": {Kind: &structpb.Value_StringValue{
							StringValue: "there",
						}},
					}},
				}},
				"strings": {Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "not"}},
						{Kind: &structpb.Value_StringValue{StringValue: "a"}},
						{Kind: &structpb.Value_StringValue{StringValue: "struct"}},
					},
				}}},
				"null": {Kind: &structpb.Value_NullValue{NullValue: 0}},
			},
		}))

		readStruct := &testStruct{}
		extraStruct := &testStruct{}
		readProto := &bbpb.Build{}
		var readStrings []string
		readNil := any(100) // not currently nil
		assert.Loosely(t, ParseProperties(props, map[string]any{
			"struct":       readStruct,
			"extra_struct": extraStruct,
			"proto":        readProto,
			"strings":      &readStrings,
			"null":         &readNil,
		}), should.BeNil)
		assert.Loosely(t, readStruct, should.Resemble(expectedStruct))
		assert.Loosely(t, extraStruct, should.Resemble(&testStruct{}))
		assert.Loosely(t, readStrings, should.Resemble(expectedStrings))
		assert.Loosely(t, readNil, should.BeNil)
		assert.Loosely(t, readProto, should.Resemble(expectedProto))

		// now, delete some keys
		assert.Loosely(t, WriteProperties(props, map[string]any{
			"struct":         nil,
			"proto":          nil,
			"does_not_exist": nil,
		}), should.BeNil)
		assert.Loosely(t, props, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"strings": {Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{
					Values: []*structpb.Value{
						{Kind: &structpb.Value_StringValue{StringValue: "not"}},
						{Kind: &structpb.Value_StringValue{StringValue: "a"}},
						{Kind: &structpb.Value_StringValue{StringValue: "struct"}},
					},
				}}},
				"null": {Kind: &structpb.Value_NullValue{NullValue: 0}},
			},
		}))
	})
}
