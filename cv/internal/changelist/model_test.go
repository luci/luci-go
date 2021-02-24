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
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"

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
				cl.Snapshot = makeSnapshot(epoch)
			})

			Convey("GetOrInsert succeed", func() {
				So(err, ShouldBeNil)
				So(cl.ExternalID, ShouldResemble, eid)
				// ID must be autoset to non-0 value.
				So(cl.ID, ShouldNotEqual, 0)
				So(cl.EVersion, ShouldEqual, 1)
				So(cl.UpdateTime, ShouldResemble, epoch)
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

func TestLoadMulti(t *testing.T) {
	t.Parallel()

	Convey("LoadMulti works", t, func() {
		ctx := memory.Use(context.Background())
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		saveCL := func(id int) *CL {
			eid, err := GobID("x-review.example.com", int64(id))
			So(err, ShouldBeNil)
			snap := makeSnapshot(epoch.Add(time.Second * time.Duration(id)))
			cl, err := eid.GetOrInsert(ctx, func(cl *CL) { cl.Snapshot = snap })
			So(err, ShouldBeNil)
			return cl
		}

		cls := make([]*CL, 1500)
		usedIDs := make(map[common.CLID]struct{}, len(cls))
		for i := range cls {
			cls[i] = saveCL(i + 1)
			cls[i].Snapshot = nil
			usedIDs[cls[i].ID] = struct{}{}
		}

		err := LoadMulti(ctx, cls)
		So(err, ShouldBeNil)
		for i, cl := range cls {
			t := cl.Snapshot.GetExternalUpdateTime()
			So(t, ShouldNotBeNil)
			So(t.AsTime(), ShouldResemble, epoch.Add(time.Second*time.Duration(i+1)))
		}

		Convey("Simulate not found error for every 13th", func() {
			lastUsed := common.CLID(0)
			nextUnused := func() common.CLID {
				for {
					lastUsed++
					if _, ok := usedIDs[lastUsed]; !ok {
						return lastUsed
					}
				}
			}
			for i, cl := range cls {
				cl.Snapshot = nil
				if i%13 == 0 {
					cl.ID = nextUnused()
				}
			}
			err := LoadMulti(ctx, cls)
			So(err, ShouldNotBeNil)
			merr, _ := err.(errors.MultiError)
			So(merr, ShouldNotBeNil)
			So(merr, ShouldHaveLength, len(cls))
			for i, cl := range cls {
				if i%13 == 0 {
					So(merr[i], ShouldEqual, datastore.ErrNoSuchEntity)
				} else {
					So(merr[i], ShouldBeNil)
					t := cl.Snapshot.GetExternalUpdateTime()
					So(t, ShouldNotBeNil)
					So(t.AsTime(), ShouldResemble, epoch.Add(time.Second*time.Duration(i+1)))
				}
			}
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

func TestLookup(t *testing.T) {
	t.Parallel()

	Convey("Lookup works", t, func() {
		ctx := memory.Use(context.Background())

		const n = 10
		ids := make([]common.CLID, n)
		eids := make([]ExternalID, n)
		for i := range eids {
			eids[i] = MustGobID("x-review.example.com", int64(i+1))
			if i%2 == 0 {
				cl, err := eids[i].GetOrInsert(ctx, func(*CL) {})
				So(err, ShouldBeNil)
				ids[i] = cl.ID
			}
		}

		actual, err := Lookup(ctx, eids)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, ids)
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	Convey("Update works", t, func() {
		epoch := testclock.TestRecentTimeUTC
		ctx := memory.Use(context.Background())
		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		Convey("new CL is created", func() {
			snap := makeSnapshot(epoch)
			acfg := makeApplicableConfig()
			asdep := makeDependentMeta(epoch)
			err := Update(ctx, eid, 0 /* unknown CLID */, UpdateFields{
				Snapshot:         snap,
				ApplicableConfig: acfg,
				AddDependentMeta: asdep,
			}, func(ctx context.Context, cl *CL) error {
				So(datastore.CurrentTransaction(ctx), ShouldNotBeNil)
				So(cl.EVersion, ShouldEqual, 1)
				return nil
			})
			So(err, ShouldBeNil)
			cl, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl.Snapshot, ShouldResembleProto, snap)
			So(cl.ApplicableConfig, ShouldResembleProto, acfg)
			So(cl.DependentMeta, ShouldResembleProto, asdep)
			So(cl.EVersion, ShouldEqual, 1)
		})

		Convey("update existing", func() {
			acfg := makeApplicableConfig()
			cl, err := eid.GetOrInsert(ctx, func(cl *CL) {
				// no snapshot attached yet
				cl.ApplicableConfig = acfg
				cl.DependentMeta = makeDependentMeta(epoch, luciProject, "another-project")
			})
			So(err, ShouldBeNil)

			snap := makeSnapshot(epoch)
			err = Update(ctx, eid, 0 /* unknown CLID */, UpdateFields{Snapshot: snap},
				func(ctx context.Context, cl *CL) error {
					So(datastore.CurrentTransaction(ctx), ShouldNotBeNil)
					So(cl.EVersion, ShouldEqual, 2)
					return nil
				})
			So(err, ShouldBeNil)

			cl2, err := eid.Get(ctx)
			So(err, ShouldBeNil)
			So(cl2.ID, ShouldEqual, cl.ID)
			So(cl2.EVersion, ShouldEqual, 2)
			So(cl2.Snapshot, ShouldResembleProto, snap)
			So(cl2.ApplicableConfig, ShouldResembleProto, makeApplicableConfig())
			// 1 entry should have been removed due to matching snapshot's project.
			asdep := makeDependentMeta(epoch, "another-project")
			So(cl2.DependentMeta, ShouldResembleProto, asdep)

			Convey("with known CLID", func() {
				snap2 := makeSnapshot(epoch.Add(time.Minute))
				err = Update(ctx, "" /*unspecified externalID*/, cl.ID,
					UpdateFields{Snapshot: snap2},
					func(ctx context.Context, cl *CL) error {
						So(datastore.CurrentTransaction(ctx), ShouldNotBeNil)
						So(cl.EVersion, ShouldEqual, 3)
						return nil
					})
				So(err, ShouldBeNil)

				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.EVersion, ShouldEqual, 3)
				So(cl3.Snapshot, ShouldResembleProto, snap2)
				So(cl3.ApplicableConfig, ShouldResembleProto, makeApplicableConfig())
				So(cl3.DependentMeta, ShouldResembleProto, asdep)
			})

			Convey("skip update if same", func() {
				err = Update(ctx, "", cl.ID, UpdateFields{
					Snapshot:         makeSnapshot(epoch.Add(-time.Minute)),
					ApplicableConfig: makeApplicableConfig(),
					AddDependentMeta: makeDependentMeta(epoch.Add(-time.Minute), "another-project"),
				}, func(context.Context, *CL) error {
					panic("must not be called")
				})
				So(err, ShouldBeNil)

				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.EVersion, ShouldEqual, 2)
				So(cl3.Snapshot, ShouldResembleProto, snap)
				So(cl3.ApplicableConfig, ShouldResembleProto, acfg)
				So(cl3.DependentMeta, ShouldResembleProto, asdep)
			})

			Convey("adds/updates DependentMeta", func() {
				asdep3 := makeDependentMeta(epoch.Add(time.Minute), "another-project", "2nd")
				err = Update(ctx, "", cl.ID, UpdateFields{AddDependentMeta: asdep3}, nil)
				So(err, ShouldBeNil)
				cl3, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl3.DependentMeta, ShouldResembleProto, asdep3)

				err = Update(ctx, "", cl.ID, UpdateFields{
					AddDependentMeta: makeDependentMeta(epoch.Add(time.Hour), "2nd", "3rd"),
				}, nil)
				So(err, ShouldBeNil)
				cl4, err := eid.Get(ctx)
				So(err, ShouldBeNil)
				So(cl4.DependentMeta, ShouldResembleProto, &DependentMeta{
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
		// use Seconds with lots of 0s at the end for easy grasp of assertion
		// failures since they are done on protos.
		epoch := (&timestamppb.Timestamp{Seconds: 14500000000}).AsTime()
		ctx, _ := testclock.UseTime(context.Background(), epoch)
		ctx = txndefer.FilterRDS(memory.Use(ctx))
		ctx, fb := featureBreaker.FilterRDS(ctx, nil)
		datastore.GetTestable(ctx).Consistent(true)
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}

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
			// Simulate opposing Snapshot and DependentMeta timestamps for better
			// test coverage.

			// For a co-prime p,N:
			//   assert sorted(set([((p*d)%N) for d in xrange(N)])) == range(N)
			// 47, 59 are actual primes.
			snapTS := epoch.Add(time.Second * time.Duration((47*d)%N))
			asdepTS := epoch.Add(time.Second * time.Duration((73*d)%N))
			go func() {
				defer wg.Done()
				snap := makeSnapshot(snapTS)
				asdep := makeDependentMeta(asdepTS, "another-project")
				var err error
				for i := 0; i < R; i++ {
					if err = Update(ctx, eid, 0, UpdateFields{snap, nil, asdep}, nil); err == nil {
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
		latestTS := epoch.Add((N - 1) * time.Second)
		So(cl.Snapshot, ShouldResembleProto, makeSnapshot(latestTS))
		So(cl.DependentMeta, ShouldResembleProto, makeDependentMeta(latestTS, "another-project"))
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
		MinEquivalentPatchset: 1,
		Patchset:              2,
		LuciProject:           luciProject,
	}
}

func makeApplicableConfig() *ApplicableConfig {
	return &ApplicableConfig{
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
