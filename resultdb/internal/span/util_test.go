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

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTypeConversion(t *testing.T) {
	t.Parallel()

	var b Buffer

	test := func(goValue, spValue interface{}) {
		// ToSpanner
		actualSPValue := ToSpanner(goValue)
		So(actualSPValue, ShouldResemble, spValue)

		// FromSpanner
		row, err := spanner.NewRow([]string{"a"}, []interface{}{actualSPValue})
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
	}

	Convey(`int64`, t, func() {
		test(int64(42), int64(42))
	})

	Convey(`*InvocationID`, t, func() {
		test(InvocationID("a"), "ca978112ca1bbdca:a")
	})

	Convey(`*InvocationIDSet`, t, func() {
		test(NewInvocationIDSet("a", "b"), []string{"3e23e8160039594a:b", "ca978112ca1bbdca:a"})
	})

	Convey(`*timestamp.Timestamp`, t, func() {
		test(
			&tspb.Timestamp{Seconds: 1000, Nanos: 1234},
			spanner.NullTime{Valid: true, Time: time.Unix(1000, 1234).UTC()},
		)
	})

	Convey(`pb.Invocation_State`, t, func() {
		test(pb.Invocation_FINALIZED, int64(3))
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
		arts := &internalpb.Artifacts{
			ArtifactsV1: []*pb.Artifact{
				{
					Name:        "traces/a.txt",
					FetchUrl:    "gs://a.txt",
					ContentType: "text/plain",
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
		test(arts.ArtifactsV1, expected)
	})

	Convey(`Compressed`, t, func() {
		Convey(`Empty`, func() {
			test(Compressed(nil), []byte(nil))
		})
		Convey(`non-Empty`, func() {
			test(
				Compressed("aaaaaaaaaaaaaaaaaaaa"),
				[]byte{122, 116, 100, 10, 40, 181, 47, 253, 4, 0, 69, 0, 0, 8, 97, 1, 84, 1, 2, 16, 4, 247, 175, 71, 227})
		})
	})

	Convey(`CompressedProto`, t, func() {
		Convey(`Empty`, func() {
			var msg *pb.Invocation
			actualSPValue := ToSpanner(CompressedProto{msg})
			So(actualSPValue, ShouldBeNil)
		})
		Convey(`non-Empty`, func() {
			msg := &pb.Invocation{
				Name: "a",
			}
			actualSPValue := ToSpanner(CompressedProto{msg})
			So(actualSPValue, ShouldResemble, []byte{122, 116, 100, 10, 40, 181, 47, 253, 4, 0, 25, 0, 0, 10, 1, 97, 135, 76, 200, 148})

			// FromSpanner
			row, err := spanner.NewRow([]string{"a"}, []interface{}{actualSPValue})
			So(err, ShouldBeNil)
			dest := &CompressedProto{&pb.Invocation{}}
			err = b.FromSpanner(row, dest)
			So(err, ShouldBeNil)
			So(dest.Message, ShouldResembleProto, msg)
		})
	})

	Convey(`Slice`, t, func() {
		var varIntA, varIntB int64
		var varState pb.Invocation_State

		row, err := spanner.NewRow([]string{"a", "b", "c"}, []interface{}{int64(42), int64(56), int64(3)})
		So(err, ShouldBeNil)
		err = b.FromSpanner(row, &varIntA, &varIntB, &varState)
		So(err, ShouldBeNil)
		So(varIntA, ShouldEqual, 42)
		So(varIntB, ShouldEqual, 56)
		So(varState, ShouldEqual, pb.Invocation_FINALIZED)

		// ToSpanner
		spValues := ToSpannerSlice(varIntA, varIntB, varState)
		So(spValues, ShouldResemble, []interface{}{int64(42), int64(56), int64(3)})
	})

	Convey(`proto.Message`, t, func() {
		msg := &pb.Invocation{
			Name: "a",
		}
		expected, err := proto.Marshal(msg)
		So(err, ShouldBeNil)
		So(ToSpanner(msg), ShouldResemble, expected)

		row, err := spanner.NewRow([]string{"a"}, []interface{}{expected})
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
