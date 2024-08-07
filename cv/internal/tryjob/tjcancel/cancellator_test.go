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

package tjcancel

import (
	"context"
	"crypto/sha1"
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskHandler(t *testing.T) {
	Convey("handleTask", t, func() {
		Convey("panics", func() {
			c := &Cancellator{}
			ctx := context.Background()

			panicker := func() {
				_ = c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     42,
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  2,
				})
			}
			So(panicker, ShouldPanicLike, "patchset numbers expected to increase")
		})
		Convey("works with", func() {
			ct := &cvtesting.Test{}
			ctx := ct.SetUp(t)
			n := tryjob.NewNotifier(ct.TQDispatcher)
			c := NewCancellator(n)
			mb := &mockBackend{}
			c.RegisterBackend(mb)
			const clid = common.CLID(100)
			cl := &changelist.CL{ID: clid}
			So(datastore.Put(ctx, cl), ShouldBeNil)
			Convey("no tryjobs", func() {
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				So(err, ShouldBeNil)
				So(mb.calledWith, ShouldHaveLength, 0)
			})
			Convey("all tryjobs ended", func() {
				tj1 := putTryjob(ctx, clid, 2, tryjob.Status_ENDED, 1, run.Status_FAILED, nil)
				tj2 := putTryjob(ctx, clid, 2, tryjob.Status_ENDED, 2, run.Status_CANCELLED, nil)
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				So(err, ShouldBeNil)
				// Should not call backend.
				So(mb.calledWith, ShouldHaveLength, 0)

				So(datastore.Get(ctx, tj1, tj2), ShouldBeNil)
				// Should not modify entities.
				So(tj1.EVersion, ShouldEqual, 1)
				So(tj2.EVersion, ShouldEqual, 1)
			})
			Convey("some tryjobs ended, others cancellable", func() {
				tj11 := putTryjob(ctx, clid, 2, tryjob.Status_ENDED, 11, run.Status_FAILED, nil)
				tj12 := putTryjob(ctx, clid, 2, tryjob.Status_TRIGGERED, 12, run.Status_CANCELLED, nil)
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				So(err, ShouldBeNil)
				// Should call backend once, with tj12.
				So(mb.calledWith, ShouldHaveLength, 1)
				So(mb.calledWith[0].ExternalID, ShouldEqual, tj12.ExternalID)

				So(datastore.Get(ctx, tj11, tj12), ShouldBeNil)
				// Should modify only tj12.
				So(tj11.EVersion, ShouldEqual, 1)
				So(tj12.EVersion, ShouldEqual, 2)
				So(tj12.Status, ShouldEqual, tryjob.Status_CANCELLED)
			})
			Convey("tryjob still watched", func() {
				tj21 := putTryjob(ctx, clid, 2, tryjob.Status_TRIGGERED, 21, run.Status_RUNNING, nil)
				task := &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				}
				err := c.handleTask(ctx, task)
				So(err, ShouldBeNil)
				// Should not call backend.
				So(mb.calledWith, ShouldHaveLength, 0)

				So(datastore.Get(ctx, tj21), ShouldBeNil)
				// Should not modify the entity.
				So(tj21.EVersion, ShouldEqual, 1)
				So(tj21.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(ct.TQ.Tasks(), ShouldHaveLength, 1)
				So(ct.TQ.Tasks()[0].Payload, ShouldResembleProto, task)
				So(ct.TQ.Tasks()[0].ETA, ShouldEqual, ct.Clock.Now().Add(cancelLaterDuration))
			})
			Convey("tryjob not triggered by cv", func() {
				tj31 := putTryjob(ctx, clid, 2, tryjob.Status_TRIGGERED, 31, run.Status_CANCELLED, func(tj *tryjob.Tryjob) {
					tj.LaunchedBy = ""
				})
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				So(err, ShouldBeNil)
				// Should not call backend.
				So(mb.calledWith, ShouldHaveLength, 0)
				So(datastore.Get(ctx, tj31), ShouldBeNil)
				// Should not modify the entity.
				So(tj31.EVersion, ShouldEqual, 1)
				So(tj31.Status, ShouldNotEqual, tryjob.Status_CANCELLED)
			})
			Convey("tryjob configured to skip stale check", func() {
				tj41 := putTryjob(ctx, clid, 2, tryjob.Status_TRIGGERED, 41, run.Status_CANCELLED, func(tj *tryjob.Tryjob) {
					tj.Definition.SkipStaleCheck = true
				})
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				So(err, ShouldBeNil)
				// Should not call backend.
				So(mb.calledWith, ShouldHaveLength, 0)
				So(datastore.Get(ctx, tj41), ShouldBeNil)
				// Should not modify the entity.
				So(tj41.EVersion, ShouldEqual, 1)
				So(tj41.Status, ShouldNotEqual, tryjob.Status_CANCELLED)
			})
			Convey("CL has Cq-Do-Not-Cancel-Tryjobs footer", func() {
				tj51 := putTryjob(ctx, clid, 2, tryjob.Status_TRIGGERED, 12, run.Status_CANCELLED, nil)
				cl.Snapshot = &changelist.Snapshot{
					Metadata: []*changelist.StringPair{
						{
							Key:   common.FooterCQDoNotCancelTryjobs,
							Value: "True",
						},
					},
				}
				So(datastore.Put(ctx, cl), ShouldBeNil)
				err := c.handleTask(ctx, &tryjob.CancelStaleTryjobsTask{
					Clid:                     int64(clid),
					PreviousMinEquivPatchset: 2,
					CurrentMinEquivPatchset:  5,
				})
				So(err, ShouldBeNil)
				// Should not call backend.
				So(mb.calledWith, ShouldHaveLength, 0)
				So(datastore.Get(ctx, tj51), ShouldBeNil)
				// Should not modify the entity.
				So(tj51.EVersion, ShouldEqual, 1)
				So(tj51.Status, ShouldNotEqual, tryjob.Status_CANCELLED)
			})
		})
	})
}

// putTryjob creates a mock Tryjob and its triggering Run.
//
// It must be called inside a Convey() context as it contains
// assertions.
func putTryjob(ctx context.Context, clid common.CLID, patchset int32, tjStatus tryjob.Status, buildNumber int64, runStatus run.Status, modify func(*tryjob.Tryjob)) *tryjob.Tryjob {
	now := datastore.RoundTime(clock.Now(ctx).UTC())
	tjID := tryjob.MustBuildbucketID("test.com", buildNumber)
	digest := mockDigest(string(tjID))
	r := &run.Run{
		ID:     common.MakeRunID("test", now, 1, digest),
		Status: runStatus,
	}
	So(datastore.Put(ctx, r), ShouldBeNil)
	tj := &tryjob.Tryjob{
		ExternalID:       tjID,
		CLPatchsets:      []tryjob.CLPatchset{tryjob.MakeCLPatchset(clid, patchset)},
		Status:           tjStatus,
		EVersion:         1,
		EntityCreateTime: now,
		EntityUpdateTime: now,
		LaunchedBy:       r.ID,
		Definition:       &tryjob.Definition{},
	}
	if modify != nil {
		modify(tj)
	}
	So(datastore.Put(ctx, tj), ShouldBeNil)
	return tj
}

// mockDigest hashes a string.
func mockDigest(s string) []byte {
	h := sha1.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}

type mockBackend struct {
	calledWith []*tryjob.Tryjob
}

func (mb *mockBackend) Kind() string {
	return "buildbucket"
}

func (mb *mockBackend) CancelTryjob(ctx context.Context, tj *tryjob.Tryjob, reason string) error {
	mb.calledWith = append(mb.calledWith, tj)
	return nil
}
