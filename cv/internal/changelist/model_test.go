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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
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

		const luciProject = "luci-project"

		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		Convey("Gerrit ExternalID", func() {
			u, err := eid.URL()
			So(err, ShouldBeNil)
			So(u, ShouldEqual, "https://x-review.example.com/12")

			_, err = GobID("https://example.com", 12)
			So(err, ShouldErrLike, "invalid host")
		})

		Convey("get not exists", func() {
			_, err := eid.Get(ctx)
			So(err, ShouldResemble, datastore.ErrNoSuchEntity)
		})

		Convey("create", func() {
			cl, err := eid.GetOrInsert(ctx, func(cl *CL) {
				cl.Snapshot = makeSnapshot(luciProject, epoch)
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

func makeSnapshot(luciProject string, updatedTime time.Time) *Snapshot {
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
