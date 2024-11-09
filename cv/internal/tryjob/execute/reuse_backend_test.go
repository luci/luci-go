// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(tryjob.Tryjob{}))
}

func TestFindReuseInBackend(t *testing.T) {
	t.Parallel()

	ftt.Run("FindReuseInBackend", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject     = "testProj"
			bbHost       = "buildbucket.example.com"
			reuseKey     = "cafecafe"
			clid         = 34586452134
			gHost        = "example-review.com"
			gRepo        = "repo/a"
			gChange      = 123
			gPatchset    = 5
			gMinPatchset = 4
		)
		var runID = common.MakeRunID(lProject, ct.Clock.Now().Add(-1*time.Hour), 1, []byte("abcd"))

		w := &worker{
			run: &run.Run{
				ID:   runID,
				Mode: run.DryRun,
				CLs:  common.CLIDs{clid},
			},
			cls: []*run.RunCL{
				{
					ID: clid,
					Detail: &changelist.Snapshot{
						Patchset:              gPatchset,
						MinEquivalentPatchset: gMinPatchset,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: &gerritpb.ChangeInfo{
									Project: gRepo,
									Number:  gChange,
								},
							},
						},
					},
				},
			},
			knownTryjobIDs: make(common.TryjobIDSet),
			reuseKey:       reuseKey,
			clPatchsets:    tryjob.CLPatchsets{tryjob.MakeCLPatchset(clid, gPatchset)},
			backend: &bbfacade.Facade{
				ClientFactory: ct.BuildbucketFake.NewClientFactory(),
			},
			mutator: tryjob.NewMutator(run.NewNotifier(ct.TQDispatcher)),
		}
		builder := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "BucketFoo",
			Builder: "BuilderFoo",
		}
		defFoo := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host:    "buildbucket.example.com",
					Builder: builder,
				},
			},
		}

		ct.BuildbucketFake.AddBuilder(bbHost, builder, nil)
		bbClient := ct.BuildbucketFake.MustNewClient(ctx, bbHost, lProject)
		build, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builder,
			GerritChanges: []*bbpb.GerritChange{{
				Host:     gHost,
				Project:  gRepo,
				Change:   gChange,
				Patchset: gPatchset,
			}},
		})
		assert.NoErr(t, err)
		epoch := ct.Clock.Now().UTC()
		build = ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), func(build *bbpb.Build) {
			build.Status = bbpb.Status_SUCCESS
			build.StartTime = timestamppb.New(epoch)
			build.EndTime = timestamppb.New(epoch.Add(30 * time.Minute))
		})
		ct.Clock.Add(1 * time.Hour)

		t.Run("Found reuse", func(t *ftt.Test) {
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			eid := tryjob.MustBuildbucketID(bbHost, build.GetId())
			result[defFoo].RetentionKey = "" // clear the retention key before comparison
			assert.Loosely(t, result[defFoo], should.Match(&tryjob.Tryjob{
				ID:               tryjob.MustResolve(ctx, eid)[0],
				ExternalID:       eid,
				EVersion:         1,
				EntityCreateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				EntityUpdateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				ReuseKey:         reuseKey,
				Definition:       defFoo,
				CLPatchsets:      tryjob.CLPatchsets{tryjob.MakeCLPatchset(clid, gPatchset)},
				Status:           tryjob.Status_ENDED,
				ReusedBy:         common.RunIDs{runID},
				Result: &tryjob.Result{
					CreateTime: build.GetCreateTime(),
					UpdateTime: build.GetUpdateTime(),
					Backend: &tryjob.Result_Buildbucket_{
						Buildbucket: &tryjob.Result_Buildbucket{
							Id:      build.GetId(),
							Builder: builder,
							Status:  bbpb.Status_SUCCESS,
						},
					},
					Status: tryjob.Result_SUCCEEDED,
				},
			}))
		})

		t.Run("Multiple matches and picks latest", func(t *ftt.Test) {
			newerBuild, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder:       builder,
				GerritChanges: build.GetInput().GetGerritChanges(),
			})
			assert.NoErr(t, err)
			epoch := ct.Clock.Now().UTC()
			newerBuild = ct.BuildbucketFake.MutateBuild(ctx, bbHost, newerBuild.GetId(), func(build *bbpb.Build) {
				build.Status = bbpb.Status_SUCCESS
				build.StartTime = timestamppb.New(epoch)
				build.EndTime = timestamppb.New(epoch.Add(10 * time.Minute))
			})
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			assert.Loosely(t, result[defFoo].ExternalID, should.Equal(tryjob.MustBuildbucketID(bbHost, newerBuild.GetId())))
		})

		t.Run("Build already known", func(t *ftt.Test) {
			w.knownExternalIDs = stringset.NewFromSlice(string(tryjob.MustBuildbucketID(bbHost, build.GetId())))
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.BeEmpty)
		})

		t.Run("Not reusable", func(t *ftt.Test) {
			failedButNewerBuild, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder:       builder,
				GerritChanges: build.GetInput().GetGerritChanges(),
			})
			assert.NoErr(t, err)
			epoch := ct.Clock.Now().UTC()
			ct.BuildbucketFake.MutateBuild(ctx, bbHost, failedButNewerBuild.GetId(), func(build *bbpb.Build) {
				build.Status = bbpb.Status_FAILURE // failed build is not reusable
				build.StartTime = timestamppb.New(epoch)
				build.EndTime = timestamppb.New(epoch.Add(10 * time.Minute))
			})

			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			assert.Loosely(t, result[defFoo].ExternalID, should.Equal(tryjob.MustBuildbucketID(bbHost, build.Id))) // reuse the old successful build
		})

		t.Run("Tryjob already exists", func(t *ftt.Test) {
			eid := tryjob.MustBuildbucketID(bbHost, build.GetId())
			tj := eid.MustCreateIfNotExists(ctx)
			ct.Clock.Add(10 * time.Second)
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			assert.NoErr(t, err)
			assert.Loosely(t, result, should.HaveLength(1))
			assert.Loosely(t, result, should.ContainKey(defFoo))
			assert.Loosely(t, result[defFoo].ExternalID, should.Equal(tryjob.MustBuildbucketID(bbHost, build.Id)))
			latest := &tryjob.Tryjob{ID: common.TryjobID(tj.ID)}
			assert.Loosely(t, datastore.Get(ctx, latest), should.BeNil)
			latest.RetentionKey = "" // clear the retention key before comparison
			assert.Loosely(t, latest, should.Match(&tryjob.Tryjob{
				ID:               tj.ID,
				ExternalID:       eid,
				EVersion:         tj.EVersion + 1,
				EntityCreateTime: tj.EntityCreateTime,
				EntityUpdateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				Definition:       defFoo,
				ReuseKey:         w.reuseKey,
				CLPatchsets:      w.clPatchsets,
				Status:           tryjob.Status_ENDED,
				ReusedBy:         common.RunIDs{runID},
				Result: &tryjob.Result{
					CreateTime: build.GetCreateTime(),
					UpdateTime: build.GetUpdateTime(),
					Backend: &tryjob.Result_Buildbucket_{
						Buildbucket: &tryjob.Result_Buildbucket{
							Id:      build.GetId(),
							Builder: builder,
							Status:  bbpb.Status_SUCCESS,
						},
					},
					Status: tryjob.Result_SUCCEEDED,
				},
			}))
		})
	})
}
