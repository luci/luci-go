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
		n.OnOccurrence = []buildbucketpb.Status{}
		n.OnNewStatus = []buildbucketpb.Status{}

		const (
			unspecified  = buildbucketpb.Status_STATUS_UNSPECIFIED
			success      = buildbucketpb.Status_SUCCESS
			failure      = buildbucketpb.Status_FAILURE
			infraFailure = buildbucketpb.Status_INFRA_FAILURE
		)

		Convey("Success", func() {
			n.OnOccurrence = append(n.OnOccurrence, success)

			So(n.ShouldNotify(unspecified, success), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(success, success), ShouldBeTrue)
		})

		Convey("Failure", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure)

			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(success, success), ShouldBeFalse)
		})

		Convey("InfraFailure", func() {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)

			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(success, success), ShouldBeFalse)
		})

		Convey("Failure and InfraFailure", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure, infraFailure)

			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(failure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(success, success), ShouldBeFalse)
		})

		Convey("New Failure", func() {
			n.OnNewStatus = append(n.OnNewStatus, failure)

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(failure, success), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, success), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("New InfraFailure", func() {
			n.OnNewStatus = append(n.OnNewStatus, infraFailure)

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeFalse)
			So(n.ShouldNotify(success, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(failure, success), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, success), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("New Failure and new InfraFailure", func() {
			n.OnNewStatus = append(n.OnNewStatus, failure, infraFailure)

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(failure, success), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, success), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("InfraFailure and new Failure and new Success", func() {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)
			n.OnNewStatus = append(n.OnNewStatus, failure, success)

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(failure, success), ShouldBeTrue)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, success), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeTrue)
		})

		Convey("OnSuccess deprecated", func() {
			n.OnSuccess = true

			So(n.ShouldNotify(success, success), ShouldBeTrue)
			So(n.ShouldNotify(success, failure), ShouldBeFalse)
			So(n.ShouldNotify(success, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(failure, success), ShouldBeTrue)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, success), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("OnFailure deprecated", func() {
			n.OnFailure = true

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(failure, success), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeTrue)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, success), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("OnChange deprecated", func() {
			n.OnChange = true

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(failure, success), ShouldBeTrue)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, success), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})

		Convey("OnNewFailure deprecated", func() {
			n.OnNewFailure = true

			So(n.ShouldNotify(success, success), ShouldBeFalse)
			So(n.ShouldNotify(success, failure), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(failure, success), ShouldBeFalse)
			So(n.ShouldNotify(failure, failure), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, success), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failure), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailure), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, success), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failure), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailure), ShouldBeFalse)
		})
	})
}
