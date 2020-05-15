// Copyright 2020 The LUCI Authors.
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

package job

import (
	"context"
	"os"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	durpb "github.com/golang/protobuf/ptypes/duration"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/luciexe/exe"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

func TestFlattenToSwarming(t *testing.T) {
	t.Parallel()

	Convey(`FlattenToSwarming`, t, func() {
		fixture, err := os.Open("jobcreate/testdata/bbagent.job.json")
		So(err, ShouldBeNil)
		defer fixture.Close()

		ctx := cryptorand.MockForTest(context.Background(), 4)

		bbJob := &Definition{}
		So((&jsonpb.Unmarshaler{}).Unmarshal(fixture, bbJob), ShouldBeNil)
		bb := bbJob.GetBuildbucket()
		totalExpiration := bb.BbagentArgs.Build.SchedulingTimeout.Seconds

		Convey(`bbagent`, func() {
			So(bbJob.FlattenToSwarming(ctx, "username", NoKitchenSupport()), ShouldBeNil)

			sw := bbJob.GetSwarming()
			So(sw, ShouldNotBeNil)
			So(sw.Task.Tags, ShouldResemble, []string{
				"allow_milo:1",
				"log_location:logdog://luci-logdog-dev.appspot.com/infra/led/username/1dd4751f899d743d0780c9644375aae211327818655f3d20f84abef6a9df0898/+/build.proto",
			})

			So(sw.Task.TaskSlices, ShouldHaveLength, 2)

			slice0 := sw.Task.TaskSlices[0]
			So(slice0.Expiration, ShouldResembleProto, &durpb.Duration{Seconds: 240})
			So(slice0.Properties.Dimensions, ShouldResembleProto, []*swarmingpb.StringListPair{
				{
					Key: "caches", Values: []string{
						"builder_1d1f048016f3dc7294e1abddfd758182bc95619cec2a87d01a3f24517b4e2814_v2",
					},
				},
				{Key: "cpu", Values: []string{"x86-64"}},
				{Key: "os", Values: []string{"Ubuntu"}},
				{Key: "pool", Values: []string{"Chrome"}},
			})

			So(slice0.Properties.Command[:3], ShouldResemble, []string{
				"bbagent${EXECUTABLE_SUFFIX}", "--output",
				"${ISOLATED_OUTDIR}/build.proto.json",
			})
			bbArgs, err := bbinput.Parse(slice0.Properties.Command[3])
			So(err, ShouldBeNil)

			So(bbArgs.ExecutablePath, ShouldResemble, "kitchen-checkout/luciexe")

			props := ledProperties{}
			err = exe.ParseProperties(bbArgs.Build.Input.Properties, map[string]interface{}{
				"$recipe_engine/led": &props,
			})
			So(err, ShouldBeNil)

			expectedProps := ledProperties{
				LedRunID: "infra/led/username/1dd4751f899d743d0780c9644375aae211327818655f3d20f84abef6a9df0898",
				CIPDInput: &cipdInput{
					"infra/recipe_bundles/chromium.googlesource.com/infra/luci/recipes-py",
					"refs/heads/master",
				},
			}
			So(props, ShouldResemble, expectedProps)

			So(slice0.Properties.CipdInputs, ShouldContain, &swarmingpb.CIPDPackage{
				DestPath:    "kitchen-checkout",
				PackageName: expectedProps.CIPDInput.Package,
				Version:     expectedProps.CIPDInput.Version,
			})

			slice1 := sw.Task.TaskSlices[1]
			So(slice1.Expiration, ShouldResembleProto, &durpb.Duration{Seconds: 21360})
			So(slice1.Properties.Dimensions, ShouldResembleProto, []*swarmingpb.StringListPair{
				{Key: "cpu", Values: []string{"x86-64"}},
				{Key: "os", Values: []string{"Ubuntu"}},
				{Key: "pool", Values: []string{"Chrome"}},
			})

			So(slice0.Expiration.Seconds+slice1.Expiration.Seconds, ShouldEqual, totalExpiration)
		})

		Convey(`kitchen`, func() {
			bb.LegacyKitchen = true
			So(bbJob.FlattenToSwarming(ctx, "username", NoKitchenSupport()), ShouldErrLike,
				"kitchen job Definitions not supported")
		})

		Convey(`explicit max expiration`, func() {
			// set a dimension to expire after end of current task
			editDims(bbJob, "final=value@40000")

			So(bbJob.FlattenToSwarming(ctx, "username", NoKitchenSupport()), ShouldBeNil)

			sw := bbJob.GetSwarming()
			So(sw, ShouldNotBeNil)
			So(sw.Task.TaskSlices, ShouldHaveLength, 2)

			So(sw.Task.TaskSlices[0].Expiration, ShouldResembleProto, &durpb.Duration{Seconds: 240})
			So(sw.Task.TaskSlices[1].Expiration, ShouldResembleProto, &durpb.Duration{Seconds: 40000 - 240})
		})

		Convey(`no expiring dims`, func() {
			bb := bbJob.GetBuildbucket()
			for _, dim := range bb.GetBbagentArgs().Build.Infra.Swarming.TaskDimensions {
				dim.Expiration = nil
			}
			for _, cache := range bb.GetBbagentArgs().Build.Infra.Swarming.Caches {
				cache.WaitForWarmCache = nil
			}

			So(bbJob.FlattenToSwarming(ctx, "username", NoKitchenSupport()), ShouldBeNil)

			sw := bbJob.GetSwarming()
			So(sw, ShouldNotBeNil)
			So(sw.Task.TaskSlices, ShouldHaveLength, 1)

			So(sw.Task.TaskSlices[0].Expiration, ShouldResembleProto, &durpb.Duration{Seconds: 21600})
		})

		Convey(`UserPayload recipe`, func() {
			SoHLEdit(bbJob, func(je HighLevelEditor) {
				je.TaskPayload("", "", "some/path")
			})
			bbJob.UserPayload.Digest = "beef"

			So(bbJob.FlattenToSwarming(ctx, "username", NoKitchenSupport()), ShouldBeNil)

			sw := bbJob.GetSwarming()
			So(sw, ShouldNotBeNil)
			So(sw.Task.TaskSlices, ShouldHaveLength, 2)

			slice0 := sw.Task.TaskSlices[0]
			for _, pkg := range slice0.Properties.CipdInputs {
				// there shouldn't be any recipe package any more
				So(pkg.Version, ShouldNotEqual, "refs/heads/master")
			}

			So(slice0.Properties.Command[:3], ShouldResemble, []string{
				"bbagent${EXECUTABLE_SUFFIX}", "--output",
				"${ISOLATED_OUTDIR}/build.proto.json",
			})
			bbArgs, err := bbinput.Parse(slice0.Properties.Command[3])
			So(err, ShouldBeNil)

			So(bbArgs.ExecutablePath, ShouldResemble, "some/path/luciexe")
		})

	})

}
