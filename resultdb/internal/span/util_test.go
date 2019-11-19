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

package span

import (
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTypeConversion(t *testing.T) {
	t.Parallel()

	var b Buffer

	test := func(goValue, spValue interface{}) {
		// FromSpanner
		row, err := spanner.NewRow([]string{"a"}, []interface{}{spValue})
		So(err, ShouldBeNil)
		goPtr := reflect.New(reflect.TypeOf(goValue))
		err = b.FromSpanner(row, goPtr.Interface())
		So(err, ShouldBeNil)
		switch goValue.(type) {
		case proto.Message, []*pb.Artifact:
			So(goPtr.Elem().Interface(), ShouldResembleProto, goValue)
		default:
			So(goPtr.Elem().Interface(), ShouldResemble, goValue)
		}

		// ToSpanner
		actual := ToSpanner(goValue)
		So(actual, ShouldResemble, spValue)
	}

	Convey(`int64`, t, func() {
		test(int64(42), int64(42))
	})

	Convey(`*InvocationID`, t, func() {
		test(InvocationID("a"), "ca978112ca1bbdca:a")
	})

	Convey(`*[]InvocationID`, t, func() {
		test([]InvocationID{"a", "b"}, []string{"ca978112ca1bbdca:a", "3e23e8160039594a:b"})
	})

	Convey(`*timestamp.Timestamp`, t, func() {
		test(
			&tspb.Timestamp{Seconds: 1000, Nanos: 1234},
			spanner.NullTime{Valid: true, Time: time.Unix(1000, 1234).UTC()},
		)
	})

	Convey(`pb.Invocation_State`, t, func() {
		test(pb.Invocation_COMPLETED, int64(2))
	})

	Convey(`pb.TestStatus`, t, func() {
		test(pb.TestStatus_PASS, int64(1))
	})

	Convey(`*pb.Variant`, t, func() {
		test(
			pbutil.Variant("a", "1", "b", "2"),
			[]string{"a:1", "b:2"},
		)
	})

	Convey(`[]*pb.StringPair`, t, func() {
		test(
			pbutil.StringPairs("a", "1", "b", "2"),
			[]string{"a:1", "b:2"},
		)
	})

	Convey(`[]*pb.Artifact`, t, func() {
		arts := &Artifacts{
			Artifacts: []*pb.Artifact{
				{
					Name:        "traces/a.txt",
					FetchUrl:    "gs://a.txt",
					ContentType: "plain/text",
					Size:        4,
					Contents:    []byte("1234"),
				},
				{
					Name:        "diff/b.png",
					FetchUrl:    "isolate://b.png",
					ContentType: "image/png",
					Size:        16384,
				},
			},
		}
		expected, err := proto.Marshal(arts)
		So(err, ShouldBeNil)
		test(arts.Artifacts, expected)
	})

	Convey(`Slice`, t, func() {
		var varIntA, varIntB int64
		var varState pb.Invocation_State

		row, err := spanner.NewRow([]string{"a", "b", "c"}, []interface{}{int64(42), int64(56), int64(2)})
		So(err, ShouldBeNil)
		err = b.FromSpanner(row, &varIntA, &varIntB, &varState)
		So(err, ShouldBeNil)
		So(varIntA, ShouldEqual, 42)
		So(varIntB, ShouldEqual, 56)
		So(varState, ShouldEqual, pb.Invocation_COMPLETED)

		// ToSpanner
		spValues := ToSpannerSlice(varIntA, varIntB, varState)
		So(spValues, ShouldResemble, []interface{}{int64(42), int64(56), int64(2)})
	})
}
