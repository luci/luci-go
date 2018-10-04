// Copyright 2018 The LUCI Authors.
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

package config

import (
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotification(t *testing.T) {
	t.Parallel()

	Convey("ShouldNotify", t, func() {
		n := Notification{}

		const (
			unspecified  = buildbucketpb.Status_STATUS_UNSPECIFIED
			success      = buildbucketpb.Status_SUCCESS
			failure      = buildbucketpb.Status_FAILURE
			infraFailure = buildbucketpb.Status_INFRA_FAILURE
		)

		Convey("OnSuccess", func() {
			n.OnSuccess = true
			So(n.ShouldNotify(unspecified, success), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
		})

		Convey("OnFailure", func() {
			n.OnFailure = true
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("OnNewFailure", func() {
			n.OnNewFailure = true
			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeTrue)
		})

		Convey("OnChange", func() {
			n.OnChange = true
			So(n.ShouldNotify(failure, success), ShouldBeTrue)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
		})
	})
}
