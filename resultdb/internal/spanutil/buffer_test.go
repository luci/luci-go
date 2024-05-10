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
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestTypeConversion(t *testing.T) {
	t.Parallel()

	var b Buffer

	test := func(goValue, spValue any) {
		// ToSpanner
		actualSPValue := ToSpanner(goValue)
		So(actualSPValue, ShouldResemble, spValue)

		// FromSpanner
		row, err := spanner.NewRow([]string{"a"}, []any{actualSPValue})
		So(err, ShouldBeNil)
		goPtr := reflect.New(reflect.TypeOf(goValue))
		err = b.FromSpanner(row, goPtr.Interface())
		So(err, ShouldBeNil)
		So(goPtr.Elem().Interface(), ShouldResemble, goValue)
	}

	Convey(`int64`, t, func() {
		test(int64(42), int64(42))
	})

	Convey(`*timestamppb.Timestamp`, t, func() {
		test(
			&timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
			spanner.NullTime{Valid: true, Time: time.Unix(1000, 1234).UTC()},
		)
	})

	Convey(`pb.ExonerationReason`, t, func() {
		test(pb.ExonerationReason_OCCURS_ON_OTHER_CLS, int64(2))
	})

	Convey(`pb.Invocation_State`, t, func() {
		test(pb.Invocation_FINALIZED, int64(3))
	})

	Convey(`pb.TestStatus`, t, func() {
		test(pb.TestStatus_PASS, int64(1))
	})

	Convey(`*pb.Variant`, t, func() {
		Convey(`Works`, func() {
			test(
				pbutil.Variant("a", "1", "b", "2"),
				[]string{"a:1", "b:2"},
			)
		})
		Convey(`Empty`, func() {
			test(
				(*pb.Variant)(nil),
				[]string{},
			)
		})
	})

	Convey(`[]*pb.StringPair`, t, func() {
		test(
			pbutil.StringPairs("a", "1", "b", "2"),
			[]string{"a:1", "b:2"},
		)
	})

	Convey(`Compressed`, t, func() {
		Convey(`Empty`, func() {
			test(Compressed(nil), []byte(nil))
		})
		Convey(`non-Empty`, func() {
			test(
				Compressed("aaaaaaaaaaaaaaaaaaaa"),
				[]byte{122, 116, 100, 10, 40, 181, 47, 253, 4, 0, 163, 0, 0, 97, 247, 175, 71, 227})
		})
	})

	Convey(`Slice`, t, func() {
		var varIntA, varIntB int64
		var varState pb.Invocation_State

		row, err := spanner.NewRow([]string{"a", "b", "c"}, []any{int64(42), int64(56), int64(3)})
		So(err, ShouldBeNil)
		err = b.FromSpanner(row, &varIntA, &varIntB, &varState)
		So(err, ShouldBeNil)
		So(varIntA, ShouldEqual, 42)
		So(varIntB, ShouldEqual, 56)
		So(varState, ShouldEqual, pb.Invocation_FINALIZED)

		// ToSpanner
		spValues := ToSpannerSlice(varIntA, varIntB, varState)
		So(spValues, ShouldResemble, []any{int64(42), int64(56), int64(3)})
	})

	Convey(`[]*pb.BigQueryExport`, t, func() {
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
			So(err, ShouldBeNil)
		}
		So(ToSpanner(bqExports), ShouldResemble, expectedBqExportsBytes)

		row, err := spanner.NewRow([]string{"a"}, []any{expectedBqExportsBytes})
		So(err, ShouldBeNil)

		Convey(`success`, func() {
			expectedPtr := []*pb.BigQueryExport{}
			err = b.FromSpanner(row, &expectedPtr)
			So(err, ShouldBeNil)
			So(expectedPtr, ShouldResembleProto, bqExports)
		})
	})

	Convey(`proto.Message`, t, func() {
		msg := &pb.Invocation{
			Name: "a",
		}
		expected, err := proto.Marshal(msg)
		So(err, ShouldBeNil)
		So(ToSpanner(msg), ShouldResemble, expected)

		row, err := spanner.NewRow([]string{"a"}, []any{expected})
		So(err, ShouldBeNil)

		Convey(`success`, func() {
			expectedPtr := &pb.Invocation{}
			err = b.FromSpanner(row, expectedPtr)
			So(err, ShouldBeNil)
			So(expectedPtr, ShouldResembleProto, msg)
		})

		Convey(`Passing nil pointer to fromSpanner`, func() {
			var expectedPtr *pb.Invocation
			err = b.FromSpanner(row, expectedPtr)
			So(err, ShouldErrLike, "nil pointer encountered")
		})
	})
}
