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

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	cv := orchestratorpb.CheckView_builder{
		OptionData: map[string]*orchestratorpb.Datum{
			URL[*structpb.ListValue](): orchestratorpb.Datum_builder{
				Value: Value(must(structpb.NewList([]any{1, 2, 3}))),
			}.Build(),
			URL[*structpb.Struct](): orchestratorpb.Datum_builder{
				Value: Value(must(structpb.NewStruct(map[string]any{
					"hello": []any{"nerds", "and", "ppls"},
				}))),
			}.Build(),
		},
	}.Build()

	extracted := GetOption[*structpb.Struct](cv)
	assert.That(t, extracted, should.Match(must(structpb.NewStruct(map[string]any{
		"hello": []any{"nerds", "and", "ppls"},
	}))))
}

func TestGetResults(t *testing.T) {
	cv := orchestratorpb.CheckView_builder{
		Results: map[int32]*orchestratorpb.CheckResultView{
			1: orchestratorpb.CheckResultView_builder{
				Data: map[string]*orchestratorpb.Datum{
					URL[*structpb.ListValue](): orchestratorpb.Datum_builder{
						Value: Value(must(structpb.NewList([]any{1, 2, 3}))),
					}.Build(),
					URL[*structpb.Struct](): orchestratorpb.Datum_builder{
						Value: Value(must(structpb.NewStruct(map[string]any{
							"hello": []any{"nerds", "and", "ppls"},
						}))),
					}.Build(),
				},
			}.Build(),
			3: orchestratorpb.CheckResultView_builder{
				Data: map[string]*orchestratorpb.Datum{
					URL[*structpb.ListValue](): orchestratorpb.Datum_builder{
						Value: Value(must(structpb.NewList([]any{1, 2, 3}))),
					}.Build(),
					URL[*structpb.Struct](): orchestratorpb.Datum_builder{
						Value: Value(must(structpb.NewStruct(map[string]any{
							"bye": []any{"party", "droids"},
						}))),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()

	results := GetResults[*structpb.Struct](cv)
	assert.That(t, results, should.Match(map[int32]*structpb.Struct{
		1: must(structpb.NewStruct(map[string]any{
			"hello": []any{"nerds", "and", "ppls"},
		})),
		3: must(structpb.NewStruct(map[string]any{
			"bye": []any{"party", "droids"},
		})),
	}))
}
