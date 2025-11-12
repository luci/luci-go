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

package spanutil

import (
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestTypeConversion(t *testing.T) {
	t.Parallel()

	var b Buffer

	test := func(goValue, spValue any) {
		// ToSpanner
		actualSPValue := ToSpanner(goValue)
		assert.Loosely(t, actualSPValue, should.Match(spValue))

		// FromSpanner
		row, err := spanner.NewRow([]string{"a"}, []any{actualSPValue})
		assert.Loosely(t, err, should.BeNil)
		goPtr := reflect.New(reflect.TypeOf(goValue))
		err = b.FromSpanner(row, goPtr.Interface())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, goPtr.Elem().Interface(), should.Match(goValue))
	}

	ftt.Run(`int64`, t, func(t *ftt.Test) {
		test(int64(42), int64(42))
	})

	ftt.Run(`*timestamppb.Timestamp`, t, func(t *ftt.Test) {
		test(
			&timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
			spanner.NullTime{Valid: true, Time: time.Unix(1000, 1234).UTC()},
		)
	})

	ftt.Run(`pb.ExonerationReason`, t, func(t *ftt.Test) {
		test(pb.ExonerationReason_OCCURS_ON_OTHER_CLS, int64(2))
	})

	ftt.Run(`pb.Invocation_State`, t, func(t *ftt.Test) {
		test(pb.Invocation_FINALIZED, int64(3))
	})

	ftt.Run(`pb.TestStatus`, t, func(t *ftt.Test) {
		test(pb.TestStatus_PASS, int64(1))
	})

	ftt.Run(`*pb.Variant`, t, func(t *ftt.Test) {
		t.Run(`Works`, func(t *ftt.Test) {
			test(
				pbutil.Variant("a", "1", "b", "2"),
				[]string{"a:1", "b:2"},
			)
		})
		t.Run(`Empty`, func(t *ftt.Test) {
			test(
				&pb.Variant{},
				[]string{},
			)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			// Nil does not roundtrip as its stored representatio is the same
			// as the empty variant.
			actualSPValue := ToSpanner((*pb.Variant)(nil))
			assert.Loosely(t, actualSPValue, should.Match([]string{}))
		})
	})

	ftt.Run(`*pb.RootInvocationDefinition_Properties`, t, func(t *ftt.Test) {
		t.Run(`Works`, func(t *ftt.Test) {
			test(
				&pb.RootInvocationDefinition_Properties{
					Def: map[string]string{
						"a": "1",
						"b": "2",
					},
				},
				[]string{"a:1", "b:2"},
			)
		})
		t.Run(`Empty`, func(t *ftt.Test) {
			test(
				&pb.RootInvocationDefinition_Properties{},
				[]string{},
			)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			// Nil does not roundtrip as its stored representation is the same
			// as the empty variant.
			actualSPValue := ToSpanner((*pb.RootInvocationDefinition_Properties)(nil))
			assert.Loosely(t, actualSPValue, should.Match([]string{}))
		})
	})

	ftt.Run(`[]*pb.StringPair`, t, func(t *ftt.Test) {
		test(
			pbutil.StringPairs("a", "1", "b", "2"),
			[]string{"a:1", "b:2"},
		)
	})

	ftt.Run(`Compressed`, t, func(t *ftt.Test) {
		t.Run(`Empty`, func(t *ftt.Test) {
			test(Compressed(nil), []byte(nil))
		})
		t.Run(`non-Empty`, func(t *ftt.Test) {
			test(
				Compressed("aaaaaaaaaaaaaaaaaaaa"),
				[]byte{122, 116, 100, 10, 40, 181, 47, 253, 4, 0, 163, 0, 0, 97, 247, 175, 71, 227})
		})
	})

	ftt.Run(`Slice`, t, func(t *ftt.Test) {
		var varIntA, varIntB int64
		var varState pb.Invocation_State

		row, err := spanner.NewRow([]string{"a", "b", "c"}, []any{int64(42), int64(56), int64(3)})
		assert.Loosely(t, err, should.BeNil)
		err = b.FromSpanner(row, &varIntA, &varIntB, &varState)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, varIntA, should.Equal(42))
		assert.Loosely(t, varIntB, should.Equal(56))
		assert.Loosely(t, varState, should.Equal(pb.Invocation_FINALIZED))

		// ToSpanner
		spValues := ToSpannerSlice(varIntA, varIntB, varState)
		assert.Loosely(t, spValues, should.Match([]any{int64(42), int64(56), int64(3)}))
	})

	ftt.Run(`[]*pb.BigQueryExport`, t, func(t *ftt.Test) {
		bqExports := []*pb.BigQueryExport{
			{
				Project: "project",
				Dataset: "dataset",
				Table:   "table1",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			},
			{
				Project: "project",
				Dataset: "dataset",
				Table:   "table2",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			},
		}

		var err error
		expectedBqExportsBytes := make([][]byte, len(bqExports))
		for i, bqExport := range bqExports {
			expectedBqExportsBytes[i], err = proto.Marshal(bqExport)
			assert.Loosely(t, err, should.BeNil)
		}
		assert.Loosely(t, ToSpanner(bqExports), should.Match(expectedBqExportsBytes))

		row, err := spanner.NewRow([]string{"a"}, []any{expectedBqExportsBytes})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`success`, func(t *ftt.Test) {
			expectedPtr := []*pb.BigQueryExport{}
			err = b.FromSpanner(row, &expectedPtr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expectedPtr, should.Match(bqExports))
		})
	})

	ftt.Run(`proto.Message`, t, func(t *ftt.Test) {
		msg := &pb.Invocation{
			Name: "a",
		}
		expected, err := proto.Marshal(msg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ToSpanner(msg), should.Match(expected))

		row, err := spanner.NewRow([]string{"a"}, []any{expected})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`success`, func(t *ftt.Test) {
			expectedPtr := &pb.Invocation{}
			err = b.FromSpanner(row, expectedPtr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expectedPtr, should.Match(msg))
		})

		t.Run(`Passing nil pointer to fromSpanner`, func(t *ftt.Test) {
			var expectedPtr *pb.Invocation
			err = b.FromSpanner(row, expectedPtr)
			assert.Loosely(t, err, should.ErrLike("nil pointer encountered"))
		})
	})
}
