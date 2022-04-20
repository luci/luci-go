// Copyright 2021 The LUCI Authors.
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

package bbfacade

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestToTryjobStatusAndResult(t *testing.T) {
	Convey("ToTryjobStatusAndResult", t, func() {
		b := &bbpb.Build{}
		So(protojson.Unmarshal([]byte(`{
			"id": "8831742013603886929",
			"createTime": "2021-11-01T18:31:34Z",
			"updateTime": "2021-11-01T18:32:01Z",
			"status": "SUCCESS"
		}`), b), ShouldBeNil)

		var status tryjob.Status
		var result *tryjob.Result
		var err error

		ctx := context.Background()

		Convey("Returns an error", func() {
			Convey("On an invalid build status", func() {
				b.Status = bbpb.Status_ENDED_MASK
				_, _, err := toTryjobStatusAndResult(ctx, b)
				So(err, ShouldErrLike, "unexpected buildbucket status")
			})
		})
		Convey("Parses a valid build proto", func() {
			Convey("For a finished build", func() {
				Convey("That succeeded", func() {
					status, result, err = toTryjobStatusAndResult(ctx, b)
					So(err, ShouldBeNil)
					So(result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
				})
				Convey("That failed", func() {
					Convey("Transiently", func() {
						b.Status = bbpb.Status_INFRA_FAILURE
						status, result, err = toTryjobStatusAndResult(ctx, b)
						So(err, ShouldBeNil)
						So(result.Status, ShouldEqual, tryjob.Result_FAILED_TRANSIENTLY)
					})
					Convey("Permanently", func() {
						b.Status = bbpb.Status_FAILURE
						status, result, err = toTryjobStatusAndResult(ctx, b)
						So(err, ShouldBeNil)
						So(result.Status, ShouldEqual, tryjob.Result_FAILED_PERMANENTLY)
					})
				})
				So(status, ShouldEqual, tryjob.Status_ENDED)
			})
			Convey("For a pending build", func() {
				Convey("That is still scheduled", func() {
					b.Status = bbpb.Status_SCHEDULED
				})
				Convey("That is already running", func() {
					b.Status = bbpb.Status_STARTED
				})
				status, result, err = toTryjobStatusAndResult(ctx, b)
				So(err, ShouldBeNil)
				So(status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(result.Status, ShouldEqual, tryjob.Result_UNKNOWN)
			})
			Convey("For a build that has been cancelled", func() {
				b.Status = bbpb.Status_CANCELED
				status, result, err = toTryjobStatusAndResult(ctx, b)
				So(err, ShouldBeNil)
				So(status, ShouldEqual, tryjob.Status_ENDED)
				So(result.Status, ShouldEqual, tryjob.Result_FAILED_TRANSIENTLY)
			})
			// Fields copied without change.
			So(result.CreateTime.Seconds, ShouldEqual, 1635791494)
			So(result.UpdateTime.Seconds, ShouldEqual, 1635791521)
			So(result.GetBuildbucket().Id, ShouldEqual, 8831742013603886929)
			So(result.GetBuildbucket().Status, ShouldEqual, b.Status)
		})

	})
}
