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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
)

type queue struct {
	_kind string `gae:"$kind,SubmitQueue"`
	// ID is the ID of this submit queue.
	//
	// Currently use LUCI Project name as ID because SubmitQueue is expected to
	// be one per Project.
	ID string `gae:"$id"`
	// Current is the Run that is currently being submitted.
	Current common.RunID
	// Waitlist is a FIFO queue contains all Runs that are waiting to be
	// submitted.
	Waitlist common.RunIDs
	// Opts controls the rate of submission. Nil means no rate limiting.
	Opts *cfgpb.SubmitOptions
	// History records the timestamps of all submissions that happend within
	// `Opts.BurstDelay` if supplied.
	History []time.Time
}

// CreateQueue creates a submit queue for the provided LUCI Project.
//
// Overwrites any queue that this LUCI Project has since one LUCI Project
// can have only one submit queue.
func CreateQueue(ctx context.Context, luciProject string, opts *cfgpb.SubmitOptions) error {
	q := &queue{
		ID:   luciProject,
		Opts: opts,
	}
	if err := datastore.Put(ctx, q); err != nil {
		return errors.Annotate(err, "failed to create SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	return nil
}

// DeleteQueue deletes the submit queue of the provided LUCI Project.
func DeleteQueue(ctx context.Context, luciProject string) error {
	q := &queue{ID: luciProject}
	if err := datastore.Delete(ctx, q); err != nil {
		return errors.Annotate(err, "failed to delete SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	return nil
}

func loadQueue(ctx context.Context, luciProject string) (*queue, error) {
	q := &queue{ID: luciProject}
	switch err := datastore.Get(ctx, q); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Reason("SubmitQueue %q doesn't exist", q.ID).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to load SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	return q, nil
}

// ErrThrottled indicates the submit queue is currently being throttled.
var ErrThrottled = errors.New("submit queue is throttled, try later.")

// TryAcquire tries to acquire the current submission slot from the submit
// queue.
//
// The acquisition will succeed iff queue is empty or the requested Run is
// the first in the waitlist and no Run is currently occupying the current
// submission slot. Otherwise, this Run will be added to the waitlist and will
// be notified later after all Runs ahead of it have released their slots.
// Acquiring a slot for an existing Run in the submit queue won't change its
// position in the queue unless this Run is the first in the waitlist and
// the current slot is empty.
//
// The caller SHOULD submit the Run only after it successfully acquires the
// current submission slot.
//
// Returns ErrThrottled if the acquisition of the current slot will exceed the
// rate limiting defined in SubmitOptions of ProjectConfig.
//
// Transaction context is not necessary.
//
// TODO(yiwzhang): returns an eta together with ErrThrottled so that caller
// can know when to retry.
func TryAcquire(ctx context.Context, runID common.RunID) (waitlisted bool, err error) {
	if datastore.CurrentTransaction(ctx) != nil {
		return tryAcquire(ctx, runID)
	}
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		waitlisted, innerErr = tryAcquire(ctx, runID)
		return innerErr
	}, nil)
	switch {
	case innerErr != nil:
		return waitlisted, innerErr
	case err != nil:
		return waitlisted, errors.Annotate(err, "failed to acquire the slot for run %q", runID).Tag(transient.Tag).Err()
	default:
		return waitlisted, nil
	}
}

func tryAcquire(ctx context.Context, runID common.RunID) (waitlisted bool, err error) {
	q, err := loadQueue(ctx, runID.LUCIProject())
	if err != nil {
		return false, err
	}
	switch waitlistIdx := q.Waitlist.Index(runID); {
	case q.Current == runID:
		return false, nil
	case waitlistIdx > 0 || (q.Current != "" && waitlistIdx == 0):
		return true, nil
	case q.Current == "" && waitlistIdx == 0:
		// Requested run is at the front of the waitlist and current slot
		// is empty. Removes it from the waitlist and promote it to Current.
		q.Waitlist = q.Waitlist[1:]
		fallthrough
	case q.Current == "" && len(q.Waitlist) == 0:
		// TODO(yiwzhang): must obey with the submission rate limit.
		q.Current = runID
		waitlisted = false // not necessary, but for readability.
	default:
		q.Waitlist = append(q.Waitlist, runID)
		waitlisted = true
	}

	if err := datastore.Put(ctx, q); err != nil {
		return false, errors.Annotate(err, "failed to put SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	return waitlisted, nil
}

// NotifyFn is used to notify the Run that it's ready for submission.
//
// `eta` is the estimated time to notify the Run so that by that time, acquiring
// the current submission slot will obey with the submission rate limit. A zero
// `eta` means notify immediately.
type NotifyFn func(ctx context.Context, runID common.RunID, eta time.Time) error

// Release releases the slot occupied by the provided Run.
//
// If the provided Run occupies the current slot, give it up and notify the
// first Run in the waitlist using the `NotifyFn`.
// If the provided Run is in waitlist, remove it from the waitlist.
// If the provided Run is not present in the submit queue, no-op.
//
// Transaction context is not necessary.
func Release(ctx context.Context, runID common.RunID, notifyFn NotifyFn) error {
	if datastore.CurrentTransaction(ctx) != nil {
		return release(ctx, runID, notifyFn)
	}
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		innerErr = release(ctx, runID, notifyFn)
		return innerErr
	}, nil)
	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to release the slot for run %q", runID).Tag(transient.Tag).Err()
	default:
		return nil
	}
}

func release(ctx context.Context, runID common.RunID, notifyFn NotifyFn) error {
	q, err := loadQueue(ctx, runID.LUCIProject())
	if err != nil {
		return err
	}

	var notify common.RunID
	var notifyETA time.Time
	switch waitlistIdx := q.Waitlist.Index(runID); {
	case waitlistIdx >= 0:
		q.Waitlist = append(q.Waitlist[:waitlistIdx], q.Waitlist[waitlistIdx+1:]...)
	case q.Current == runID:
		q.Current = ""
		if len(q.Waitlist) > 0 {
			notify = q.Waitlist[0]
			// TODO(yiwzhang): compute notifyETA based on submission rate limit and
			// history.
		}
	default:
		return nil
	}
	if err := datastore.Put(ctx, q); err != nil {
		return errors.Annotate(err, "failed to put SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	if notify != "" && notifyFn != nil {
		if err := notifyFn(ctx, notify, notifyETA); err != nil {
			return err
		}
	}
	return nil
}
