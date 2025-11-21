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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
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

func exampleCheck() *orchestratorpb.Check {
	mkDatum := func(msg proto.Message) *orchestratorpb.Datum {
		return orchestratorpb.Datum_builder{
			Value: data.Value(msg),
		}.Build()
	}
	rsltID := func(i int) *idspb.CheckResult {
		ret, err := id.CheckResultErr("whatever", i)
		if err != nil {
			panic(err)
		}
		return ret
	}

	return orchestratorpb.Check_builder{
		Options: []*orchestratorpb.Datum{
			mkDatum(must(structpb.NewList([]any{1, 2, 3}))),
			mkDatum(must(structpb.NewStruct(map[string]any{
				"hello": []any{"nerds", "and", "ppls"},
				"extra": 100,
			}))),
		},
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
					mkDatum(must(structpb.NewList([]any{4, 5, 6}))),
					mkDatum(must(structpb.NewStruct(map[string]any{
						"bye": []any{"party", "droids"},
					}))),
				},
			}.Build(),
		},
	}.Build()
}

func ExampleGetOption() {
	opt := data.GetOption[*structpb.Struct](exampleCheck()).AsMap()
	for _, key := range slices.Sorted(maps.Keys(opt)) {
		fmt.Println(key, opt[key])
	}
	// Output:
	// extra 100
	// hello [nerds and ppls]
}

func ExampleGetResults() {
	results := data.GetResults[*structpb.ListValue](exampleCheck())
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
