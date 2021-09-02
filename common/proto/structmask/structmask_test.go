// Copyright 2021 The LUCI Authors.
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

package structmask

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestStructMask(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mask   []*StructMask
		input  string
		output string
	}{
		{
			"noop",
			makeMask(),
			`{"a": "b"}`,
			`{"a": "b"}`,
		},

		{
			"all star 1",
			makeMask(`*`),
			`{}`,
			`{}`,
		},
		{
			"all star 2",
			makeMask(`*`, `*`),
			`{"a": "b", "c": {"d": ["e"]}}`,
			`{"a": "b", "c": {"d": ["e"]}}`,
		},

		{
			"field selector",
			makeMask(`a`),
			`{"a": "b", "c": {"d": ["e"]}}`,
			`{"a": "b"}`,
		},
		{
			"nested field selector",
			makeMask(`a.b`),
			`{
				"a": {"b": 1, "z": "..."},
				"b": 2,
				"c": {"b": 3}
			}`,
			`{"a": {"b": 1}}`,
		},

		{
			"dict star last",
			makeMask(`a.*`),
			`{
				"a": {"b": 1, "c": 2},
				"b": "zzz"
			}`,
			`{
				"a": {"b": 1, "c": 2}
			}`,
		},
		{
			"dict star not last",
			makeMask(`*.b`),
			`{
				"a": {"b": 1, "c": 2},
				"b": {"z": 123},
				"c": 123,
				"d": []
			}`,
			`{
				"a": {"b": 1}
			}`,
		},
		{
			"dict star nested",
			makeMask(`*.b.*`),
			`{
				"f1": 1,
				"f2": {"z": []},
				"f3": {"b": [1, 2, 3]},
				"f4": {"b": 123}
			}`,
			`{
				"f3": {"b": [1, 2, 3]}
			}`,
		},

		{
			"list star last",
			makeMask(`a.*`),
			`{
				"a": [{"a": "b"}, 2, {"a": "b"}],
				"b": "zzz"
			}`,
			`{
				"a": [{"a": "b"}, 2, {"a": "b"}]
			}`,
		},
		{
			"list star not last",
			makeMask(`a.*.b`),
			`{
				"a": [{"b": "c"}, 2, {"a": "c"}, null],
				"b": "zzz"
			}`,
			`{
				"a": [{"b": "c"}, null, null, null]
			}`,
		},
		{
			"list star nested",
			makeMask(`a.*.*`),
			`{
				"a": [
					"skip",
					null,
					123,
					{"a": "b"},
					{"a": {"b": "c"}},
					[1, 2, 3]
				]
			}`,
			`{
				"a": [
					null,
					null,
					null,
					{"a": "b"},
					{"a": {"b": "c"}},
					[1, 2, 3]
				]
			}`,
		},

		{
			"multiple field selectors, no stars",
			makeMask(`a.a`, `a.b`, `b`),
			`{
				"a": {
					"a": 1,
					"b": 2,
					"c": 3
				},
				"b": [1, 2, 3],
				"c": 5
			}`,
			`{
				"a": {
					"a": 1,
					"b": 2
				},
				"b": [1, 2, 3]
			}`,
		},

		{
			"early leaf nodes, fields",
			makeMask(`a.b.c`, `a.b`),
			`{
				"a": {
					"b": {"c": 1, "d": 2, "e": {"f": 3}},
					"c": 2
				}
			}`,
			`{
				"a": {
					"b": {"c": 1, "d": 2, "e": {"f": 3}}
				}
			}`,
		},
		{
			"early leaf nodes, fields, reversed",
			makeMask(`a.b`, `a.b.c`),
			`{
				"a": {
					"b": {"c": 1, "d": 2, "e": {"f": 3}},
					"c": 2
				}
			}`,
			`{
				"a": {
					"b": {"c": 1, "d": 2, "e": {"f": 3}}
				}
			}`,
		},

		{
			"early leaf nodes, stars",
			makeMask(`a.*.c`, `a.*`),
			`{
				"a": {
					"b": {"c": 1, "d": 2, "e": {"f": 3}},
					"c": 2
				}
			}`,
			`{
				"a": {
					"b": {"c": 1, "d": 2, "e": {"f": 3}},
					"c": 2
				}
			}`,
		},

		{
			"star + field selector merging, simple",
			makeMask(`*.a`, `a.b`),
			`{
				"a": {"a": 1, "b": 2, "c": 3},
				"b": {"a": 1, "b": 2, "c": 3},
				"c": 123
			}`,
			`{
				"a": {"a": 1, "b": 2},
				"b": {"a": 1}
			}`,
		},
		{
			"star + field selector merging, deeper",
			makeMask(`*.a.a`, `a.a.b`),
			`{
				"a": {"a": {"a": 1, "b": 2}},
				"b": {"a": {"a": 1, "b": 2}}
			}`,
			`{
				"a": {"a": {"a": 1, "b": 2}},
				"b": {"a": {"a": 1}}
			}`,
		},
		{
			"star + field selector merging, deeper, reverse order",
			makeMask(`a.a.b`, `*.a.a`),
			`{
				"a": {"a": {"a": 1, "b": 2}},
				"b": {"a": {"a": 1, "b": 2}}
			}`,
			`{
				"a": {"a": {"a": 1, "b": 2}},
				"b": {"a": {"a": 1}}
			}`,
		},

		{
			"list merges, simple",
			makeMask(`*.*.a`, `a.*.b`),
			`{
				"a": [
					{"a": 1, "b": 1},
					{"b": 1},
					{"c": 1}
				],
				"b": [
					{"a": 1, "b": 1},
					{"b": 1},
					{"c": 1}
				]
			}`,
			`{
				"a": [
					{"a": 1, "b": 1},
					{"b": 1},
					null
				],
				"b": [
					{"a": 1},
					null,
					null
				]
			}`,
		},
		{
			"list merges with nulls",
			makeMask(`a.*.a`, `*.*.b`),
			`{
				"a": [
					{"b": 1, "c": 1},
					{"a": 1, "c": 1},
					{"c": 1}
				]
			}`,
			`{
				"a": [
					{"b": 1},
					{"a": 1},
					null
				]
			}`,
		},
		{
			"list merges with nulls, deeper",
			makeMask(`*.*.*.b`, `x.*.a.c`),
			`{
				"x": [
					{"a": {"c": 1}},
					{"y": {"b": 1}}
				]
			}`,
			`{
				"x": [
					{"a": {"c": 1}},
					{"y": {"b": 1}}
				]
			}`,
		},

		{
			"merging exact same values",
			makeMask(`a.*`, `*.b`),
			`{"a": {"b": {"c": 1}}}`,
			`{"a": {"b": {"c": 1}}}`,
		},

		{
			"merging scalars into nils",
			makeMask(`a.*.a`, `*.*`),
			`{
				"a": [
					{"b": 1},
					1
				]
			}`,
			`{
				"a": [
					{"b": 1},
					1
				]
			}`,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			filter, err := NewFilter(cs.mask)
			if err != nil {
				t.Errorf("bad filter: %s", err)
				return
			}
			expected := asProto(cs.output)
			output := filter.Apply(asProto(cs.input))
			if !proto.Equal(output, expected) {
				t.Errorf("got:\n---------\n%s\n---------\nbut want\n---------\n%s\n---------", asJSON(output), asJSON(expected))
			}
		})
	}
}

func makeMask(masks ...string) []*StructMask {
	out := make([]*StructMask, len(masks))
	for idx, m := range masks {
		out[idx] = &StructMask{Path: strings.Split(m, ".")}
	}
	return out
}

func asProto(json string) *structpb.Struct {
	s := &structpb.Struct{}
	if err := protojson.Unmarshal([]byte(json), s); err != nil {
		panic(err)
	}
	return s
}

func asJSON(s *structpb.Struct) string {
	blob, err := (protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}).Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(blob)
}
