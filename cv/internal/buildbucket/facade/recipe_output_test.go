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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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
