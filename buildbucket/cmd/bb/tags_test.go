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

package main

import (
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStringPairSliceFlag(t *testing.T) {
	t.Parallel()

	Convey("StringPairSliceFlag", t, func() {
		var flag stringPairSliceFlag

		Convey("one", func() {
			So(flag.Set("a:1"), ShouldBeNil)
			So(flag.Get(), ShouldResembleProto, []*buildbucketpb.StringPair{
				{Key: "a", Value: "1"},
			})
			So(flag.String(), ShouldEqual, "a:1")
		})

		Convey("two", func() {
			So(flag.Set("a:1"), ShouldBeNil)
			So(flag.Set("b:2"), ShouldBeNil)
			So(flag.Get(), ShouldResembleProto, []*buildbucketpb.StringPair{
				{Key: "a", Value: "1"},
				{Key: "b", Value: "2"},
			})
			So(flag.String(), ShouldEqual, "a:1, b:2")
		})

		Convey("no colon", func() {
			So(flag.Set("a"), ShouldErrLike, "a tag must have a colon")
		})
	})
}
