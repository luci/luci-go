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
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestOutputOnly(t *testing.T) {
	t.Parallel()

	ftt.Run(`Output-only field check`, t, func(t *ftt.Test) {
		msg := &Outer{
			Output: "stuff",
			OutputInner: &Inner{
				Regular: "a bunch of stuff",
				Output:  "ignored because output_inner is cleared",
				Struct: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"input": {
							Kind: &structpb.Value_StringValue{
								StringValue: "inner struct",
							},
						},
					},
				},
				Recursive: &Inner_Recursive{
					OutputOnly: 1,
					Regular:    1,
					Next: &Inner_Recursive{
						OutputOnly: 2,
						Regular:    2,
					},
				},
			},
			IntMapInner: map[int32]*Inner{
				1: {
					Recursive: &Inner_Recursive{
						OutputOnly: 1,
						Regular:    1,
						Next: &Inner_Recursive{
							OutputOnly: 2,
							Regular:    2,
							Next: &Inner_Recursive{
								OutputOnly: 3,
								Regular:    3,
							},
						},
					},
				},
			},
			Struct: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"input": {
						Kind: &structpb.Value_StringValue{
							StringValue: "outer struct",
						},
					},
				},
			},
			A: &A{
				AValue: "a_value_1",
				B: &B{
					BValue: "b_value_2",
					A: &A{
						AValue: "a_value_3",
						B: &B{
							BValue: "b_value_4",
						},
					},
				},
				C: &Chh{
					AMap: map[string]*A{
						"key": {
							AValue: "a_value_1",
							B: &B{
								BValue: "b_value_2",
							},
						},
					},
				},
			},
			B: &B{
				BValue: "b_value_1",
				A: &A{
					AValue: "a_value_2",
					B: &B{
						BValue: "b_value_3",
						A: &A{
							AValue: "a_value_4",
							B: &B{
								BValue: "b_value_5",
							},
						},
					},
				},
			},
		}
		assert.Loosely(t, Fields(msg, &OutputOnlyProcessor{}).Strings(), should.Resemble([]string{
			`.output: cleared OUTPUT_ONLY field`,
			`.int_map_inner[1].recursive.output_only: cleared OUTPUT_ONLY field`,
			`.int_map_inner[1].recursive.next.output_only: cleared OUTPUT_ONLY field`,
			`.int_map_inner[1].recursive.next.next.output_only: cleared OUTPUT_ONLY field`,
			`.output_inner: cleared OUTPUT_ONLY field`,
			`.struct: cleared OUTPUT_ONLY field`,
			`.a.b.b_value: cleared OUTPUT_ONLY field`,
			`.a.b.a.b.b_value: cleared OUTPUT_ONLY field`,
			`.a.c.a_map["key"].b.b_value: cleared OUTPUT_ONLY field`,
			`.b.b_value: cleared OUTPUT_ONLY field`,
			`.b.a.b.b_value: cleared OUTPUT_ONLY field`,
			`.b.a.b.a.b.b_value: cleared OUTPUT_ONLY field`,
		}))

		assert.Loosely(t, msg, should.Resemble(
			&Outer{
				IntMapInner: map[int32]*Inner{
					1: {Recursive: &Inner_Recursive{
						Regular: 1,
						Next: &Inner_Recursive{
							Regular: 2,
							Next: &Inner_Recursive{
								Regular: 3,
							},
						},
					}},
				},
				A: &A{
					AValue: "a_value_1",
					B: &B{
						A: &A{
							AValue: "a_value_3",
							B:      &B{},
						},
					},
					C: &Chh{
						AMap: map[string]*A{
							"key": {
								AValue: "a_value_1",
								B:      &B{},
							},
						},
					},
				},
				B: &B{
					A: &A{
						AValue: "a_value_2",
						B: &B{
							A: &A{
								AValue: "a_value_4",
								B:      &B{},
							},
						},
					},
				},
			},
		))
	})
}
