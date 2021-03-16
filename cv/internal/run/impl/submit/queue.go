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
	"go.chromium.org/luci/cv/internal/run"
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
	// Waitlist is a FIFO queue containing all Runs that are waiting to be
	// submitted.
	Waitlist common.RunIDs
	// Opts controls the rate of submission. Nil means no rate limiting.
	Opts *cfgpb.SubmitOptions
	// History records the timestamps of all submissions that happend within
	// `Opts.BurstDelay` if supplied.
	History []time.Time
}

// nextSubmissionETA computes the eta of when next submission can happen based
// on SubmitOptions and submission history. A zero time means the Run can be
// submitted immediately.
func (q *queue) nextSubmissionETA() time.Time {
	// TODO: implement
	return time.Time{}
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
// If the Run fails to acquire the current submission slot because it is
// geting throttled according to `SubmitOptions` config, this Run will always
// be placed at the front of the waitlist and an `eta` will be returned to
// instruct the caller when to retry.
//
// Re-uses transaction if already in context, otherwise runs independent
// transaction.
func TryAcquire(ctx context.Context, runID common.RunID) (waitlisted bool, eta time.Time, err error) {
	if datastore.CurrentTransaction(ctx) != nil {
		return tryAcquire(ctx, runID)
	}
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		waitlisted, eta, innerErr = tryAcquire(ctx, runID)
		return innerErr
	}, nil)
	switch {
	case innerErr != nil:
		return false, time.Time{}, innerErr
	case err != nil:
		return false, time.Time{}, errors.Annotate(err, "failed to acquire the slot for Run %q", runID).Tag(transient.Tag).Err()
	default:
		return waitlisted, eta, nil
	}
}

func tryAcquire(ctx context.Context, runID common.RunID) (waitlisted bool, eta time.Time, err error) {
	q, err := loadQueue(ctx, runID.LUCIProject())
	if err != nil {
		return false, time.Time{}, err
	}
	switch waitlistIdx := q.Waitlist.Index(runID); {
	case q.Current == runID:
		return false, time.Time{}, nil
	case waitlistIdx > 0 || (q.Current != "" && waitlistIdx == 0):
		return true, time.Time{}, nil
	case q.Current == "" && waitlistIdx == 0:
		// Requested Run is the first in the waitlist and the current slot is empty.
		// If can submit now, pop this Run from the waitlist and promote it to
		// Current. Otherwise, keep it in the waitlist.
		if eta = q.nextSubmissionETA(); !eta.IsZero() {
			return true, eta, nil
		}
		q.Waitlist = q.Waitlist[1:]
		q.Current = runID
		waitlisted = false
	case q.Current == "" && len(q.Waitlist) == 0:
		// Queue is completely empty. If can submit now, add the Run to Current.
		// Otherwise, add it to the waitlist.
		if eta = q.nextSubmissionETA(); !eta.IsZero() {
			q.Waitlist = common.RunIDs{runID}
			waitlisted = true
		} else {
			q.Current = runID
			waitlisted = false
		}
	default:
		q.Waitlist = append(q.Waitlist, runID)
		waitlisted = true
	}

	if err := datastore.Put(ctx, q); err != nil {
		return false, time.Time{}, errors.Annotate(err, "failed to put SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	return waitlisted, eta, nil
}

// Release releases the slot occupied by the provided Run.
//
// If the provided Run occupies the current slot, give it up and notify the
// first Run in the waitlist is ready for submission.
// If the provided Run is in waitlist, remove it from the waitlist.
// If the provided Run is not present in the submit queue, no-op.
//
// Re-uses transaction if already in context, otherwise runs independent
// transaction.
func Release(ctx context.Context, runID common.RunID) error {
	if datastore.CurrentTransaction(ctx) != nil {
		return release(ctx, runID)
	}
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		innerErr = release(ctx, runID)
		return innerErr
	}, nil)
	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to release the slot for Run %q", runID).Tag(transient.Tag).Err()
	default:
		return nil
	}
}

func release(ctx context.Context, runID common.RunID) error {
	q, err := loadQueue(ctx, runID.LUCIProject())
	if err != nil {
		return err
	}

	var notify common.RunID
	switch waitlistIdx := q.Waitlist.Index(runID); {
	case waitlistIdx >= 0:
		q.Waitlist = append(q.Waitlist[:waitlistIdx], q.Waitlist[waitlistIdx+1:]...)
	case q.Current == runID:
		q.Current = ""
		if len(q.Waitlist) > 0 {
			notify = q.Waitlist[0]
		}
	default:
		return nil
	}
	if err := datastore.Put(ctx, q); err != nil {
		return errors.Annotate(err, "failed to put SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	if notify != "" {
		if err := run.NotifyReadyForSubmission(ctx, notify, q.nextSubmissionETA()); err != nil {
			return err
		}
	}
	return nil
}
