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

package data

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/turboci/id"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestURL(t *testing.T) {
	assert.That(t, URL[*structpb.Struct](), should.Equal(
		"type.googleapis.com/google.protobuf.Struct",
	))
}

func TestURLMsg(t *testing.T) {
	assert.That(t, URLMsg((*structpb.Struct)(nil)), should.Equal(
		"type.googleapis.com/google.protobuf.Struct",
	))
}

func TestValueErr(t *testing.T) {
	st, err := structpb.NewStruct(map[string]any{
		"hello": 100,
		"world": 200,
	})
	assert.NoErr(t, err)

	val, err := ValueErr(st)
	assert.NoErr(t, err)

	extracted := ExtractValue[*structpb.Struct](val)
	assert.That(t, extracted, should.Match(st))
}

func TestExtractMismatch(t *testing.T) {
	st, err := structpb.NewStruct(map[string]any{
		"hello": 100,
		"world": 200,
	})
	assert.NoErr(t, err)

	val, err := ValueErr(st)
	assert.NoErr(t, err)

	extracted := ExtractValue[*structpb.Value](val)
	assert.That(t, extracted, should.Match((*structpb.Value)(nil)))
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

func TestGetOption(t *testing.T) {
	c := orchestratorpb.Check_builder{
		Options: []*orchestratorpb.Datum{
			orchestratorpb.Datum_builder{
				Value: Value(must(structpb.NewList([]any{1, 2, 3}))),
			}.Build(),
			orchestratorpb.Datum_builder{
				Value: Value(must(structpb.NewStruct(map[string]any{
					"hello": []any{"nerds", "and", "ppls"},
				}))),
			}.Build(),
		},
	}.Build()

	extracted := GetOption[*structpb.Struct](c)
	assert.That(t, extracted, should.Match(must(structpb.NewStruct(map[string]any{
		"hello": []any{"nerds", "and", "ppls"},
	}))))
}

func mkDatum(msg proto.Message) *orchestratorpb.Datum {
	return orchestratorpb.Datum_builder{
		Value: Value(msg),
	}.Build()
}

func rsltID(i int) *idspb.CheckResult {
	ret, err := id.CheckResultErr("whatever", i)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestGetResults(t *testing.T) {
	c := orchestratorpb.Check_builder{
		Results: []*orchestratorpb.Check_Result{
			orchestratorpb.Check_Result_builder{
				Identifier: rsltID(1),
				Data: []*orchestratorpb.Datum{
					mkDatum(must(structpb.NewList([]any{1, 2, 3}))),
					mkDatum(must(structpb.NewStruct(map[string]any{
						"hello": []any{"nerds", "and", "ppls"},
					}))),
				},
			}.Build(),
			orchestratorpb.Check_Result_builder{
				Identifier: rsltID(2),
			}.Build(),
			orchestratorpb.Check_Result_builder{
				Identifier: rsltID(3),
				Data: []*orchestratorpb.Datum{
					mkDatum(must(structpb.NewList([]any{1, 2, 3}))),
					mkDatum(must(structpb.NewStruct(map[string]any{
						"bye": []any{"party", "droids"},
					}))),
				},
			}.Build(),
		},
	}.Build()

	results := GetResults[*structpb.Struct](c)
	assert.That(t, results, should.Match(map[int32]*structpb.Struct{
		1: must(structpb.NewStruct(map[string]any{
			"hello": []any{"nerds", "and", "ppls"},
		})),
		3: must(structpb.NewStruct(map[string]any{
			"bye": []any{"party", "droids"},
		})),
	}))
}

func TestValuesErr(t *testing.T) {
	t.Parallel()

	st1, err := structpb.NewStruct(map[string]any{
		"a": 1,
		"b": "two",
	})
	assert.NoErr(t, err)

	lv1, err := structpb.NewList([]any{
		"c", "d",
	})
	assert.NoErr(t, err)

	bv1 := structpb.NewBoolValue(true)

	msgs := []proto.Message{st1, lv1, bv1}
	vals, err := ValuesErr(msgs...)
	assert.NoErr(t, err)
	assert.Loosely(t, vals, should.HaveLength(3))

	extractedSt1 := ExtractValue[*structpb.Struct](vals[0])
	assert.That(t, extractedSt1, should.Match(st1))

	extractedSt2 := ExtractValue[*structpb.ListValue](vals[1])
	assert.That(t, extractedSt2, should.Match(lv1))

	extractedLv1 := ExtractValue[*structpb.Value](vals[2])
	assert.That(t, extractedLv1, should.Match(bv1))
}
