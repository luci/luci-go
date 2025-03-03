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

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(queue{}))
}

func TestQueue(t *testing.T) {
	t.Parallel()

	ftt.Run("Queue", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		notifier := &fakeNotifier{}
		const lProject = "lProject"
		run1 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("deaddead"))
		run2 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("beefbeef"))
		run3 := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("cafecafe"))
		mustLoadQueue := func() *queue {
			q, err := loadQueue(ctx, lProject)
			assert.NoErr(t, err)
			return q
		}
		mustTryAcquire := func(ctx context.Context, runID common.RunID, opts *cfgpb.SubmitOptions) bool {
			var waitlisted bool
			var innerErr error
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				waitlisted, innerErr = TryAcquire(ctx, notifier.notify, runID, opts)
				return innerErr
			}, nil)
			assert.Loosely(t, innerErr, should.BeNil)
			assert.NoErr(t, err)
			return waitlisted
		}

		t.Run("TryAcquire", func(t *ftt.Test) {
			t.Run("When queue is empty", func(t *ftt.Test) {
				waitlisted := mustTryAcquire(ctx, run1, nil)
				assert.Loosely(t, waitlisted, should.BeFalse)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:      lProject,
					Current: run1,
				}))

				t.Run("And acquire same Run again", func(t *ftt.Test) {
					waitlisted := mustTryAcquire(ctx, run1, nil)
					assert.Loosely(t, waitlisted, should.BeFalse)
					assert.That(t, mustLoadQueue(), should.Match(&queue{
						ID:      lProject,
						Current: run1,
					}))
				})
			})

			t.Run("Waitlisted", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &queue{
					ID:      lProject,
					Current: run1,
					Opts:    nil,
				}), should.BeNil)
				waitlisted := mustTryAcquire(ctx, run2, nil)
				assert.Loosely(t, waitlisted, should.BeTrue)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2},
				}))
				waitlisted = mustTryAcquire(ctx, run3, nil)
				assert.Loosely(t, waitlisted, should.BeTrue)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2, run3},
				}))

				t.Run("And acquire same Run in waitlist again", func(t *ftt.Test) {
					for _, r := range []common.RunID{run2, run3} {
						waitlisted := mustTryAcquire(ctx, r, nil)
						assert.Loosely(t, waitlisted, should.BeTrue)
						assert.That(t, mustLoadQueue(), should.Match(&queue{
							ID:       lProject,
							Current:  run1,
							Waitlist: common.RunIDs{run2, run3},
						}))
					}
				})
			})

			t.Run("Promote the first in the Waitlist if Current is empty", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &queue{
					ID:       lProject,
					Waitlist: common.RunIDs{run2, run3},
					Opts:     nil,
				}), should.BeNil)
				waitlisted := mustTryAcquire(ctx, run2, nil)
				assert.Loosely(t, waitlisted, should.BeFalse)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:       lProject,
					Current:  run2,
					Waitlist: common.RunIDs{run3},
				}))
			})
		})

		t.Run("Release", func(t *ftt.Test) {
			mustRelease := func(ctx context.Context, runID common.RunID) {
				var innerErr error
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					innerErr = Release(ctx, notifier.notify, runID)
					return innerErr
				}, nil)
				assert.Loosely(t, innerErr, should.BeNil)
				assert.NoErr(t, err)
			}
			assert.Loosely(t, datastore.Put(ctx, &queue{
				ID:       lProject,
				Current:  run1,
				Waitlist: common.RunIDs{run2, run3},
			}), should.BeNil)
			t.Run("Current slot", func(t *ftt.Test) {
				mustRelease(ctx, run1)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:       lProject,
					Waitlist: common.RunIDs{run2, run3},
				}))
				assert.That(t, notifier.notifyETAs(ctx, run2), should.Match([]time.Time{ct.Clock.Now()}))
				for _, r := range []common.RunID{run1, run3} {
					assert.Loosely(t, notifier.notifyETAs(ctx, r), should.BeEmpty)
				}
			})

			t.Run("Run in waitlist", func(t *ftt.Test) {
				mustRelease(ctx, run2)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run3},
				}))
				for _, r := range []common.RunID{run1, run2, run3} {
					assert.Loosely(t, notifier.notifyETAs(ctx, r), should.BeEmpty)
				}
			})

			t.Run("Non-existing run", func(t *ftt.Test) {
				nonExisting := common.MakeRunID(lProject, clock.Now(ctx), 1, []byte("badbadbad"))
				mustRelease(ctx, nonExisting)
				assert.That(t, mustLoadQueue(), should.Match(&queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2, run3},
				}))
				for _, r := range []common.RunID{run1, run2, run3} {
					assert.Loosely(t, notifier.notifyETAs(ctx, r), should.BeEmpty)
				}
			})
		})

		submitOpts := &cfgpb.SubmitOptions{
			MaxBurst:   2,
			BurstDelay: durationpb.New(1 * time.Minute),
		}
		now := datastore.RoundTime(ct.Clock.Now()).UTC()
		t.Run("With SubmitOpts", func(t *ftt.Test) {
			t.Run("ReleaseOnSuccess", func(t *ftt.Test) {
				mustReleaseOnSuccess := func(ctx context.Context, runID common.RunID, submittedAt time.Time) {
					var innerErr error
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						innerErr = ReleaseOnSuccess(ctx, notifier.notify, runID, submittedAt)
						return innerErr
					}, nil)
					assert.Loosely(t, innerErr, should.BeNil)
					assert.NoErr(t, err)
				}
				q := &queue{
					ID:       lProject,
					Current:  run1,
					Waitlist: common.RunIDs{run2},
					Opts:     submitOpts,
				}
				assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)

				t.Run("Records submitted timestamp", func(t *ftt.Test) {
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					assert.That(t, mustLoadQueue().History, should.Match([]time.Time{now.Add(-1 * time.Second)}))
				})

				t.Run("Cleanup old history", func(t *ftt.Test) {
					q.History = []time.Time{
						now.Add(-1 * time.Hour),   // removed
						now.Add(-1 * time.Minute), // kept
					}
					assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					assert.That(t, mustLoadQueue().History, should.Match([]time.Time{
						now.Add(-1 * time.Minute),
						now.Add(-1 * time.Second),
					}))
				})

				t.Run("Ensure order", func(t *ftt.Test) {
					q.History = []time.Time{now.Add(-100 * time.Millisecond)}
					assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-200*time.Millisecond))
					assert.That(t, mustLoadQueue().History, should.Match([]time.Time{
						now.Add(-200 * time.Millisecond),
						now.Add(-100 * time.Millisecond),
					}))
				})

				t.Run("Notify the next Run immediately if below max burst", func(t *ftt.Test) {
					q.History = []time.Time{now.Add(-2 * time.Minute)}
					assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					assert.That(t, notifier.notifyETAs(ctx, run2), should.Match([]time.Time{ct.Clock.Now()}))
				})

				t.Run("Notify the next Run with later ETA if quota has been exhausted", func(t *ftt.Test) {
					q.History = []time.Time{
						now.Add(-45 * time.Second),
						now.Add(-30 * time.Second),
						// next submit should happen on (now - 30s) + 1min = now + 30s
					}
					assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)
					mustReleaseOnSuccess(ctx, run1, now.Add(-1*time.Second))
					assert.That(t, notifier.notifyETAs(ctx, run2), should.Match([]time.Time{now.Add(30 * time.Second)}))
				})
			})

			t.Run("TryAcquire", func(t *ftt.Test) {
				q := &queue{
					ID:   lProject,
					Opts: submitOpts,
				}
				assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)

				t.Run("Succeeds if below max burst", func(t *ftt.Test) {
					q.History = []time.Time{
						now.Add(-2 * time.Minute),
						now.Add(-30 * time.Second),
					}
					assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)
					assert.Loosely(t, mustTryAcquire(ctx, run1, submitOpts), should.BeFalse)
					assert.Loosely(t, notifier.notifyETAs(ctx, run1), should.BeNil)
				})

				t.Run("Waitlist the Run and notify it with later ETA if quota has been exhausted", func(t *ftt.Test) {
					q.History = []time.Time{
						now.Add(-2 * time.Minute),
						now.Add(-45 * time.Second),
						now.Add(-30 * time.Second),
						// next submit should happen on (now - 45s) + 1min = now + 15s
					}
					assert.Loosely(t, datastore.Put(ctx, q), should.BeNil)
					assert.Loosely(t, mustTryAcquire(ctx, run1, submitOpts), should.BeTrue)
					assert.That(t, notifier.notifyETAs(ctx, run1), should.Match([]time.Time{now.Add(15 * time.Second)}))
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
