// Copyright 2025 The LUCI Authors.
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

package scan

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

// SliceExpirationEnforcer is TaskVisitor that updates tasks whose current
// task slice (represented by a TaskToRun entity) has expired.
//
// Normally a TaskToRun entity is associated with an RBE reservation and RBE
// itself keeps track of the expiration, sending us PubSub notifications when
// they happen (see expireSliceBasedOnReservation in ReservationServer).
//
// But sometimes these notifications either get significantly delayed or, maybe,
// do not arrive at all (hard to tell). This is when this scanner kicks in.
type SliceExpirationEnforcer struct {
	// GracePeriod is how long to wait past actual expiration before doing
	// anything.
	//
	// This exists to make sure we do not race with the RBE's PubSub-based slice
	// expiration mechanism, to avoid unnecessary datastore contention.
	GracePeriod time.Duration

	// TasksManager is used to change state of tasks.
	TasksManager tasks.Manager

	// Config is a snapshot of the server configuration.
	Config *cfg.Config

	// The goroutine group executing the transactions.
	eg *errgroup.Group
	// Number of slices successfully expired.
	expiredCount atomic.Int64
	// Number of slices we attempted to expire, but failed.
	failedCount atomic.Int64
	// Number of slices we attempted to expire, but ended up skipping.
	skippedCount atomic.Int64
}

var _ TaskVisitor = (*SliceExpirationEnforcer)(nil)

// Prepare implements TaskVisitor.
func (s *SliceExpirationEnforcer) Prepare(ctx context.Context) {
	s.eg, _ = errgroup.WithContext(ctx)
	s.eg.SetLimit(512)
}

// Visit implements TaskVisitor.
func (s *SliceExpirationEnforcer) Visit(ctx context.Context, trs *model.TaskResultSummary) {
	if s.isCurrentSliceExpired(ctx, trs) {
		s.eg.Go(func() error {
			tid := model.RequestKeyToTaskID(trs.TaskRequestKey(), model.AsRequest)
			switch expired, err := s.expireSlice(ctx, trs.TaskRequestKey(), trs.CurrentTaskSlice); {
			case err != nil:
				logging.Errorf(ctx, "Expiring slice %s/%d: %s", tid, trs.CurrentTaskSlice, err)
				s.failedCount.Add(1)
			case expired:
				logging.Infof(ctx, "Expiring slice %s/%d: success", tid, trs.CurrentTaskSlice)
				s.expiredCount.Add(1)
			default:
				logging.Infof(ctx, "Expiring slice %s/%d: skipped", tid, trs.CurrentTaskSlice)
				s.skippedCount.Add(1)
			}
			return nil
		})
	}
}

// Finalize implements TaskVisitor.
func (s *SliceExpirationEnforcer) Finalize(ctx context.Context, _ error) error {
	_ = s.eg.Wait()

	logging.Infof(ctx, "Expired %d task slices", s.expiredCount.Load())
	if skipped := s.skippedCount.Load(); skipped != 0 {
		logging.Infof(ctx, "Skipped expiring %d task slices (because this was not necessary anymore)", skipped)
	}
	if failed := s.failedCount.Load(); failed != 0 {
		logging.Errorf(ctx, "Failed to expire %d task slices", failed)
		return errors.Reason("failed to expire some task slices").Err()
	}

	return nil
}

// isCurrentSliceExpired checks if the current slice has expired.
func (s *SliceExpirationEnforcer) isCurrentSliceExpired(ctx context.Context, trs *model.TaskResultSummary) bool {
	if trs.State != apipb.TaskState_PENDING || !trs.SliceExpiration.IsSet() {
		return false
	}
	return clock.Now(ctx).After(trs.SliceExpiration.Get().Add(s.GracePeriod))
}

// expireSlice transactionally expires the given slice if necessary.
func (s *SliceExpirationEnforcer) expireSlice(ctx context.Context, reqKey *datastore.Key, sliceIndex int64) (expired bool, err error) {
	tr, err := model.FetchTaskRequest(ctx, reqKey)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return false, nil // it is gone already, nothing to expire
	case err != nil:
		return false, errors.Annotate(err, "failed to fetch TaskRequest").Err()
	}

	toRunKey, err := model.TaskRequestToToRunKey(ctx, tr, int(sliceIndex))
	if err != nil {
		return false, err
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		outcome, err := s.TasksManager.ExpireSliceTxn(ctx, &tasks.ExpireSliceOp{
			Request:  tr,
			ToRunKey: toRunKey,
			Reason:   tasks.ExpiredViaScan,
			Config:   s.Config,
		})
		if err != nil {
			return err
		}
		expired = outcome.Expired
		return nil
	}, nil)
	return
}
