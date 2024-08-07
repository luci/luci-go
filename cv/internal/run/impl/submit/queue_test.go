// Copyright 2021 The LUCI Authors.
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

package submit

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQueue(t *testing.T) {
	t.Parallel()

	Convey("Queue", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		notifier := &fakeNotifier{}
		const lProject = "lProject"
		run1 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("deaddead"))
		run2 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("beefbeef"))
		run3 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("cafecafe"))
		mustLoadQueue := func() *queue {
			q, err := loadQueue(ctx, lProject)
			So(err, ShouldBeNil)
			return q
		}
		mustTryAcquire := func(ctx context.Context, runID common.RunID, opts *cfgpb.SubmitOptions) bool {
			var waitlisted bool
			var innerErr error
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				waitlisted, innerErr = TryAcquire(ctx, notifier.notify, runID, opts)
				return innerErr
			}, nil)
			So(innerErr, ShouldBeNil)
			So(err, ShouldBeNil)
			return waitlisted
		}

		Convey("TryAcquire", func() {
			Convey("When queue is empty", func() {
				waitlisted := mustTryAcquire(ctx, run1, nil)
				So(waitlisted, ShouldBeFalse)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:      lProject,
					Current: run1,
				})

				Convey("And acquire same Run again", func() {
					waitlisted := mustTryAcquire(ctx, run1, nil)
					So(waitlisted, ShouldBeFalse)
					So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
						ID:      lProject,
						Current: run1,
					})
				})
			})

			Convey("Waitlisted", func() {
				So(datastore.Put(ctx, &queue{
					ID:      lProject,
					Current: run1,
					Opts:    nil,
				}), ShouldBeNil)
				waitlisted := mustTryAcquire(ctx, run2, nil)
				So(waitlisted, ShouldBeTrue)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2},
				})
				waitlisted = mustTryAcquire(ctx, run3, nil)
				So(waitlisted, ShouldBeTrue)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2, run3},
				})

				Convey("And acquire same Run in waitlist again", func() {
					for _, r := range []common.RunID{run2, run3} {
						waitlisted := mustTryAcquire(ctx, r, nil)
						So(waitlisted, ShouldBeTrue)
						So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
							ID:       lProject,
							Current:  run1,
							Waitlist: common.RunIDs{run2, run3},
						})
					}
				})
			})

			Convey("Promote the first in the Waitlist if Current is empty", func() {
				So(datastore.Put(ctx, &queue{
					ID:       lProject,
					Waitlist: common.RunIDs{run2, run3},
					Opts:     nil,
				}), ShouldBeNil)
				waitlisted := mustTryAcquire(ctx, run2, nil)
				So(waitlisted, ShouldBeFalse)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:       lProject,
					Current:  run2,
					Waitlist: common.RunIDs{run3},
				})
			})
		})

		Convey("Release", func() {
			mustRelease := func(ctx context.Context, runID common.RunID) {
				var innerErr error
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					innerErr = Release(ctx, notifier.notify, runID)
					return innerErr
				}, nil)
				So(innerErr, ShouldBeNil)
				So(err, ShouldBeNil)
			}
			So(datastore.Put(ctx, &queue{
				ID:       lProject,
				Current:  run1,
				Waitlist: common.RunIDs{run2, run3},
			}), ShouldBeNil)
			Convey("Current slot", func() {
				mustRelease(ctx, run1)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:       lProject,
					Waitlist: common.RunIDs{run2, run3},
				})
				So(notifier.notifyETAs(ctx, run2), ShouldResemble, []time.Time{ct.Clock.Now()})
				for _, r := range []common.RunID{run1, run3} {
					So(notifier.notifyETAs(ctx, r), ShouldBeEmpty)
				}
			})

			Convey("Run in waitlist", func() {
				mustRelease(ctx, run2)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run3},
				})
				for _, r := range []common.RunID{run1, run2, run3} {
					So(notifier.notifyETAs(ctx, r), ShouldBeEmpty)
				}
			})

			Convey("Non-existing run", func() {
				nonExisting := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("badbadbad"))
				mustRelease(ctx, nonExisting)
				So(mustLoadQueue(), cvtesting.SafeShouldResemble, &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2, run3},
				})
				for _, r := range []common.RunID{run1, run2, run3} {
					So(notifier.notifyETAs(ctx, r), ShouldBeEmpty)
				}
			})
		})

		submitOpts := &cfgpb.SubmitOptions{
			MaxBurst:   2,
			BurstDelay: durationpb.New(1 * time.Minute),
		}
		now := datastore.RoundTime(ct.Clock.Now()).UTC()
		Convey("With SubmitOpts", func() {
			Convey("ReleaseOnSuccess", func() {
				mustReleaseOnSuccess := func(ctx context.Context, runID common.RunID, submittedAt time.Time) {
					var innerErr error
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						innerErr = ReleaseOnSuccess(ctx, notifier.notify, runID, submittedAt)
						return innerErr
					}, nil)
					So(innerErr, ShouldBeNil)
					So(err, ShouldBeNil)
				}
				q := &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2},
					Opts:     submitOpts,
				}
				So(datastore.Put(ctx, q), ShouldBeNil)

				Convey("Records submitted timestamp", func() {
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					So(mustLoadQueue().History, ShouldResemble, []time.Time{now.Add(-1 * time.Second)})
				})

				Convey("Cleanup old history", func() {
					q.History = []time.Time{
						now.Add(-1 * time.Hour),   // removed
						now.Add(-1 * time.Minute), // kept
					}
					So(datastore.Put(ctx, q), ShouldBeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					So(mustLoadQueue().History, ShouldResemble, []time.Time{
						now.Add(-1 * time.Minute),
						now.Add(-1 * time.Second),
					})
				})

				Convey("Ensure order", func() {
					q.History = []time.Time{now.Add(-100 * time.Millisecond)}
					So(datastore.Put(ctx, q), ShouldBeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-200*time.Millisecond))
					So(mustLoadQueue().History, ShouldResemble, []time.Time{
						now.Add(-200 * time.Millisecond),
						now.Add(-100 * time.Millisecond),
					})
				})

				Convey("Notify the next Run immediately if below max burst", func() {
					q.History = []time.Time{now.Add(-2 * time.Minute)}
					So(datastore.Put(ctx, q), ShouldBeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					So(notifier.notifyETAs(ctx, run2), ShouldResemble, []time.Time{ct.Clock.Now()})
				})

				Convey("Notify the next Run with later ETA if quota has been exhausted", func() {
					q.History = []time.Time{
						now.Add(-45 * time.Second),
						now.Add(-30 * time.Second),
						// next submit should happen on (now - 30s) + 1min = now + 30s
					}
					So(datastore.Put(ctx, q), ShouldBeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					So(notifier.notifyETAs(ctx, run2), ShouldResemble, []time.Time{now.Add(30 * time.Second)})
				})
			})

			Convey("TryAcquire", func() {
				q := &queue{
					ID:   lProject,
					Opts: submitOpts,
				}
				So(datastore.Put(ctx, q), ShouldBeNil)

				Convey("Succeeds if below max burst", func() {
					q.History = []time.Time{
						now.Add(-2 * time.Minute),
						now.Add(-30 * time.Second),
					}
					So(datastore.Put(ctx, q), ShouldBeNil)
					So(mustTryAcquire(ctx, run1, submitOpts), ShouldBeFalse)
					So(notifier.notifyETAs(ctx, run1), ShouldBeNil)
				})

				Convey("Waitlist the Run and notify it with later ETA if quota has been exhausted", func() {
					q.History = []time.Time{
						now.Add(-2 * time.Minute),
						now.Add(-45 * time.Second),
						now.Add(-30 * time.Second),
						// next submit should happen on (now - 45s) + 1min = now + 15s
					}
					So(datastore.Put(ctx, q), ShouldBeNil)
					So(mustTryAcquire(ctx, run1, submitOpts), ShouldBeTrue)
					So(notifier.notifyETAs(ctx, run1), ShouldResemble, []time.Time{now.Add(15 * time.Second)})
				})
			})
		})
	})
}

type fakeNotifier struct {
	notifications map[common.RunID][]time.Time
}

func (f *fakeNotifier) notify(ctx context.Context, runID common.RunID, eta time.Time) error {
	if f.notifications == nil {
		f.notifications = make(map[common.RunID][]time.Time, 1)
	}
	if _, ok := f.notifications[runID]; !ok {
		f.notifications[runID] = make([]time.Time, 0, 1)
	}
	f.notifications[runID] = append(f.notifications[runID], eta)
	return nil
}

func (f *fakeNotifier) notifyETAs(ctx context.Context, runID common.RunID) []time.Time {
	etas, ok := f.notifications[runID]
	if !ok {
		return nil
	}
	ret := make([]time.Time, len(etas))
	copy(ret, etas)
	return ret
}
