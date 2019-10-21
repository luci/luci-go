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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestColumnReader(t *testing.T) {
	t.Parallel()

	read := func(dest interface{}, value interface{}) {
		row, err := spanner.NewRow([]string{"a"}, []interface{}{value})
		So(err, ShouldBeNil)
		err = NewColumnReader(dest).Read(row)
		So(err, ShouldBeNil)
	}

	Convey(`int64`, t, func() {
		var v int64
		read(&v, 42)
		So(v, ShouldEqual, int64(42))
	})

	Convey(`*string`, t, func() {
		var v string
		Convey(`NOT NULL`, func() {
			read(&v, "sis boom bah")
			So(v, ShouldEqual, "sis boom bah")
		})

		Convey(`valid NULLable`, func() {
			read(&v, spanner.NullString{StringVal: "sis boom bah", Valid: true})
			So(v, ShouldEqual, "sis boom bah")
		})

		Convey(`NULL`, func() {
			read(&v, spanner.NullString{Valid: false})
			So(v, ShouldEqual, "")
		})
	})

	Convey(`*timestamp.Timestamp`, t, func() {
		var v *tspb.Timestamp
		Convey(`NOT NULL`, func() {
			read(&v, time.Unix(1000, 1234))
			So(v, ShouldResembleProto, &tspb.Timestamp{Seconds: 1000, Nanos: 1234})
		})

		Convey(`valid NULLable`, func() {
			read(&v, spanner.NullTime{Time: time.Unix(1000, 1234), Valid: true})
			So(v, ShouldResembleProto, &tspb.Timestamp{Seconds: 1000, Nanos: 1234})
		})

		Convey(`NULL`, func() {
			read(&v, spanner.NullTime{Valid: false})
			So(v, ShouldBeNil)
		})
	})

	Convey(`pb.Invocation_State`, t, func() {
		var v pb.Invocation_State
		read(&v, 2)
		So(v, ShouldEqual, pb.Invocation_COMPLETED)
	})

	Convey(`*pb.VariantDef`, t, func() {
		var v *pb.VariantDef
		read(&v, []string{"k1:v1", "key/k2:v2", "key/with/part/k3:v3"})
		So(v, ShouldResembleProto, &pb.VariantDef{Def: map[string]string{
			"k1":               "v1",
			"key/k2":           "v2",
			"key/with/part/k3": "v3",
		}})
	})

	Convey(`[]*pb.StringPair`, t, func() {
		var v []*pb.StringPair
		read(&v, []string{"k1:v1", "key/k2:v2", "key/with/part/k3:v3"})
		So(v, ShouldResemble, pbutil.StringPairs(
			"k1", "v1",
			"key/k2", "v2",
			"key/with/part/k3", "v3",
		))
	})

	Convey(`interspersed types`, t, func() {
		var varIntA, varIntB int64
		var varState pb.Invocation_State

		row, err := spanner.NewRow([]string{"a", "b", "c"}, []interface{}{int64(42), int64(56), int64(2)})
		So(err, ShouldBeNil)

		err = NewColumnReader(&varIntA, &varIntB, &varState).Read(row)
		So(err, ShouldBeNil)
		So(varIntA, ShouldEqual, 42)
		So(varIntB, ShouldEqual, 56)
		So(varState, ShouldEqual, pb.Invocation_COMPLETED)
	})
}
