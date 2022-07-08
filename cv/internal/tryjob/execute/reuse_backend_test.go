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

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	bbfake "go.chromium.org/luci/cv/internal/buildbucket/fake"
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
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject     = "testProj"
			bbHost       = "buildbucket.example.com"
			buildID      = 9524107902457
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

		now := ct.Clock.Now().UTC()
		build := bbfake.NewBuildConstructor().
			WithHost(bbHost).
			WithID(buildID).
			WithBuilderID(builder).
			WithCreateTime(now.Add(-1 * time.Hour)).
			WithStartTime(now.Add(-1 * time.Hour)).
			WithEndTime(now.Add(-30 * time.Minute)).
			WithUpdateTime(now.Add(-30 * time.Minute)).
			WithStatus(bbpb.Status_SUCCESS).
			AppendGerritChanges(&bbpb.GerritChange{
				Host:     gHost,
				Project:  gRepo,
				Change:   gChange,
				Patchset: gPatchset,
			}).
			Construct()
		ct.BuildbucketFake.AddBuild(build)

		Convey("Found reuse", func() {
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			eid := tryjob.MustBuildbucketID(bbHost, buildID)
			So(result[defFoo], cvtesting.SafeShouldResemble, &tryjob.Tryjob{
				ID:               tryjob.MustResolve(ctx, eid)[0],
				ExternalID:       eid,
				EVersion:         1,
				EntityCreateTime: now,
				EntityUpdateTime: now,
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
							Id:      buildID,
							Builder: builder,
							Status:  bbpb.Status_SUCCESS,
						},
					},
					Status: tryjob.Result_SUCCEEDED,
				},
			})
		})

		Convey("Multiple matches and picks latest", func() {
			newerBuild := bbfake.NewConstructorFromBuild(build).
				WithID(build.Id - 1).
				WithCreateTime(build.GetCreateTime().AsTime().Add(10 * time.Minute)).WithStartTime(build.GetStartTime().AsTime().Add(10 * time.Minute)).
				WithEndTime(build.GetEndTime().AsTime().Add(10 * time.Minute)).
				WithUpdateTime(build.GetUpdateTime().AsTime().Add(10 * time.Minute)).
				Construct()
			ct.BuildbucketFake.AddBuild(newerBuild)
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ExternalID, ShouldEqual, tryjob.MustBuildbucketID(bbHost, newerBuild.GetId()))
		})

		Convey("Build already known", func() {
			w.knownExternalIDs = stringset.NewFromSlice(string(tryjob.MustBuildbucketID(bbHost, buildID)))
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Not reusable", func() {
			failedButNewerBuild := bbfake.NewConstructorFromBuild(build).
				WithID(build.Id - 1).
				WithCreateTime(build.GetCreateTime().AsTime().Add(10 * time.Minute)).WithStartTime(build.GetStartTime().AsTime().Add(10 * time.Minute)).
				WithEndTime(build.GetEndTime().AsTime().Add(10 * time.Minute)).
				WithUpdateTime(build.GetUpdateTime().AsTime().Add(10 * time.Minute)).
				WithStatus(bbpb.Status_FAILURE). // failed build is not reusable
				Construct()
			ct.BuildbucketFake.AddBuild(failedButNewerBuild)
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ExternalID, ShouldEqual, tryjob.MustBuildbucketID(bbHost, build.Id)) // reuse the old successful build
		})

		Convey("Tryjob already exists", func() {
			eid := tryjob.MustBuildbucketID(bbHost, buildID)
			tj := eid.MustCreateIfNotExists(ctx)
			ct.Clock.Add(10 * time.Second)
			result, err := w.findReuseInBackend(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ExternalID, ShouldEqual, tryjob.MustBuildbucketID(bbHost, build.Id))
			latest := &tryjob.Tryjob{ID: common.TryjobID(tj.ID)}
			So(datastore.Get(ctx, latest), ShouldBeNil)
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
							Id:      buildID,
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
