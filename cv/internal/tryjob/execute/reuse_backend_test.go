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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFindReuseInBackend(t *testing.T) {
	t.Parallel()

	Convey("FindReuseInBackend", t, func() {
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
			rm: run.NewNotifier(ct.TQDispatcher),
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
		So(err, ShouldBeNil)
		epoch := ct.Clock.Now().UTC()
		build = ct.BuildbucketFake.MutateBuild(ctx, bbHost, build.GetId(), func(build *bbpb.Build) {
			build.Status = bbpb.Status_SUCCESS
			build.StartTime = timestamppb.New(epoch)
			build.EndTime = timestamppb.New(epoch.Add(30 * time.Minute))
		})
		ct.Clock.Add(1 * time.Hour)

		Convey("Found reuse", func() {
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			eid := tryjob.MustBuildbucketID(bbHost, build.GetId())
			result[defFoo].RetentionKey = "" // clear the retention key before comparison
			So(result[defFoo], cvtesting.SafeShouldResemble, &tryjob.Tryjob{
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
			})
		})

		Convey("Multiple matches and picks latest", func() {
			newerBuild, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder:       builder,
				GerritChanges: build.GetInput().GetGerritChanges(),
			})
			So(err, ShouldBeNil)
			epoch := ct.Clock.Now().UTC()
			newerBuild = ct.BuildbucketFake.MutateBuild(ctx, bbHost, newerBuild.GetId(), func(build *bbpb.Build) {
				build.Status = bbpb.Status_SUCCESS
				build.StartTime = timestamppb.New(epoch)
				build.EndTime = timestamppb.New(epoch.Add(10 * time.Minute))
			})
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ExternalID, ShouldEqual, tryjob.MustBuildbucketID(bbHost, newerBuild.GetId()))
		})

		Convey("Build already known", func() {
			w.knownExternalIDs = stringset.NewFromSlice(string(tryjob.MustBuildbucketID(bbHost, build.GetId())))
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Not reusable", func() {
			failedButNewerBuild, err := bbClient.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder:       builder,
				GerritChanges: build.GetInput().GetGerritChanges(),
			})
			So(err, ShouldBeNil)
			epoch := ct.Clock.Now().UTC()
			ct.BuildbucketFake.MutateBuild(ctx, bbHost, failedButNewerBuild.GetId(), func(build *bbpb.Build) {
				build.Status = bbpb.Status_FAILURE // failed build is not reusable
				build.StartTime = timestamppb.New(epoch)
				build.EndTime = timestamppb.New(epoch.Add(10 * time.Minute))
			})

			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ExternalID, ShouldEqual, tryjob.MustBuildbucketID(bbHost, build.Id)) // reuse the old successful build
		})

		Convey("Tryjob already exists", func() {
			eid := tryjob.MustBuildbucketID(bbHost, build.GetId())
			tj := eid.MustCreateIfNotExists(ctx)
			ct.Clock.Add(10 * time.Second)
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ExternalID, ShouldEqual, tryjob.MustBuildbucketID(bbHost, build.Id))
			latest := &tryjob.Tryjob{ID: common.TryjobID(tj.ID)}
			So(datastore.Get(ctx, latest), ShouldBeNil)
			latest.RetentionKey = "" // clear the retention key before comparison
			So(latest, cvtesting.SafeShouldResemble, &tryjob.Tryjob{
				ID:               tj.ID,
				ExternalID:       eid,
				EVersion:         tj.EVersion + 1,
				EntityCreateTime: tj.EntityCreateTime,
				EntityUpdateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				Definition:       defFoo,
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
			})
		})
	})
}
