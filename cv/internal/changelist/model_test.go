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

package changelist

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist/clpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCL(t *testing.T) {
	t.Parallel()

	Convey("CL", t, func() {
		ctx := memory.Use(context.Background())
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		eid, err := GobID("x-review.example.com", 12)
		Convey("Gerrit ExternalID", func() {
			So(err, ShouldBeNil)
			_, err := GobID("https://example.com", 12)
			So(err, ShouldErrLike, "invalid host")
		})

		Convey("get not exists", func() {
			_, err := eid.Get(ctx)
			So(err, ShouldResemble, datastore.ErrNoSuchEntity)
		})

		Convey("create", func() {
			cl, err := eid.GetOrInsert(ctx, func(cl *CL) {
				cl.Snapshot = &clpb.Snapshot{Patchset: 1}
			})

			Convey("GetOrInsert succeed", func() {
				So(err, ShouldBeNil)
				So(cl.ExternalID, ShouldResemble, eid)
				// ID must be autoset to non-0 value.
				So(cl.ID, ShouldNotEqual, 0)
				So(cl.EVersion, ShouldEqual, 1)
				So(cl.UpdateTime, ShouldResemble, epoch)

				So(cl.Snapshot.Patchset, ShouldEqual, 1)
			})

			Convey("Get exists", func() {
				cl2, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl2.ID, ShouldEqual, cl.ID)
				So(cl2.ExternalID, ShouldEqual, eid)
				So(cl2.EVersion, ShouldEqual, 1)
				So(cl2.UpdateTime, ShouldEqual, cl.UpdateTime)
				So(cl2.Snapshot, ShouldResembleProto, cl.Snapshot)
			})

			Convey("GetOrInsert already exists", func() {
				cl3, err := eid.GetOrInsert(ctx, func(cl *CL) {
					cl.Snapshot = &clpb.Snapshot{Patchset: 999}
				})
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.ExternalID, ShouldResemble, eid)
				So(cl3.EVersion, ShouldEqual, 1)
				So(cl3.UpdateTime, ShouldEqual, cl.UpdateTime)
				So(cl3.Snapshot, ShouldResembleProto, cl.Snapshot)
			})

			Convey("Delete works", func() {
				err := Delete(ctx, cl.ID)
				So(err, ShouldBeNil)
				_, err = eid.Get(ctx)
				So(err, ShouldResemble, datastore.ErrNoSuchEntity)
				So(datastore.Get(ctx, cl), ShouldResemble, datastore.ErrNoSuchEntity)

				Convey("delete is now noop", func() {
					err := Delete(ctx, cl.ID)
					So(err, ShouldBeNil)
				})
			})
		})
	})
}

func TestUpdateSnapshot(t *testing.T) {
	t.Parallel()

	Convey("UpdateSnapshot", t, func() {
		epoch := testclock.TestRecentTimeUTC
		ctx := memory.Use(context.Background())
		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		Convey("new CL is created", func() {
			snap := makeSnapshot(epoch)
			err := UpdateSnapshot(ctx, eid, 0 /* unknown CLID */, snap)
			So(err, ShouldBeNil)
			cl, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl.Snapshot, ShouldResembleProto, snap)
			So(cl.EVersion, ShouldEqual, 1)
		})

		Convey("update existing", func() {
			cl, err := eid.GetOrInsert(ctx, func(*CL) { /* no snapshot attached yet*/ })
			So(err, ShouldBeNil)

			snap := makeSnapshot(epoch)
			err = UpdateSnapshot(ctx, eid, 0 /* unknown CLID */, snap)
			So(err, ShouldBeNil)

			cl2, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl2.ID, ShouldEqual, cl.ID)
			So(cl2.EVersion, ShouldEqual, 2)
			So(cl2.Snapshot, ShouldResembleProto, snap)

			Convey("with known CLID", func() {
				snap2 := makeSnapshot(epoch.Add(time.Minute))
				err = UpdateSnapshot(ctx, "" /*unspecified externalID*/, cl.ID, snap2)
				So(err, ShouldBeNil)

				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.EVersion, ShouldEqual, 3)
				So(cl3.Snapshot, ShouldResembleProto, snap2)
			})

			Convey("skip if not newer", func() {
				snap2 := makeSnapshot(epoch.Add(-time.Minute))
				err = UpdateSnapshot(ctx, "", cl.ID, snap2)
				So(err, ShouldBeNil)

				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.EVersion, ShouldEqual, 2)
				So(cl3.Snapshot, ShouldResembleProto, snap)
			})
		})
	})
}

func TestConcurrentUpdateSnapshot(t *testing.T) {
	t.Parallel()

	Convey("UpdateSnapshot is atomic when called concurrently with flaky datastore", t, func() {
		epoch := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), epoch)
		ctx = txndefer.FilterRDS(memory.Use(ctx))
		ctx, fb := featureBreaker.FilterRDS(ctx, nil)
		datastore.GetTestable(ctx).Consistent(true)

		// Use a single random source for all flaky.Errors(...) instances. Otherwise
		// they repeat the same random pattern each time withBrokenDS is called.
		rnd := rand.NewSource(0)

		// Make datastore very faulty.
		fb.BreakFeaturesWithCallback(
			flaky.Errors(flaky.Params{
				Rand:                             rnd,
				DeadlineProbability:              0.4,
				ConcurrentTransactionProbability: 0.4,
			}),
			featureBreaker.DatastoreFeatures...,
		)
		// Number of tries per worker.
		// With probabilities above, it typically takes <40 tries.
		const R = 200
		// Number of workers.
		const N = 10

		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		wg := sync.WaitGroup{}
		wg.Add(N)
		delays := mathrand.Perm(ctx, N) // 0,1,...,N-1
		for _, d := range delays {
			d := d
			go func() {
				defer wg.Done()
				snap := makeSnapshot(epoch.Add(time.Minute * time.Duration(d)))
				var err error
				for i := 0; i < R; i++ {
					if err = UpdateSnapshot(ctx, eid, 0, snap); err == nil {
						t.Logf("succeeded after %d tries", i)
						return
					}
				}
				panic(errors.Annotate(err, "all %d tries exhausted", R).Err())
			}()
		}
		wg.Wait()

		// "Fix" datastore, letting us examine it.
		fb.BreakFeaturesWithCallback(
			func(context.Context, string) error { return nil },
			featureBreaker.DatastoreFeatures...,
		)
		cl, err := eid.Get(ctx)
		So(err, ShouldBeNil)
		// Since all workers have succeded, the latest snapshot
		// (by ExternalUpdateTime) must be the current snapshot in datastore.
		latest := makeSnapshot(epoch.Add((N - 1) * time.Minute))
		So(cl.Snapshot, ShouldResembleProto, latest)
		// Furthermore, there must have been at most N non-noop UpdateSnapshot
		// calls (one per worker, iff they did exactly in the increasing order of
		// the ExternalUpdateTime).
		t.Logf("%d updates done", cl.EVersion)
		So(cl.EVersion, ShouldBeLessThan, N+1)
	})
}

func makeSnapshot(updatedTime time.Time) *clpb.Snapshot {
	return &clpb.Snapshot{
		ExternalUpdateTime: timestamppb.New(updatedTime),
		Kind: &clpb.Snapshot_Gerrit{Gerrit: &clpb.Gerrit{
			Info: &gerritpb.ChangeInfo{
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number: 1,
						Kind:   gerritpb.RevisionInfo_REWORK,
					},
				},
			},
		}},
	}
}
