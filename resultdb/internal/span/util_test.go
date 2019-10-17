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

	Convey(`*timestamp.Timestamp`, t, func() {
		var v *tspb.Timestamp
		read(&v, time.Unix(1000, 1234))
		So(v, ShouldResembleProto, &tspb.Timestamp{Seconds: 1000, Nanos: 1234})
	})

	Convey(`Works with pb.Invocation_State`, t, func() {
		var v pb.Invocation_State
		read(&v, 2)
		So(v, ShouldEqual, pb.Invocation_COMPLETED)
	})

	Convey(`Works with interspersed types`, t, func() {
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
