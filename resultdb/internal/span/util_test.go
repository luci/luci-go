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

	tspb "github.com/golang/protobuf/ptypes/timestamp"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPointerSubstitution(t *testing.T) {
	t.Parallel()

	Convey(`Works with primitive (int)`, t, func() {
		var varInt int
		ptrs := []interface{}{&varInt}

		subPtrs := replacePointers(ptrs)
		val, ok := subPtrs[0].(*int)
		So(ok, ShouldBeTrue)
		*val = 42

		replaceValues(ptrs, subPtrs)
		_, ok = ptrs[0].(*int)
		So(ok, ShouldBeTrue)
		So(varInt, ShouldEqual, 42)
	})

	Convey(`Works with *timestamp.Timestamp`, t, func() {
		var varReplace *tspb.Timestamp
		ptrs := []interface{}{&varReplace}

		subPtrs := replacePointers(ptrs)
		val, ok := subPtrs[0].(*time.Time)
		So(ok, ShouldBeTrue)
		*val = time.Unix(1000, 1234)

		replaceValues(ptrs, subPtrs)
		_, ok = ptrs[0].(**tspb.Timestamp)
		So(ok, ShouldBeTrue)
		So(varReplace, ShouldResembleProto, &tspb.Timestamp{Seconds: 1000, Nanos: 1234})

		Convey(`panics with bad time`, func() {
			t := time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC)
			So(func() {
				replaceValues([]interface{}{&varReplace}, []interface{}{&t})
			}, ShouldPanic)
		})
	})

	Convey(`Works with pb.Invocation_State`, t, func() {
		var varReplace pb.Invocation_State
		ptrs := []interface{}{&varReplace}

		subPtrs := replacePointers(ptrs)
		val, ok := subPtrs[0].(*int64)
		So(ok, ShouldBeTrue)
		*val = 2

		replaceValues(ptrs, subPtrs)
		_, ok = ptrs[0].(*pb.Invocation_State)
		So(ok, ShouldBeTrue)
		So(varReplace, ShouldEqual, pb.Invocation_COMPLETED)
	})

	Convey(`Works with interspersed types`, t, func() {
		var varIntA, varIntB int
		var varState pb.Invocation_State
		ptrs := []interface{}{&varIntA, &varState, &varIntB}

		subPtrs := replacePointers(ptrs)
		val, ok := subPtrs[0].(*int)
		So(ok, ShouldBeTrue)
		*val = 45
		val64, ok := subPtrs[1].(*int64)
		So(ok, ShouldBeTrue)
		*val64 = 2
		val, ok = subPtrs[2].(*int)
		So(ok, ShouldBeTrue)
		*val = 56

		replaceValues(ptrs, subPtrs)
		So(varIntA, ShouldEqual, 45)
		So(varIntB, ShouldEqual, 56)
		So(varState, ShouldEqual, pb.Invocation_COMPLETED)
	})
}
