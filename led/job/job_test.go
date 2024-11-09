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

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/luciexe/exe"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job/experiments"
)

const phonyTagExperiment = "luci.test.add_phony_tag_experiment"

func init() {
	experiments.Register(phonyTagExperiment, func(ctx context.Context, b *bbpb.Build, task *swarmingpb.NewTaskRequest) error {
		task.Tags = append(task.Tags, "phony_tag:experiment")
		return nil
	})
}

func TestFlattenToSwarming(t *testing.T) {
	t.Parallel()

	ftt.Run(`FlattenToSwarming`, t, func(t *ftt.Test) {
		fixture, err := os.ReadFile("jobcreate/testdata/bbagent_cas.job.json")
		assert.Loosely(t, err, should.BeNil)

		ctx := cryptorand.MockForTest(context.Background(), 4)

		bbJob := &Definition{}
		assert.Loosely(t, protojson.Unmarshal(fixture, bbJob), should.BeNil)
		bb := bbJob.GetBuildbucket()
		totalExpiration := bb.BbagentArgs.Build.SchedulingTimeout.Seconds

		t.Run(`bbagent`, func(t *ftt.Test) {
			assert.Loosely(t, bbJob.FlattenToSwarming(ctx, "username", "parent_task_id", NoKitchenSupport(), "off"), should.BeNil)

			sw := bbJob.GetSwarming()
			assert.Loosely(t, sw, should.NotBeNil)
			assert.Loosely(t, sw.Task.Tags, should.Resemble([]string{
				"allow_milo:1",
				"log_location:logdog://luci-logdog-dev.appspot.com/infra/led/username/1dd4751f899d743d0780c9644375aae211327818655f3d20f84abef6a9df0898/+/build.proto",
			}))

			assert.Loosely(t, sw.Task.TaskSlices, should.HaveLength(2))

			slice0 := sw.Task.TaskSlices[0]
			assert.Loosely(t, slice0.ExpirationSecs, should.Equal(240))
			assert.Loosely(t, slice0.Properties.Dimensions, should.Resemble([]*swarmingpb.StringPair{
				{
					Key:   "caches",
					Value: "builder_1d1f048016f3dc7294e1abddfd758182bc95619cec2a87d01a3f24517b4e2814_v2",
				},
				{Key: "cpu", Value: "x86-64"},
				{Key: "os", Value: "Ubuntu"},
				{Key: "pool", Value: "Chrome"},
			}))

			assert.Loosely(t, slice0.Properties.Command[:3], should.Resemble([]string{
				"bbagent${EXECUTABLE_SUFFIX}", "--output",
				"${ISOLATED_OUTDIR}/build.proto.json",
			}))
			bbArgs, err := bbinput.Parse(slice0.Properties.Command[3])
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, bbArgs.PayloadPath, should.Match("kitchen-checkout"))
			assert.Loosely(t, bbArgs.Build.Exe.Cmd, should.Resemble([]string{"luciexe"}))

			props := ledProperties{}
			err = exe.ParseProperties(bbArgs.Build.Input.Properties, map[string]any{
				"$recipe_engine/led": &props,
			})
			assert.Loosely(t, err, should.BeNil)

			expectedProps := ledProperties{
				LedRunID: "infra/led/username/1dd4751f899d743d0780c9644375aae211327818655f3d20f84abef6a9df0898",
				CIPDInput: &cipdInput{
					"infra/recipe_bundles/chromium.googlesource.com/infra/luci/recipes-py",
					"HEAD",
				},
			}
			assert.Loosely(t, props, should.Resemble(expectedProps))

			// Added `kitchen-checkout` as the last CipdInputs entry.
			assert.Loosely(t, slice0.Properties.CipdInput.Packages, should.HaveLength(17)) // see bbagent.job.json
			assert.Loosely(t, slice0.Properties.CipdInput.Packages[16], should.Resemble(&swarmingpb.CipdPackage{
				Path:        "kitchen-checkout",
				PackageName: expectedProps.CIPDInput.Package,
				Version:     expectedProps.CIPDInput.Version,
			}))

			slice1 := sw.Task.TaskSlices[1]
			assert.Loosely(t, slice1.ExpirationSecs, should.Equal(21360))
			assert.Loosely(t, slice1.Properties.Dimensions, should.Resemble([]*swarmingpb.StringPair{
				{Key: "cpu", Value: "x86-64"},
				{Key: "os", Value: "Ubuntu"},
				{Key: "pool", Value: "Chrome"},
			}))

			assert.Loosely(t, slice0.ExpirationSecs+slice1.ExpirationSecs, should.Equal(totalExpiration))
		})

		t.Run(`kitchen`, func(t *ftt.Test) {
			bb.LegacyKitchen = true
			assert.Loosely(t, bbJob.FlattenToSwarming(ctx, "username", "parent_task_id", NoKitchenSupport(), "off"), should.ErrLike(
				"kitchen job Definitions not supported"))
		})

		t.Run(`explicit max expiration`, func(t *ftt.Test) {
			// set a dimension to expire after end of current task
			editDims(t, bbJob, "final=value@40000")

			assert.Loosely(t, bbJob.FlattenToSwarming(ctx, "username", "parent_task_id", NoKitchenSupport(), "off"), should.BeNil)

			sw := bbJob.GetSwarming()
			assert.Loosely(t, sw, should.NotBeNil)
			assert.Loosely(t, sw.Task.TaskSlices, should.HaveLength(2))

			assert.Loosely(t, sw.Task.TaskSlices[0].ExpirationSecs, should.Equal(240))
			assert.Loosely(t, sw.Task.TaskSlices[1].ExpirationSecs, should.Equal(40000-240))
		})

		t.Run(`no expiring dims`, func(t *ftt.Test) {
			bb := bbJob.GetBuildbucket()
			for _, dim := range bb.GetBbagentArgs().Build.Infra.Swarming.TaskDimensions {
				dim.Expiration = nil
			}
			for _, cache := range bb.GetBbagentArgs().Build.Infra.Swarming.Caches {
				cache.WaitForWarmCache = nil
			}

			assert.Loosely(t, bbJob.FlattenToSwarming(ctx, "username", "parent_task_id", NoKitchenSupport(), "off"), should.BeNil)

			sw := bbJob.GetSwarming()
			assert.Loosely(t, sw, should.NotBeNil)
			assert.Loosely(t, sw.Task.TaskSlices, should.HaveLength(1))

			assert.Loosely(t, sw.Task.TaskSlices[0].ExpirationSecs, should.Equal(21600))
		})

		t.Run(`CasUserPayload recipe`, func(t *ftt.Test) {
			MustHLEdit(t, bbJob, func(je HighLevelEditor) {
				je.TaskPayloadSource("", "")
				je.CASTaskPayload("some/path", &swarmingpb.CASReference{
					CasInstance: "projects/chromium-swarm-dev/instances/default_instance",
					Digest: &swarmingpb.Digest{
						Hash:      "b7c329e532e221e23809ba23f9af5b309aa17d490d845580207493d381998bd9",
						SizeBytes: 24,
					},
				})
			})

			assert.Loosely(t, bbJob.FlattenToSwarming(ctx, "username", "parent_task_id", NoKitchenSupport(), "off"), should.BeNil)

			sw := bbJob.GetSwarming()
			assert.Loosely(t, sw, should.NotBeNil)
			assert.Loosely(t, sw.Task.TaskSlices, should.HaveLength(2))

			slice0 := sw.Task.TaskSlices[0]
			for _, pkg := range slice0.Properties.CipdInput.Packages {
				// there shouldn't be any recipe package any more
				assert.Loosely(t, pkg.Version, should.NotEqual("HEAD"))
			}

			assert.Loosely(t, slice0.Properties.Command[:3], should.Resemble([]string{
				"bbagent${EXECUTABLE_SUFFIX}", "--output",
				"${ISOLATED_OUTDIR}/build.proto.json",
			}))
			bbArgs, err := bbinput.Parse(slice0.Properties.Command[3])
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, bbArgs.PayloadPath, should.Match("some/path"))
			assert.Loosely(t, bbArgs.Build.Exe.Cmd, should.Resemble([]string{"luciexe"}))

			props := ledProperties{}
			err = exe.ParseProperties(bbArgs.Build.Input.Properties, map[string]any{
				"$recipe_engine/led": &props,
			})
			assert.Loosely(t, err, should.BeNil)
			expectedProps := ledProperties{
				LedRunID: "infra/led/username/1dd4751f899d743d0780c9644375aae211327818655f3d20f84abef6a9df0898",
				RbeCasInput: &swarmingpb.CASReference{
					CasInstance: "projects/chromium-swarm-dev/instances/default_instance",
					Digest: &swarmingpb.Digest{
						Hash:      "b7c329e532e221e23809ba23f9af5b309aa17d490d845580207493d381998bd9",
						SizeBytes: 24,
					},
				},
			}
			assert.Loosely(t, props, should.Resemble(expectedProps))
		})
		t.Run(`With experiments`, func(t *ftt.Test) {
			MustHLEdit(t, bbJob, func(je HighLevelEditor) {
				je.Experiments(map[string]bool{
					phonyTagExperiment:             true, // see init() above
					"should_be_skipped_as_unknown": true,
				})
			})

			assert.Loosely(t, bbJob.FlattenToSwarming(ctx, "username", "parent_task_id", NoKitchenSupport(), "off"), should.BeNil)

			sw := bbJob.GetSwarming()
			assert.Loosely(t, sw, should.NotBeNil)
			assert.Loosely(t, sw.Task.Tags[len(sw.Task.Tags)-1], should.Equal("phony_tag:experiment"))
		})
	})

}
