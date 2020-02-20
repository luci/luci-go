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

		successfulBuild := &buildbucketpb.Build{Status: success}
		failedBuild := &buildbucketpb.Build{Status: failure}
		infraFailedBuild := &buildbucketpb.Build{Status: infraFailure}

		Convey("Success", func() {
			n.OnOccurrence = append(n.OnOccurrence, success)

			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, successfulBuild), ShouldBeTrue)
		})

		Convey("Failure", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure)

			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
		})

		Convey("InfraFailure", func() {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)

			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
		})

		Convey("Failure and InfraFailure", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure, infraFailure)

			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
		})

		Convey("New Failure", func() {
			n.OnNewStatus = append(n.OnNewStatus, failure)

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("New InfraFailure", func() {
			n.OnNewStatus = append(n.OnNewStatus, infraFailure)

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("New Failure and new InfraFailure", func() {
			n.OnNewStatus = append(n.OnNewStatus, failure, infraFailure)

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("InfraFailure and new Failure and new Success", func() {
			n.OnOccurrence = append(n.OnOccurrence, infraFailure)
			n.OnNewStatus = append(n.OnNewStatus, failure, success)

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeTrue)
		})

		Convey("Failure with step regex", func() {
			n.OnOccurrence = append(n.OnOccurrence, failure)
			n.FailedStepRegexp = "yes"
			n.FailedStepRegexpExclude = "no"

			So(n.ShouldNotify(success, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "yes",
						Status: failure,
					},
				},
			}), ShouldBeTrue)
			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "yes",
						Status: success,
					},
				},
			}), ShouldBeFalse)

			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "no",
						Status: failure,
					},
				},
			}), ShouldBeFalse)
			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "yes",
						Status: success,
					},
					&buildbucketpb.Step{
						Name: "no",
						Status: failure,
					},
				},
			}), ShouldBeFalse)
			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "yes",
						Status: failure,
					},
					&buildbucketpb.Step{
						Name: "no",
						Status: failure,
					},
				},
			}), ShouldBeTrue)
			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "yesno",
						Status: failure,
					},
				},
			}), ShouldBeFalse)
			So(n.ShouldNotify(success, &buildbucketpb.Build{
				Status: failure,
				Steps: []*buildbucketpb.Step{
					&buildbucketpb.Step{
						Name: "yesno",
						Status: failure,
					},
					&buildbucketpb.Step{
						Name: "yes",
						Status: failure,
					},
				},
			}), ShouldBeTrue)
		})

		Convey("OnSuccess deprecated", func() {
			n.OnSuccess = true

			So(n.ShouldNotify(success, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("OnFailure deprecated", func() {
			n.OnFailure = true

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("OnChange deprecated", func() {
			n.OnChange = true

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})

		Convey("OnNewFailure deprecated", func() {
			n.OnNewFailure = true

			So(n.ShouldNotify(success, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(success, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(success, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, failedBuild), ShouldBeFalse)
			So(n.ShouldNotify(failure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(infraFailure, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(infraFailure, infraFailedBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, successfulBuild), ShouldBeFalse)
			So(n.ShouldNotify(unspecified, failedBuild), ShouldBeTrue)
			So(n.ShouldNotify(unspecified, infraFailedBuild), ShouldBeFalse)
		})
	})
}
