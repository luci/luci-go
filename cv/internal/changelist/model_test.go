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
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

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
				cl.Snapshot = &Snapshot{Patchset: 1}
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
					cl.Snapshot = &Snapshot{Patchset: 999}
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

func TestExternalID(t *testing.T) {
	t.Parallel()

	Convey("ExternalID works", t, func() {

		Convey("GobID", func() {
			eid, err := GobID("x-review.example.com", 12)
			So(err, ShouldBeNil)
			So(eid, ShouldResemble, ExternalID("gerrit/x-review.example.com/12"))

			host, change, err := eid.ParseGobID()
			So(err, ShouldBeNil)
			So(host, ShouldResemble, "x-review.example.com")
			So(change, ShouldEqual, 12)

		})
		Convey("Invalid GobID", func() {
			_, _, err := ExternalID("meh").ParseGobID()
			So(err, ShouldErrLike, "is not a valid GobID")

			_, _, err = ExternalID("gerrit/x/y").ParseGobID()
			So(err, ShouldErrLike, "is not a valid GobID")
		})
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	Convey("UpdateSnapshot", t, func() {
		epoch := testclock.TestRecentTimeUTC
		ctx := memory.Use(context.Background())
		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		Convey("new CL is created", func() {
			snap := makeSnapshot(epoch)
			acfg := makeApplicableConfig(epoch)
			asdep := makeDependentMeta(epoch)
			err := Update(ctx, eid, 0 /* unknown CLID */, UpdateFields{
				Snapshot:         snap,
				ApplicableConfig: acfg,
				AddDependentMeta: asdep,
			})
			So(err, ShouldBeNil)
			cl, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl.Snapshot, ShouldResembleProto, snap)
			So(cl.ApplicableConfig, ShouldResembleProto, acfg)
			So(cl.AsDepMeta, ShouldResembleProto, asdep)
			So(cl.EVersion, ShouldEqual, 1)
		})

		Convey("update existing", func() {
			acfg := makeApplicableConfig(epoch)
			cl, err := eid.GetOrInsert(ctx, func(cl *CL) {
				// no snapshot attached yet
				cl.ApplicableConfig = acfg
				cl.AsDepMeta = makeDependentMeta(epoch, luciProject, "another-project")
			})
			So(err, ShouldBeNil)

			snap := makeSnapshot(epoch)
			err = Update(ctx, eid, 0 /* unknown CLID */, UpdateFields{Snapshot: snap})
			So(err, ShouldBeNil)

			cl2, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl2.ID, ShouldEqual, cl.ID)
			So(cl2.EVersion, ShouldEqual, 2)
			So(cl2.Snapshot, ShouldResembleProto, snap)
			So(cl2.ApplicableConfig, ShouldResembleProto, makeApplicableConfig(epoch))
			// 1 entry should have been removed due to matching snapshot's project.
			asdep := makeDependentMeta(epoch, "another-project")
			So(cl2.AsDepMeta, ShouldResembleProto, asdep)

			Convey("with known CLID", func() {
				acfg2 := makeApplicableConfig(epoch.Add(time.Minute))
				err = Update(ctx, "" /*unspecified externalID*/, cl.ID,
					UpdateFields{ApplicableConfig: acfg2})
				So(err, ShouldBeNil)

				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.EVersion, ShouldEqual, 3)
				So(cl3.Snapshot, ShouldResembleProto, snap)
				So(cl3.ApplicableConfig, ShouldResembleProto, acfg2)
				So(cl3.AsDepMeta, ShouldResembleProto, asdep)
			})

			Convey("skip if not newer", func() {
				err = Update(ctx, "", cl.ID, UpdateFields{
					Snapshot:         makeSnapshot(epoch.Add(-time.Minute)),
					ApplicableConfig: makeApplicableConfig(epoch.Add(-time.Minute)),
					AddDependentMeta: makeDependentMeta(epoch.Add(-time.Minute), "another-project"),
				})
				So(err, ShouldBeNil)

				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.EVersion, ShouldEqual, 2)
				So(cl3.Snapshot, ShouldResembleProto, snap)
				So(cl3.ApplicableConfig, ShouldResembleProto, acfg)
				So(cl3.AsDepMeta, ShouldResembleProto, asdep)
			})

			Convey("adds/updates AsDepMeta", func() {
				asdep3 := makeDependentMeta(epoch.Add(time.Minute), "another-project", "2nd")
				err = Update(ctx, "", cl.ID, UpdateFields{AddDependentMeta: asdep3})
				So(err, ShouldBeNil)
				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.AsDepMeta, ShouldResembleProto, asdep3)

				err = Update(ctx, "", cl.ID, UpdateFields{
					AddDependentMeta: makeDependentMeta(epoch.Add(time.Hour), "2nd", "3rd"),
				})
				So(err, ShouldBeNil)
				cl4, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl4.AsDepMeta, ShouldResembleProto, &DependentMeta{
					ByProject: map[string]*DependentMeta_Meta{
						"another-project": {
							UpdateTime: timestamppb.New(epoch.Add(time.Minute)),
							NoAccess:   true,
						},
						"2nd": {
							UpdateTime: timestamppb.New(epoch.Add(time.Hour)),
							NoAccess:   true,
						},
						"3rd": {
							UpdateTime: timestamppb.New(epoch.Add(time.Hour)),
							NoAccess:   true,
						},
					},
				})
			})
		})
	})
}

