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

	Convey(`Works with *timestamp.Timestamp`, t, func() {
		var (
			varInt     int
			varReplace *tspb.Timestamp
		)
		ptrs := []interface{}{&varInt, &varReplace}

		subInds := replacePointers(ptrs)
		So(len(subInds), ShouldEqual, 1)
		val, ok := ptrs[1].(*time.Time)
		So(ok, ShouldBeTrue)
		*val = time.Unix(1000, 1234)

		replaceValues(ptrs, subInds)
		_, ok = ptrs[1].(**tspb.Timestamp)
		So(ok, ShouldBeTrue)
		So(varReplace, ShouldResembleProto, &tspb.Timestamp{Seconds: 1000, Nanos: 1234})
	})

	Convey(`Works with pb.Invocation_State`, t, func() {
		var (
			varInt     int
			varReplace pb.Invocation_State
		)
		ptrs := []interface{}{&varInt, &varReplace}

		subInds := replacePointers(ptrs)
		So(len(subInds), ShouldEqual, 1)
		val, ok := ptrs[1].(*int64)
		So(ok, ShouldBeTrue)
		*val = 2

		replaceValues(ptrs, subInds)
		_, ok = ptrs[1].(*pb.Invocation_State)
		So(ok, ShouldBeTrue)
		So(varReplace, ShouldEqual, pb.Invocation_COMPLETED)
	})
}
