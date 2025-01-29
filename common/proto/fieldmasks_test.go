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

package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/proto/internal/testingpb"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFixFieldMasks(t *testing.T) {
	t.Parallel()

	ftt.Run("OK", t, func(t *ftt.Test) {
		cases := []struct {
			name     string
			body     string
			expected proto.Message
		}{
			{
				"no field mask",
				`{"id": "1"}`,
				&testingpb.Simple{Id: 1},
			},
			{
				"works with str",
				`{"fields": "id,someField"}`,
				&testingpb.Simple{Fields: &fieldmaskpb.FieldMask{Paths: []string{
					"id", "some_field",
				}}},
			},
			{
				"works with obj",
				`{"fields": {"paths": ["id", "someField"]}}`,
				&testingpb.Simple{Fields: &fieldmaskpb.FieldMask{Paths: []string{
					"id", "some_field",
				}}},
			},
			{
				"works with obj and snake case",
				`{"fields": {"paths": ["id", "some_field"]}}`,
				&testingpb.Simple{Fields: &fieldmaskpb.FieldMask{Paths: []string{
					"id", "some_field",
				}}},
			},
			{
				"JSONPB field names in the message itself",
				`{"otherSome": {"i": 1}, "customJSON": {"i": 2}, "otherFields": "id,someField"}`,
				&testingpb.Simple{
					OtherSome:     &testingpb.Some{I: 1},
					OtherSomeJson: &testingpb.Some{I: 2},
					OtherFields: &fieldmaskpb.FieldMask{Paths: []string{
						"id", "some_field",
					}},
				},
			},
			{
				"proto field names in the message itself",
				`{"other_some": {"i": 1}, "other_some_json": {"i": 2}, "other_fields": "id,someField"}`,
				&testingpb.Simple{
					OtherSome:     &testingpb.Some{I: 1},
					OtherSomeJson: &testingpb.Some{I: 2},
					OtherFields: &fieldmaskpb.FieldMask{Paths: []string{
						"id", "some_field",
					}},
				},
			},
			{
				"unknown field in the message itself",
				`{"this_field_isnt_defined": 1}`,
				&testingpb.Simple{},
			},
			{
				"structpb",
				`{"properties": {"foo": "bar"}}`,
				&testingpb.Props{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"foo": {Kind: &structpb.Value_StringValue{StringValue: "bar"}},
						},
					},
				},
			},
			{
				"nested type",
				`{"msgs": [{"simple": {"fields": "id,someField"}}]}`,
				&testingpb.WithInner{
					Msgs: []*testingpb.WithInner_Inner{
						{
							Msg: &testingpb.WithInner_Inner_Simple{
								Simple: &testingpb.Simple{Fields: &fieldmaskpb.FieldMask{Paths: []string{
									"id", "some_field",
								}}},
							},
						},
					},
				},
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *ftt.Test) {
				clone := proto.Clone(c.expected)
				proto.Reset(clone)
				assert.That(t, UnmarshalJSONWithNonStandardFieldMasks([]byte(c.body), clone), should.ErrLike(nil))
				assert.That(t, clone, should.Match(c.expected))
			})
		}
	})

	ftt.Run("Fails", t, func(t *ftt.Test) {
		t.Run("quotes", func(t *ftt.Test) {
			assert.That(t, parseFieldMaskString("`a,b`,c"), should.Match([]string{"`a,b`", "c"}))
		})

		t.Run("two seps", func(t *ftt.Test) {
			assert.That(t, parseFieldMaskString("a,b,c"), should.Match([]string{"a", "b", "c"}))
		})
	})
}
