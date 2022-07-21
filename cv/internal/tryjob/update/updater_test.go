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

package tjupdate

import (
	"context"
	"math/rand"
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type mockRN struct {
	notifiedRuns common.RunIDs
}

func (r *mockRN) NotifyTryjobsUpdated(ctx context.Context, run common.RunID, _ *tryjob.TryjobUpdatedEvents) error {
	r.notifiedRuns = append(r.notifiedRuns, run)
	return nil
}

type returnValues struct {
	s   tryjob.Status
	r   *tryjob.Result
	err error
}
type mockBackend struct {
	returns []*returnValues
}

func (b *mockBackend) Update(ctx context.Context, saved *tryjob.Tryjob) (tryjob.Status, *tryjob.Result, error) {
	var ret *returnValues
	switch len(b.returns) {
	case 0:
		return 0, nil, nil
	case 1:
		ret = b.returns[0]
	default:
		ret = b.returns[0]
		b.returns = b.returns[1:]
	}
	return ret.s, ret.r, ret.err
}

func (b *mockBackend) Kind() string {
	return "buildbucket"
}

func makeTryjob(ctx context.Context) (*tryjob.Tryjob, error) {
	return makeTryjobWithStatus(ctx, tryjob.Status_TRIGGERED)
}

func makeTestRunID(ctx context.Context, someNumber int64) common.RunID {
	mockDigest := []byte{
		byte(0xff & someNumber),
		byte(0xff & (someNumber >> 8)),
		byte(0xff & (someNumber >> 16)),
		byte(0xff & (someNumber >> 24))}
	return common.MakeRunID("test", clock.Now(ctx), 1, mockDigest)
}

func makeTryjobWithStatus(ctx context.Context, status tryjob.Status) (*tryjob.Tryjob, error) {
	buildID := int64(rand.Int31())
	return makeTryjobWithDetails(ctx, buildID, status, makeTestRunID(ctx, buildID), nil)
}

func makeTryjobWithDetails(ctx context.Context, buildID int64, status tryjob.Status, triggerer common.RunID, reusers common.RunIDs) (*tryjob.Tryjob, error) {
	tj := tryjob.MustBuildbucketID("cr-buildbucket.example.com", buildID).MustCreateIfNotExists(ctx)
	tj.LaunchedBy = triggerer
	tj.ReusedBy = reusers
	tj.Status = status
	return tj, datastore.Put(ctx, tj)
}

func TestHandleTask(t *testing.T) {
	Convey("HandleTryjobUpdateTask", t, func() {
		ct := cvtesting.Test{}
		ctx, clean := ct.SetUp()
		defer clean()

		rn := &mockRN{}
		mb := &mockBackend{}
		updater := NewUpdater(tryjob.NewNotifier(ct.TQDispatcher), rn)
		updater.RegisterBackend(mb)

		Convey("noop", func() {
			tj, err := makeTryjob(ctx)
			So(err, ShouldBeNil)
			eid := tj.ExternalID
			originalEVersion := tj.EVersion
			mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, nil, nil}}

			So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{ExternalId: string(eid)}), ShouldBeNil)
			So(rn.notifiedRuns, ShouldHaveLength, 0)

			// Reload to ensure no changes took place.
			tj = eid.MustCreateIfNotExists(ctx)
			So(tj.EVersion, ShouldEqual, originalEVersion)
		})
		Convey("succeeds updating", func() {
			Convey("status and result", func() {
				tj, err := makeTryjob(ctx)
				So(err, ShouldBeNil)
				originalEVersion := tj.EVersion
				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}
				Convey("by internal id", func() {
					So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
						Id: int64(tj.ID),
					}), ShouldBeNil)
				})
				Convey("by external id", func() {
					So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
						ExternalId: string(tj.ExternalID),
					}), ShouldBeNil)
				})
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				// Ensure status updated.
				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_ENDED)
				So(tj.Result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
			})

			Convey("result only", func() {
				tj, err := makeTryjob(ctx)
				So(err, ShouldBeNil)

				originalEVersion := tj.EVersion
				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(tj.Result.Status, ShouldEqual, tryjob.Result_SUCCEEDED)
			})

			Convey("status only", func() {
				tj, err := makeTryjobWithStatus(ctx, tryjob.Status_PENDING)
				So(err, ShouldBeNil)

				originalEVersion := tj.EVersion
				mb.returns = []*returnValues{{tryjob.Status_TRIGGERED, nil, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, tj.LaunchedBy)

				tj = tj.ExternalID.MustCreateIfNotExists(ctx)
				So(tj.EVersion, ShouldEqual, originalEVersion+1)
				So(tj.Status, ShouldEqual, tryjob.Status_TRIGGERED)
				So(tj.Result, ShouldBeNil)
			})
			Convey("and notifying triggerer and reuser Runs", func() {
				buildID := int64(rand.Int31())
				triggerer := makeTestRunID(ctx, buildID)
				reusers := common.RunIDs{makeTestRunID(ctx, buildID+100), makeTestRunID(ctx, buildID+200)}
				tj, err := makeTryjobWithDetails(ctx, buildID, tryjob.Status_TRIGGERED, triggerer, reusers)
				So(err, ShouldBeNil)

				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				// Should have called notifier thrice.
				So(rn.notifiedRuns, ShouldHaveLength, 3)
				So(rn.notifiedRuns.Equal(common.RunIDs{triggerer, reusers[0], reusers[1]}), ShouldBeTrue)
			})
			Convey("and notifying reuser Run with no triggerer Run", func() {
				buildID := int64(rand.Int31())
				reusers := common.RunIDs{makeTestRunID(ctx, buildID+100)}
				tj, err := makeTryjobWithDetails(ctx, buildID, tryjob.Status_TRIGGERED, "", reusers)
				So(err, ShouldBeNil)

				mb.returns = []*returnValues{{tryjob.Status_ENDED, &tryjob.Result{Status: tryjob.Result_SUCCEEDED}, nil}}
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{Id: int64(tj.ID)}), ShouldBeNil)
				So(rn.notifiedRuns, ShouldHaveLength, 1)
				So(rn.notifiedRuns[0], ShouldEqual, reusers[0])
			})
		})
		Convey("fails to", func() {
			tj, err := makeTryjob(ctx)
			So(err, ShouldBeNil)

			Convey("update a tryjob with an id that doesn't exist", func() {
				So(datastore.Delete(ctx, tj), ShouldBeNil)
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					Id: int64(tj.ID),
				}), ShouldErrLike, "unknown Tryjob with id")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
			Convey("update a tryjob with an external id that doesn't exist", func() {
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					ExternalId: string(tryjob.MustBuildbucketID("does-not-exist.example.com", 1)),
				}), ShouldErrLike, "unknown Tryjob with ExternalID")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
			Convey("update a tryjob with neither internal nor external id", func() {
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{}), ShouldErrLike, "expected at least one of {Id, ExternalId}")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
			Convey("update a tryjob with mismatching internal and external ids", func() {
				So(updater.handleTask(ctx, &tryjob.UpdateTryjobTask{
					Id:         int64(tj.ID),
					ExternalId: string(tryjob.MustBuildbucketID("cr-buildbucket.example.com", 1)),
				}), ShouldErrLike, "the given internal and external ids for the tryjob do not match")
				So(rn.notifiedRuns, ShouldHaveLength, 0)
			})
		})
	})
}
