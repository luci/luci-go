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
	"go.chromium.org/luci/common/testing/ftt"
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

func TestLaunch(t *testing.T) {
	t.Parallel()

	ftt.Run("Launch", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject     = "testProj"
			bbHost       = "buildbucket.example.com"
			buildID      = 9524107902457
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
					Host:    bbHost,
					Builder: builder,
				},
			},
		}
		ct.BuildbucketFake.AddBuilder(bbHost, builder, nil)
		t.Run("Works", func(t *ftt.Test) {
			tj := w.makePendingTryjob(ctx, defFoo)
			assert.NoErr(t, datastore.Put(ctx, tj))
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			assert.NoErr(t, err)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			eid := tryjobs[0].ExternalID
			assert.Loosely(t, eid, should.NotBeEmpty)
			assert.Loosely(t, eid.MustLoad(ctx), should.NotBeNil)
			assert.Loosely(t, tryjobs[0].Status, should.Equal(tryjob.Status_TRIGGERED))
			host, buildID, err := eid.ParseBuildbucketID()
			assert.NoErr(t, err)
			assert.Loosely(t, host, should.Equal(bbHost))
			bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
			assert.NoErr(t, err)
			build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID})
			assert.NoErr(t, err)
			assert.Loosely(t, build, should.NotBeNil)
			assert.That(t, w.logEntries, should.Match([]*tryjob.ExecutionLogEntry{
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
			}))
		})

		t.Run("Failed to trigger", func(t *ftt.Test) {
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
			assert.NoErr(t, datastore.Put(ctx, tj))
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			assert.NoErr(t, err)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			assert.Loosely(t, tryjobs[0].ExternalID, should.BeEmpty)
			assert.Loosely(t, tryjobs[0].Status, should.Equal(tryjob.Status_UNTRIGGERED))
			assert.Loosely(t, tryjobs[0].UntriggeredReason, should.Equal("received NotFound from buildbucket. message: builder testProj/BucketFoo/non-existent-builder not found"))
			assert.That(t, w.logEntries, should.Match([]*tryjob.ExecutionLogEntry{
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
			}))
		})

		t.Run("Reconcile with existing", func(t *ftt.Test) {
			tj := w.makePendingTryjob(ctx, defFoo)
			tj.LaunchedBy = runID
			reuseRun := common.MakeRunID(lProject, ct.Clock.Now().Add(-30*time.Minute), 1, []byte("beef"))
			tj.ReusedBy = append(tj.ReusedBy, reuseRun)
			assert.NoErr(t, datastore.Put(ctx, tj))
			var existingTryjobID common.TryjobID
			w.backend = &decoratedBackend{
				TryjobBackend: w.backend,
				launchedTryjobsHook: func(tryjobs []*tryjob.Tryjob, launchResults []*tryjob.LaunchResult) {
					assert.Loosely(t, tryjobs, should.HaveLength(1))
					assert.Loosely(t, launchResults, should.HaveLength(1))
					// Save a tryjob that has the same external ID but different internal
					// ID from the input tryjob.
					existingTryjob, err := w.mutator.Upsert(ctx, launchResults[0].ExternalID, func(tj *tryjob.Tryjob) error {
						tj.Definition = tryjobs[0].Definition
						tj.Status = launchResults[0].Status
						tj.Result = launchResults[0].Result
						return nil
					})
					assert.NoErr(t, err)
					existingTryjobID = existingTryjob.ID
				},
			}

			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			assert.NoErr(t, err)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			assert.Loosely(t, tryjobs[0].ID, should.Equal(existingTryjobID))
			assert.Loosely(t, tryjobs[0].ExternalID.MustLoad(ctx).ID, should.Equal(existingTryjobID))
			// Check the dropped tryjob
			tj = &tryjob.Tryjob{ID: tj.ID}
			assert.Loosely(t, datastore.Get(ctx, tj), should.BeNil)
			assert.Loosely(t, tj.Status, should.Equal(tryjob.Status_UNTRIGGERED))
		})
		t.Run("Launched Tryjob has CL in submission order", func(t *ftt.Test) {
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
			assert.NoErr(t, datastore.Put(ctx, tj))
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			assert.NoErr(t, err)
			assert.Loosely(t, tryjobs, should.HaveLength(1))
			eid := tryjobs[0].ExternalID
			assert.Loosely(t, eid, should.NotBeEmpty)
			assert.Loosely(t, eid.MustLoad(ctx), should.NotBeNil)
			assert.Loosely(t, tryjobs[0].Status, should.Equal(tryjob.Status_TRIGGERED))
			host, buildID, err := eid.ParseBuildbucketID()
			assert.NoErr(t, err)
			assert.Loosely(t, host, should.Equal(bbHost))
			bbClient, err := ct.BuildbucketFake.NewClientFactory().MakeClient(ctx, bbHost, lProject)
			assert.NoErr(t, err)
			build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID})
			assert.NoErr(t, err)
			assert.That(t, build.Input.GetGerritChanges(), should.Match([]*bbpb.GerritChange{
				// Dep CL is listed first even though in the worker it is after the
				// dependent CL.
				{Host: gHost, Project: gRepo, Change: depCL.Detail.GetGerrit().GetInfo().GetNumber(), Patchset: gPatchset},
				{Host: gHost, Project: gRepo, Change: w.cls[0].Detail.GetGerrit().GetInfo().GetNumber(), Patchset: gPatchset},
			}))
		})
	})
}

// decoratedBackend allows test to instrument TryjobBackend interface.
type decoratedBackend struct {
	TryjobBackend
	launchedTryjobsHook func([]*tryjob.Tryjob, []*tryjob.LaunchResult)
}

func (db *decoratedBackend) Launch(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) []*tryjob.LaunchResult {
	launchResults := db.TryjobBackend.Launch(ctx, tryjobs, r, cls)
	if db.launchedTryjobsHook != nil {
		db.launchedTryjobsHook(tryjobs, launchResults)
	}
	return launchResults
}
