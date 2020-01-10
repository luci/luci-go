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

package pbutil

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInvocationName(t *testing.T) {
	t.Parallel()
	Convey("ParseInvocationName", t, func() {
		Convey("Parse", func() {
			id, err := ParseInvocationName("invocations/a")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, "a")
		})

		Convey("Invalid", func() {
			_, err := ParseInvocationName("invocations/-")
			So(err, ShouldErrLike, `does not match`)
		})

		Convey("Format", func() {
			So(InvocationName("a"), ShouldEqual, "invocations/a")
		})

	})
}

func TestInvocationUtils(t *testing.T) {
	t.Parallel()
	Convey(`Normalization works`, t, func() {
		inv := &pb.Invocation{
			Tags: StringPairs(
				"k2", "v21",
				"k2", "v20",
				"k3", "v30",
				"k1", "v1",
				"k3", "v31",
			),
		}

		NormalizeInvocation(inv)

		So(inv.Tags, ShouldResembleProto, StringPairs(
			"k1", "v1",
			"k2", "v20",
			"k2", "v21",
			"k3", "v30",
			"k3", "v31",
		))
	})
}
