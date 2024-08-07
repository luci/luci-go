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
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLaunch(t *testing.T) {
	t.Parallel()

	Convey("Launch", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

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
									Owner: &gerritpb.AccountInfo{
										Email: "owner@example.com",
									},
								},
							},
						},
					},
					Trigger: &run.Trigger{
						Mode:  string(run.DryRun),
						Email: "triggerer@example.com",
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
					Host:    bbHost,
					Builder: builder,
				},
			},
		}
		ct.BuildbucketFake.AddBuilder(bbHost, builder, nil)
		Convey("Works", func() {
			tj := w.makePendingTryjob(ctx, defFoo)
			So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
			}, nil), ShouldBeNil)
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			eid := tryjobs[0].ExternalID
			So(eid, ShouldNotBeEmpty)
			So(eid.MustLoad(ctx), ShouldNotBeNil)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_TRIGGERED)
			host, buildID, err := eid.ParseBuildbucketID()
			So(err, ShouldBeNil)
			So(host, ShouldEqual, bbHost)
			bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
			So(err, ShouldBeNil)
			build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID})
			So(err, ShouldBeNil)
			So(build, ShouldNotBeNil)
			So(w.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
				{
					Time: timestamppb.New(ct.Clock.Now().UTC()),
					Kind: &tryjob.ExecutionLogEntry_TryjobsLaunched_{
						TryjobsLaunched: &tryjob.ExecutionLogEntry_TryjobsLaunched{
							Tryjobs: []*tryjob.ExecutionLogEntry_TryjobSnapshot{
								makeLogTryjobSnapshot(defFoo, tryjobs[0], false),
							},
						},
					},
				},
			})
		})

		Convey("Failed to trigger", func() {
			def := &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: bbHost,
						Builder: &bbpb.BuilderID{
							Project: lProject,
							Bucket:  "BucketFoo",
							Builder: "non-existent-builder",
						},
					},
				},
			}
			tj := w.makePendingTryjob(ctx, def)
			So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
			}, nil), ShouldBeNil)
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			So(tryjobs[0].ExternalID, ShouldBeEmpty)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_UNTRIGGERED)
			So(tryjobs[0].UntriggeredReason, ShouldEqual, "received NotFound from buildbucket. message: builder testProj/BucketFoo/non-existent-builder not found")
			So(w.logEntries, ShouldResembleProto, []*tryjob.ExecutionLogEntry{
				{
					Time: timestamppb.New(ct.Clock.Now().UTC()),
					Kind: &tryjob.ExecutionLogEntry_TryjobsLaunchFailed_{
						TryjobsLaunchFailed: &tryjob.ExecutionLogEntry_TryjobsLaunchFailed{
							Tryjobs: []*tryjob.ExecutionLogEntry_TryjobLaunchFailed{
								{
									Definition: def,
									Reason:     tryjobs[0].UntriggeredReason,
								},
							},
						},
					},
				},
			})
		})

		Convey("Reconcile with existing", func() {
			tj := w.makePendingTryjob(ctx, defFoo)
			tj.LaunchedBy = runID
			reuseRun := common.MakeRunID(lProject, ct.Clock.Now().Add(-30*time.Minute), 1, []byte("beef"))
			tj.ReusedBy = append(tj.ReusedBy, reuseRun)
			So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
			}, nil), ShouldBeNil)
			existingTryjobID := tj.ID + 59
			w.backend = &decoratedBackend{
				TryjobBackend: w.backend,
				launchedTryjobsHook: func(tryjobs []*tryjob.Tryjob) {
					So(tryjobs, ShouldHaveLength, 1)
					// Save a tryjob that has the same external ID but different internal
					// ID from the input tryjob.
					originalID := tj.ID
					tryjobs[0].ID = existingTryjobID
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
					}, nil), ShouldBeNil)
					tryjobs[0].ID = originalID
				},
			}

			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			So(tryjobs[0].ID, ShouldEqual, existingTryjobID)
			So(tryjobs[0].ExternalID.MustLoad(ctx).ID, ShouldEqual, existingTryjobID)
			// Check the dropped tryjob
			tj = &tryjob.Tryjob{ID: tj.ID}
			So(datastore.Get(ctx, tj), ShouldBeNil)
			So(tj.Status, ShouldEqual, tryjob.Status_UNTRIGGERED)
		})
		Convey("Launched Tryjob has CL in submission order", func() {
			depCL := &run.RunCL{
				ID: clid + 1,
				Detail: &changelist.Snapshot{
					Patchset:              gPatchset,
					MinEquivalentPatchset: gMinPatchset,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Project: gRepo,
								Number:  gChange + 1,
								Owner: &gerritpb.AccountInfo{
									Email: "owner@example.com",
								},
							},
						},
					},
				},
				Trigger: &run.Trigger{
					Mode:  string(run.DryRun),
					Email: "triggerer@example.com",
				},
			}
			w.cls[0].Detail.Deps = []*changelist.Dep{
				{Clid: int64(depCL.ID), Kind: changelist.DepKind_HARD},
			}
			w.cls[0].Detail.GetGerrit().GitDeps = []*changelist.GerritGitDep{
				{Change: depCL.Detail.GetGerrit().GetInfo().GetNumber(), Immediate: true},
			}
			w.cls = append(w.cls, depCL)
			tj := w.makePendingTryjob(ctx, defFoo)
			So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}, nil)
			}, nil), ShouldBeNil)
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			eid := tryjobs[0].ExternalID
			So(eid, ShouldNotBeEmpty)
			So(eid.MustLoad(ctx), ShouldNotBeNil)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_TRIGGERED)
			host, buildID, err := eid.ParseBuildbucketID()
			So(err, ShouldBeNil)
			So(host, ShouldEqual, bbHost)
			bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
			So(err, ShouldBeNil)
			build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID})
			So(err, ShouldBeNil)
			So(build.Input.GetGerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
				// Dep CL is listed first even though in the worker it is after the
				// dependent CL.
				{Host: gHost, Project: gRepo, Change: depCL.Detail.GetGerrit().GetInfo().GetNumber(), Patchset: gPatchset},
				{Host: gHost, Project: gRepo, Change: w.cls[0].Detail.GetGerrit().GetInfo().GetNumber(), Patchset: gPatchset},
			})
		})
	})
}

// decoratedBackend allows test to instrument TryjobBackend interface.
type decoratedBackend struct {
	TryjobBackend
	launchedTryjobsHook func([]*tryjob.Tryjob)
}

func (db *decoratedBackend) Launch(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) error {
	if err := db.TryjobBackend.Launch(ctx, tryjobs, r, cls); err != nil {
		return err
	}
	if db.launchedTryjobsHook != nil {
		db.launchedTryjobsHook(tryjobs)
	}
	return nil
}
