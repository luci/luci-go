// Copyright 2018 The LUCI Authors.
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

package bq

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq/testdata"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMetric(t *testing.T) {
	t.Parallel()
	u := Uploader{}
	u.UploadsMetricName = "fakeCounter"
	ftt.Run("Test metric creation", t, func(t *ftt.Test) {
		t.Run("Expect uploads metric was created", func(t *ftt.Test) {
			_ = u.getCounter() // To actually create the metric
			assert.Loosely(t, u.uploads.Info().Name, should.Equal("fakeCounter"))
		})
	})
}

func TestSave(t *testing.T) {
	ftt.Run("filled", t, func(t *ftt.Test) {
		recentTime := testclock.TestRecentTimeUTC
		ts := timestamppb.New(recentTime)

		r := &Row{
			Message: &testdata.TestMessage{
				Name:      "testname",
				Timestamp: ts,
				Nested: &testdata.NestedTestMessage{
					Name: "nestedname",
				},
				RepeatedNested: []*testdata.NestedTestMessage{
					{Name: "repeated_one"},
					{Name: "repeated_two"},
				},
				Foo: testdata.TestMessage_Y,
				FooRepeated: []testdata.TestMessage_FOO{
					testdata.TestMessage_Y,
					testdata.TestMessage_X,
				},

				Empty:    &emptypb.Empty{},
				Empties:  []*emptypb.Empty{{}, {}},
				Duration: durationpb.New(2*time.Second + 3*time.Millisecond),
				OneOf: &testdata.TestMessage_First{First: &testdata.NestedTestMessage{
					Name: "first",
				}},
				StringMap: map[string]string{
					"map_key_1": "map_value_1",
					"map_key_2": "map_value_2",
				},
				BqTypeOverride: 1234,
				StringEnumMap: map[string]testdata.TestMessage_FOO{
					"e_map_key_1": testdata.TestMessage_Y,
					"e_map_key_2": testdata.TestMessage_X,
				},
				StringDurationMap: map[string]*durationpb.Duration{
					"d_map_key_1": durationpb.New(1*time.Second + 1*time.Millisecond),
					"d_map_key_2": durationpb.New(2*time.Second + 2*time.Millisecond),
				},
				StringTimestampMap: map[string]*timestamppb.Timestamp{
					"t_map_key": ts,
				},
				StringProtoMap: map[string]*testdata.NestedTestMessage{
					"p_map_key": {Name: "nestedname"},
				},
			},
			InsertID: "testid",
		}
		row, id, err := r.Save()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal("testid"))
		assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
			"name":      "testname",
			"timestamp": recentTime,
			"nested":    map[string]bigquery.Value{"name": "nestedname"},
			"repeated_nested": []any{
				map[string]bigquery.Value{"name": "repeated_one"},
				map[string]bigquery.Value{"name": "repeated_two"},
			},
			"foo":          "Y",
			"foo_repeated": []any{"Y", "X"},
			"duration":     2.003,
			"first":        map[string]bigquery.Value{"name": "first"},
			"string_map": []any{
				map[string]bigquery.Value{"key": "map_key_1", "value": "map_value_1"},
				map[string]bigquery.Value{"key": "map_key_2", "value": "map_value_2"},
			},
			"bq_type_override": int64(1234),
			"string_enum_map": []any{
				map[string]bigquery.Value{"key": "e_map_key_1", "value": "Y"},
				map[string]bigquery.Value{"key": "e_map_key_2", "value": "X"},
			},
			"string_duration_map": []any{
				map[string]bigquery.Value{"key": "d_map_key_1", "value": 1.001},
				map[string]bigquery.Value{"key": "d_map_key_2", "value": 2.002},
			},
			"string_timestamp_map": []any{
				map[string]bigquery.Value{"key": "t_map_key", "value": recentTime},
			},
			"string_proto_map": []any{
				map[string]bigquery.Value{"key": "p_map_key", "value": map[string]bigquery.Value{"name": "nestedname"}},
			},
		}))
	})

	ftt.Run("known proto types", t, func(t *ftt.Test) {
		recentTime := testclock.TestRecentTimeUTC
		ts := timestamppb.New(recentTime)
		inputStruct := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"num": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
				"str": {Kind: &structpb.Value_StringValue{StringValue: "a"}},
			},
		}

		r := &Row{
			Message: &testdata.TestMessage{
				Name:      "testname",
				Timestamp: ts,
				Struct:    inputStruct,
				Duration:  durationpb.New(2*time.Second + 3*time.Millisecond),
			},
			InsertID: "testid",
		}
		row, id, err := r.Save()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal("testid"))

		assert.Loosely(t, row, should.ContainKey("name"))
		assert.Loosely(t, row, should.ContainKey("timestamp"))
		assert.Loosely(t, row, should.ContainKey("struct"))
		assert.Loosely(t, row, should.ContainKey("duration"))

		assert.Loosely(t, row["name"], should.Equal("testname"))
		assert.Loosely(t, row["timestamp"], should.Match(recentTime))
		assert.Loosely(t, row["duration"], should.Equal(2.003))

		outputStruct := &structpb.Struct{}
		bytesString, ok := row["struct"].(string)
		assert.Loosely(t, ok, should.BeTrue)
		err = outputStruct.UnmarshalJSON([]byte(bytesString))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, outputStruct, should.Match(inputStruct))
	})

	ftt.Run("empty", t, func(t *ftt.Test) {
		r := &Row{
			Message:  &testdata.TestMessage{},
			InsertID: "testid",
		}
		row, id, err := r.Save()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal("testid"))
		assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
			// only scalar proto fields
			// because for them, proto3 does not distinguish empty and unset
			// values.
			"bq_type_override": int64(0),
			"foo":              "X", // enums are always set
			"name":             "",  // in proto3, empty string and unset are indistinguishable
		}))
	})

	ftt.Run("empty optional enum", t, func(t *ftt.Test) {
		r := &Row{
			Message:  &testdata.TestOptionalEnumMessage{},
			InsertID: "testid",
		}
		row, id, err := r.Save()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal("testid"))
		assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
			// only scalar proto fields
			// because for them, proto3 does not distinguish empty and unset
			// values.
			"bar":  "Q", // enums are always set
			"name": "",  // in proto3, empty string and unset are indistinguishable
		}))
	})

	ftt.Run("non-existent enum", t, func(t *ftt.Test) {
		r := &Row{
			Message: &testdata.TestMessage{
				Foo: 777, // I'm feeling lucky
			},
			InsertID: "testid",
		}
		row, id, err := r.Save()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id, should.Equal("testid"))
		assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
			// only scalar proto fields
			// because for them, proto3 does not distinguish empty and unset
			// values.
			"bq_type_override": int64(0),
			"foo":              "X", // enums should be set to default
			"name":             "",  // in proto3, empty string and unset are indistinguishable
		}))
	})

	ftt.Run("repeated fields", t, func(t *ftt.Test) {
		t.Run("empty message", func(t *ftt.Test) {
			r := &Row{
				Message:  &testdata.TestRepeatedFieldMessage{},
				InsertID: "testid",
			}
			row, id, err := r.Save()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, id, should.Equal("testid"))
			assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
				// only scalar proto fields (ie. Name)
				"name": "",
			}))
		})

		t.Run("arrays of empty values", func(t *ftt.Test) {
			r := &Row{
				Message: &testdata.TestRepeatedFieldMessage{
					Name:    "",
					Strings: []string{"", ""},
					Bar:     []testdata.BAR{},
					Nested:  []*testdata.NestedTestMessage{{}, {}},
					Empties: []*emptypb.Empty{{}, {}},
				},
				InsertID: "testid",
			}
			row, id, err := r.Save()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, id, should.Equal("testid"))
			assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
				// only scalar proto fields (ie. strings)
				"name":    "",
				"strings": []any{"", ""},
				"nested": []any{
					map[string]bigquery.Value{"name": ""},
					map[string]bigquery.Value{"name": ""},
				},
			}))
		})

		t.Run("arrays of filled values", func(t *ftt.Test) {
			r := &Row{
				Message: &testdata.TestRepeatedFieldMessage{
					Name:    "Repeated Fields",
					Strings: []string{"string1", "string2"},
					Bar: []testdata.BAR{
						testdata.BAR_Q,
						testdata.BAR_R,
					},
					Nested: []*testdata.NestedTestMessage{
						{Name: "nested1"},
						{Name: "nested2"},
					},
					Empties: []*emptypb.Empty{{}, {}},
				},
				InsertID: "testid",
			}
			row, id, err := r.Save()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, id, should.Equal("testid"))
			assert.Loosely(t, row, should.Match(map[string]bigquery.Value{
				"name":    "Repeated Fields",
				"strings": []any{"string1", "string2"},
				"bar":     []any{"Q", "R"},
				"nested": []any{
					map[string]bigquery.Value{"name": "nested1"},
					map[string]bigquery.Value{"name": "nested2"},
				},
			}))
		})
	})
}

func TestBatch(t *testing.T) {
	t.Parallel()

	ftt.Run("Test batch", t, func(t *ftt.Test) {
		rowLimit := 2
		rows := make([]*Row, 3)
		for i := 0; i < 3; i++ {
			rows[i] = &Row{}
		}

		want := [][]*Row{
			{{}, {}},
			{{}},
		}
		rowSets := batch(rows, rowLimit)
		assert.Loosely(t, rowSets, should.HaveLength(len(want)))
		for i, set := range rowSets {
			targ := want[i]
			assert.Loosely(t, set, should.HaveLength(len(targ)))
			for j, itm := range set {
				assert.Loosely(t, itm.Message, should.Match(targ[j].Message),
					truth.Explain("want[%d][%d]", i, j))
				assert.Loosely(t, itm.InsertID, should.Match(targ[j].InsertID),
					truth.Explain("want[%d][%d]", i, j))
			}
		}
	})
}
