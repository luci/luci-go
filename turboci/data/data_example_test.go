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

package data_test

import (
	"fmt"
	"maps"
	"slices"

	"google.golang.org/protobuf/types/known/structpb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
)

func ExampleURL() {
	fmt.Println(data.URL[*structpb.Struct]())
	// Output:
	// type.googleapis.com/google.protobuf.Struct
}

func ExampleURLMsg() {
	fmt.Println(data.URLMsg((*structpb.Struct)(nil)))
	// Output:
	// type.googleapis.com/google.protobuf.Struct
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

func ExampleValue() {
	val := data.Value(must(structpb.NewList([]any{"hello", "world"})))

	fmt.Println(val.GetValue().TypeUrl)
	fmt.Println(len(val.GetValue().Value))
	// Output:
	// type.googleapis.com/google.protobuf.ListValue
	// 18
}

func ExampleExtractValue() {
	val := data.Value(must(structpb.NewList([]any{"hello", "world"})))

	lst := data.ExtractValue[*structpb.ListValue](val)
	for _, element := range lst.AsSlice() {
		fmt.Println(element)
	}
	// Output:
	// hello
	// world
}

func exampleCheckView() *orchestratorpb.CheckView {
	return orchestratorpb.CheckView_builder{
		OptionData: map[string]*orchestratorpb.Datum{
			data.URL[*structpb.ListValue](): orchestratorpb.Datum_builder{
				Value: data.Value(must(structpb.NewList([]any{1, 2, 3}))),
			}.Build(),
			data.URL[*structpb.Struct](): orchestratorpb.Datum_builder{
				Value: data.Value(must(structpb.NewStruct(map[string]any{
					"hello": []any{"nerds", "and", "ppls"},
					"extra": 100,
				}))),
			}.Build(),
		},
		Results: map[int32]*orchestratorpb.CheckResultView{
			1: orchestratorpb.CheckResultView_builder{
				Data: map[string]*orchestratorpb.Datum{
					data.URL[*structpb.ListValue](): orchestratorpb.Datum_builder{
						Value: data.Value(must(structpb.NewList([]any{1, 2, 3}))),
					}.Build(),
					data.URL[*structpb.Struct](): orchestratorpb.Datum_builder{
						Value: data.Value(must(structpb.NewStruct(map[string]any{
							"hello": []any{"nerds", "and", "ppls"},
						}))),
					}.Build(),
				},
			}.Build(),
			3: orchestratorpb.CheckResultView_builder{
				Data: map[string]*orchestratorpb.Datum{
					data.URL[*structpb.ListValue](): orchestratorpb.Datum_builder{
						Value: data.Value(must(structpb.NewList([]any{4, 5, 6}))),
					}.Build(),
					data.URL[*structpb.Struct](): orchestratorpb.Datum_builder{
						Value: data.Value(must(structpb.NewStruct(map[string]any{
							"bye": []any{"party", "droids"},
						}))),
					}.Build(),
				},
			}.Build(),
		},
	}.Build()
}

func ExampleGetOption() {
	cv := exampleCheckView()
	opt := data.GetOption[*structpb.Struct](cv).AsMap()
	for _, key := range slices.Sorted(maps.Keys(opt)) {
		fmt.Println(key, opt[key])
	}
	// Output:
	// extra 100
	// hello [nerds and ppls]
}

func ExampleGetResults() {
	cv := exampleCheckView()
	results := data.GetResults[*structpb.ListValue](cv)
	for _, idx := range slices.Sorted(maps.Keys(results)) {
		for _, element := range results[idx].AsSlice() {
			fmt.Printf("result %d: %v\n", idx, element)
		}
	}
	// Output:
	// result 1: 1
	// result 1: 2
	// result 1: 3
	// result 3: 4
	// result 3: 5
	// result 3: 6
}