func TestConcurrentUpdate(t *testing.T) {
	t.Parallel()

	Convey("Update is atomic when called concurrently with flaky datastore", t, func() {
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
		// With probabilities above, it typically takes <60 tries.
		const R = 200
		// Number of workers.
		const N = 10

		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		wg := sync.WaitGroup{}
		wg.Add(N)
		for d := 0; d < N; d++ {
			// Simulate opposing Snapshot and ApplicableConfig timestamps for better
			// test coverage.

			// For a co-prime p,N:
			//   assert sorted(set([((p*d)%N) for d in xrange(N)])) == range(N)
			// 47, 59, 67 are actual primes.
			snapTS := epoch.Add(time.Minute * time.Duration((47*d)%N))
			acfgTS := epoch.Add(time.Minute * time.Duration((59*d)%N))
			asdepTS := epoch.Add(time.Minute * time.Duration((67*d)%N))
			go func() {
				defer wg.Done()
				snap := makeSnapshot(snapTS)
				acfg := makeApplicableConfig(acfgTS)
				asdep := makeDependentMeta(asdepTS, "another-project")
				var err error
				for i := 0; i < R; i++ {
					if err = Update(ctx, eid, 0, UpdateFields{snap, acfg, asdep}); err == nil {
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
		latestTS := epoch.Add((N - 1) * time.Minute)
		So(cl.Snapshot, ShouldResembleProto, makeSnapshot(latestTS))
		So(cl.ApplicableConfig, ShouldResembleProto, makeApplicableConfig(latestTS))
		So(cl.AsDepMeta, ShouldResembleProto, makeDependentMeta(latestTS, "another-project"))
		// Furthermore, there must have been at most N non-noop UpdateSnapshot
		// calls (one per worker, iff they did exactly in the increasing order of
		// the ExternalUpdateTime).
		t.Logf("%d updates done", cl.EVersion)
		So(cl.EVersion, ShouldBeLessThan, N+1)
	})
}

const luciProject = "luci-project"

func makeSnapshot(updatedTime time.Time) *Snapshot {
	return &Snapshot{
		ExternalUpdateTime: timestamppb.New(updatedTime),
		Kind: &Snapshot_Gerrit{Gerrit: &Gerrit{
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
		LuciProject: luciProject,
	}
}

func makeApplicableConfig(updatedTime time.Time) *ApplicableConfig {
	return &ApplicableConfig{
		UpdateTime: timestamppb.New(updatedTime),
		Projects: []*ApplicableConfig_Project{
			{Name: luciProject, ConfigGroupIds: []string{"blah"}},
		},
	}
}

func makeDependentMeta(updatedTime time.Time, projects ...string) *DependentMeta {
	if len(projects) == 0 {
		projects = []string{luciProject}
	}
	a := &DependentMeta{ByProject: make(map[string]*DependentMeta_Meta, len(projects))}
	for _, p := range projects {
		a.ByProject[p] = &DependentMeta_Meta{
			NoAccess:   true,
			UpdateTime: timestamppb.New(updatedTime),
		}
	}
	return a
}
