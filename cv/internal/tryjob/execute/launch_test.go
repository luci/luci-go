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
	"sync"
	"testing"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	bbfacade "go.chromium.org/luci/cv/internal/buildbucket/facade"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLaunch(t *testing.T) {
	t.Parallel()

	Convey("Launch", t, func() {
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

		fakeRM := &fakeRM{}
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
			rm: fakeRM,
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
			So(tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}), ShouldBeNil)
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
			ct.Clock.SetTimerCallback(func(dur time.Duration, timer clock.Timer) {
				for _, tag := range testclock.GetTags(timer) {
					if tag == launchRetryClockTag {
						ct.Clock.Add(dur)
					}
				}
			})
			So(tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}), ShouldBeNil)
			tryjobs, err := w.launchTryjobs(ctx, []*tryjob.Tryjob{tj})
			So(err, ShouldBeNil)
			So(tryjobs, ShouldHaveLength, 1)
			So(tryjobs[0].ExternalID, ShouldBeEmpty)
			So(tryjobs[0].Status, ShouldEqual, tryjob.Status_UNTRIGGERED)
			So(tryjobs[0].UntriggeredReason, ShouldEqual, "received NotFound from buildbucket. message: builder testProj/BucketFoo/non-existent-builder not found")
		})

		Convey("Reconcile with existing", func() {
			tj := w.makePendingTryjob(ctx, defFoo)
			tj.TriggeredBy = runID
			reuseRun := common.MakeRunID(lProject, ct.Clock.Now().Add(-30*time.Minute), 1, []byte("beef"))
			tj.ReusedBy = append(tj.ReusedBy, reuseRun)
			So(tryjob.SaveTryjobs(ctx, []*tryjob.Tryjob{tj}), ShouldBeNil)
			existingTryjobID := tj.ID + 59
			w.backend = &decoratedBackend{
				TryjobBackend: w.backend,
				launchedTryjobsHook: func(tryjobs []*tryjob.Tryjob) {
					So(tryjobs, ShouldHaveLength, 1)
					// Save a tryjob that has the same external ID but different internal
					// ID from the input tryjob.
					originalID := tj.ID
					tryjobs[0].ID = existingTryjobID
					So(tryjob.SaveTryjobs(ctx, tryjobs), ShouldBeNil)
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
			So(fakeRM.notified, ShouldContainKey, reuseRun)
			So(fakeRM.notified[reuseRun], ShouldResemble, common.TryjobIDs{tj.ID})
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

type fakeRM struct {
	mu       sync.Mutex
	notified map[common.RunID]common.TryjobIDs
}

func (rm *fakeRM) NotifyTryjobsUpdated(ctx context.Context, runID common.RunID, events *tryjob.TryjobUpdatedEvents) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.notified == nil {
		rm.notified = make(map[common.RunID]common.TryjobIDs, 1)
	}
	for _, event := range events.GetEvents() {
		rm.notified[runID] = append(rm.notified[runID], common.TryjobID(event.GetTryjobId()))
	}
	return nil
}
