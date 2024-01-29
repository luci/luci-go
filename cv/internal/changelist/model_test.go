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
	"fmt"
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

		eid, err := GobID("x-review.example.com", 12)
		So(err, ShouldBeNil)

		Convey("ExternalID.Get returns nil if CL doesn't exist", func() {
			cl, err := eid.Load(ctx)
			So(err, ShouldBeNil)
			So(cl, ShouldBeNil)
		})

		Convey("ExternalID.MustCreateIfNotExists creates a CL", func() {
			cl := eid.MustCreateIfNotExists(ctx)
			So(cl, ShouldNotBeNil)
			So(cl.ExternalID, ShouldResemble, eid)
			// ID must be autoset to non-0 value.
			So(cl.ID, ShouldNotEqual, 0)
			So(cl.EVersion, ShouldEqual, 1)
			So(cl.UpdateTime, ShouldEqual, epoch)
			So(cl.RetentionKey, ShouldEqual, fmt.Sprintf("%02d/%010d", cl.ID%retentionKeyShards, epoch.Unix()))

			Convey("ExternalID.Get loads existing CL", func() {
				cl2, err := eid.Load(ctx)
				So(err, ShouldBeNil)
				So(cl2.ID, ShouldEqual, cl.ID)
				So(cl2.ExternalID, ShouldEqual, eid)
				So(cl2.EVersion, ShouldEqual, 1)
				So(cl2.UpdateTime, ShouldEqual, cl.UpdateTime)
				So(cl2.Snapshot, ShouldResembleProto, cl.Snapshot)
			})

			Convey("ExternalID.MustCreateIfNotExists loads existing CL", func() {
				cl3 := eid.MustCreateIfNotExists(ctx)
				So(cl3, ShouldNotBeNil)
				So(cl3.ID, ShouldEqual, cl.ID)
				So(cl3.ExternalID, ShouldResemble, eid)
				So(cl3.EVersion, ShouldEqual, 1)
				So(cl3.UpdateTime, ShouldEqual, cl.UpdateTime)
				So(cl3.Snapshot, ShouldResembleProto, cl.Snapshot)
			})

			Convey("Delete works", func() {
				err := Delete(ctx, cl.ID)
				So(err, ShouldBeNil)
				// Verify.
				So(datastore.Get(ctx, cl), ShouldResemble, datastore.ErrNoSuchEntity)
				cl2, err2 := eid.Load(ctx)
				So(err2, ShouldBeNil)
				So(cl2, ShouldBeNil)

				Convey("delete is now a noop", func() {
					err := Delete(ctx, cl.ID)
					So(err, ShouldBeNil)
				})
			})
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
				ids[i] = eids[i].MustCreateIfNotExists(ctx).ID
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
