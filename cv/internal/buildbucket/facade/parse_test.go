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
	"fmt"
	"io/ioutil"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseStatusAndResult(t *testing.T) {
	Convey("Parse Status and Result", t, func() {
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
				_, _, err := parseStatusAndResult(ctx, b)
				So(err, ShouldErrLike, "unexpected buildbucket status")
			})
		})
		Convey("Parses a valid build proto", func() {
			Convey("For a finished build", func() {
				Convey("That succeeded", func() {
					status, result, err = parseStatusAndResult(ctx, b)
					So(err, ShouldBeNil)
					So(result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
				})
				Convey("That timed out", func() {
					b.Status = bbpb.Status_FAILURE
					b.StatusDetails = &bbpb.StatusDetails{
						Timeout: &bbpb.StatusDetails_Timeout{},
					}
					status, result, err = parseStatusAndResult(ctx, b)
					So(err, ShouldBeNil)
					So(result.Status, ShouldEqual, tryjob.Result_TIMEOUT)
				})
				Convey("That failed", func() {
					Convey("Transiently", func() {
						b.Status = bbpb.Status_INFRA_FAILURE
						status, result, err = parseStatusAndResult(ctx, b)
						So(err, ShouldBeNil)
						So(result.Status, ShouldEqual, tryjob.Result_FAILED_TRANSIENTLY)
					})
					Convey("Permanently", func() {
						b.Status = bbpb.Status_FAILURE
						status, result, err = parseStatusAndResult(ctx, b)
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
				status, result, err = parseStatusAndResult(ctx, b)
				So(err, ShouldBeNil)
				So(status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(result.Status, ShouldEqual, tryjob.Result_UNKNOWN)
			})
			Convey("For a build that has been cancelled", func() {
				b.Status = bbpb.Status_CANCELED
				status, result, err = parseStatusAndResult(ctx, b)
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

func TestParseOutput(t *testing.T) {
	ctx := context.Background()
	Convey("parseOutput", t, func() {
		Convey("Allow reuse", func() {
			Convey("For full runs", func() {
				result := parseBuildResult(ctx, loadTestBuild("reuse_full"))
				So(result.output, ShouldResembleProto, &recipe.Output{
					Reuse: []*recipe.Output_Reuse{{ModeRegexp: "FULL_RUN"}},
				})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
			Convey("For dry runs", func() {
				result := parseBuildResult(ctx, loadTestBuild("reuse_dry"))
				So(result.output, ShouldResembleProto, &recipe.Output{
					Reuse: []*recipe.Output_Reuse{{ModeRegexp: "DRY_RUN"}},
				})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
		})
		Convey("Triggered ids", func() {
			Convey("Legacy property only", func() {
				result := parseBuildResult(ctx, loadTestBuild("triggered_builds_legacy"))
				So(result.output, ShouldResembleProto, &recipe.Output{
					TriggeredBuildIds: []int64{8832715138311111281},
				})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
			Convey("Proto property only", func() {
				result := parseBuildResult(ctx, loadTestBuild("triggered_builds_new"))
				So(result.output, ShouldResembleProto, &recipe.Output{
					TriggeredBuildIds: []int64{8832715138311111281},
				})
				So(result.isTransFailure, ShouldBeFalse)
			})
			Convey("Proto overrides legacy", func() {
				// In this test, legacy has a different triggered build id, the
				// id set in the protobuf property should be the one in the
				// returned output.
				result := parseBuildResult(ctx, loadTestBuild("triggered_builds_conflict"))
				So(result.output, ShouldResembleProto, &recipe.Output{
					TriggeredBuildIds: []int64{8832715138311111281},
				})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
		})
		Convey("Do not retry", func() {
			Convey("Legacy property only", func() {
				result := parseBuildResult(ctx, loadTestBuild("retry_denied_legacy"))
				So(result.output, ShouldResembleProto, &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
			Convey("Proto property only", func() {
				result := parseBuildResult(ctx, loadTestBuild("retry_denied_new"))
				So(result.output, ShouldResembleProto, &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
			Convey("Proto overrides legacy", func() {
				// In this test, the protobuf-based property allows retry and
				// the legacy property denies it.
				// Test that the protobuf property overrides the legacy one.
				result := parseBuildResult(ctx, loadTestBuild("retry_denied_conflict"))
				So(result.output, ShouldResembleProto, &recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_ALLOWED})
				So(result.isTransFailure, ShouldBeFalse)
				So(result.err, ShouldBeNil)
			})
		})
		Convey("Transient failure", func() {
			result := parseBuildResult(ctx, loadTestBuild("transient_failure"))
			So(result.output, ShouldResembleProto, &recipe.Output{})
			So(result.isTransFailure, ShouldBeTrue)
			So(result.err, ShouldBeNil)
		})
		Convey("No properties", func() {
			result := parseBuildResult(ctx, loadTestBuild("no_props"))
			So(result.output, ShouldBeNil)
			So(result.isTransFailure, ShouldBeFalse)
			So(result.err, ShouldBeNil)
		})
		Convey("Bad data", func() {
			result := parseBuildResult(ctx, loadTestBuild("bad_data"))
			So(result.output, ShouldResembleProto, &recipe.Output{})
			So(result.isTransFailure, ShouldBeFalse)
			So(result.err.Errors, ShouldHaveLength, 3)
		})
	})
}

func loadTestBuild(fixtureBaseName string) *bbpb.Build {
	data, err := ioutil.ReadFile(fmt.Sprintf("testdata/%s.json", fixtureBaseName))
	if err != nil {
		panic(err)
	}
	ret := &bbpb.Build{}
	if err := protojson.Unmarshal(data, ret); err != nil {
		panic(err)
	}
	return ret
}
