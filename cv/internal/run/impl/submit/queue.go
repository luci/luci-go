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

	"google.golang.org/protobuf/proto"

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
	Current common.RunID `gae:",noindex"`
	// Waitlist is a FIFO queue containing all Runs that are waiting to be
	// submitted.
	Waitlist common.RunIDs `gae:",noindex"`
	// Opts controls the rate of submission. Nil means no rate limiting.
	Opts *cfgpb.SubmitOptions
	// History records the timestamps of all submissions that happened within
	// `Opts.BurstDelay` if supplied.
	History []time.Time `gae:",noindex"`
}

// nextSubmissionETA computes the eta of when next submission can happen based
// on SubmitOptions and submission history. A zero time means the Run can be
// submitted immediately.
func (q *queue) nextSubmissionETA() time.Time {
	// TODO: implement
	return time.Time{}
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

// NotifyFn is used to notify the run is ready for submission at `eta`
//
// In production, it is run.Notifier.NotifyReadyForSubmission(...)
type NotifyFn func(ctx context.Context, runID common.RunID, eta time.Time) error

// TryAcquire tries to acquire the current submission slot from the submit
// queue.
//
// Succeeds iff both
//  1. queue is empty or the requested Run is the first in the waitlist and no
//    Run is occupying the current submission slot at this moment
//  2. acquisition of the current slot satisfies the rate limit.
//
// Otherwise, this Run will be added to the waitlist and will be notified later
// after all Runs ahead of it have released their slots and the rate limit
// allows. Acquiring a slot for an existing Run in the submit queue won't
// change its position in the queue unless this Run is the first Run in the
// waitlist and the aforementioned conditions are met.
//
// The caller SHOULD submit the Run only after it successfully acquires the
// current submission slot. The caller is also RECOMMENDED to submit a task
// in the same transaction that runs later to check if the submission have
// failed unexpectedly without releasing the slot so that the submission slot
// is blocked on this Run.
//
// MUST be called in a datastore transaction.
func TryAcquire(ctx context.Context, notifyFn NotifyFn, runID common.RunID, opts *cfgpb.SubmitOptions) (waitlisted bool, err error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("TryAcquire must be called in a datastore transaction")
	}
	return tryAcquire(ctx, notifyFn, runID, opts)
}

func tryAcquire(ctx context.Context, notifyFn NotifyFn, runID common.RunID, opts *cfgpb.SubmitOptions) (waitlisted bool, err error) {
	var shouldSave bool
	q := &queue{ID: runID.LUCIProject()}
	switch err := datastore.Get(ctx, q); {
	case err == datastore.ErrNoSuchEntity:
		shouldSave = true
	case err != nil:
		return false, errors.Annotate(err, "failed to load SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}
	if !proto.Equal(q.Opts, opts) {
		q.Opts = opts
		shouldSave = true
	}

	switch waitlistIdx := q.Waitlist.Index(runID); {
	case q.Current == runID:
		waitlisted = false
	case waitlistIdx > 0 || (q.Current != "" && waitlistIdx == 0):
		waitlisted = true
	case q.Current == "" && len(q.Waitlist) == 0:
		// Queue is completely empty and waitlistIdx == -1.
		q.Waitlist = common.RunIDs{runID}
		shouldSave = true
		fallthrough
	case q.Current == "" && waitlistIdx == 0:
		// Requested Run is the first in the waitlist and the current slot is empty.
		// If the next submission can happen immediately, remove this Run from the
		// waitlist and promote it to Current. Otherwise, keep it in the waitlist.
		if eta := q.nextSubmissionETA(); eta.IsZero() {
			q.Current, q.Waitlist = q.Waitlist[0], q.Waitlist[1:]
			shouldSave = true
			waitlisted = false
		} else {
			if err := notifyFn(ctx, runID, eta); err != nil {
				return false, err
			}
			waitlisted = true
		}
	default:
		q.Waitlist = append(q.Waitlist, runID)
		shouldSave = true
		waitlisted = true
	}

	if shouldSave {
		if err := datastore.Put(ctx, q); err != nil {
			return false, errors.Annotate(err, "failed to put SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
		}
	}
	return waitlisted, nil
}

// Release releases the slot occupied by the provided Run.
//
// If the provided Run occupies the current slot, give it up and notify the
// first Run in the waitlist is ready for submission.
// If the provided Run is in waitlist, remove it from the waitlist.
// If the provided Run is not present in the submit queue, no-op.
//
// MUST be called in a datastore transaction.
func Release(ctx context.Context, notifyFn NotifyFn, runID common.RunID) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("Release must be called in a datastore transaction")
	}
	return release(ctx, notifyFn, runID)
}

func release(ctx context.Context, notifyFn NotifyFn, runID common.RunID) error {
	q, err := loadQueue(ctx, runID.LUCIProject())
	if err != nil {
		return err
	}

	switch waitlistIdx := q.Waitlist.Index(runID); {
	case waitlistIdx != -1:
		q.Waitlist = append(q.Waitlist[:waitlistIdx], q.Waitlist[waitlistIdx+1:]...)
	case q.Current == runID:
		q.Current = ""
	default:
		return nil
	}

	if err := datastore.Put(ctx, q); err != nil {
		return errors.Annotate(err, "failed to put SubmitQueue %q", q.ID).Tag(transient.Tag).Err()
	}

	if q.Current == "" && len(q.Waitlist) > 0 {
		if err := notifyFn(ctx, q.Waitlist[0], q.nextSubmissionETA()); err != nil {
			return err
		}
	}
	return nil
}

// CurrentRun returns the RunID that is currently submitting in the submit queue
// of the provided LUCI Project.
func CurrentRun(ctx context.Context, luciProject string) (common.RunID, error) {
	q, err := loadQueue(ctx, luciProject)
	if err != nil {
		return "", err
	}
	return q.Current, nil
}

// MustCurrentRun is like `CurrentRun(...)` but panic on error.
func MustCurrentRun(ctx context.Context, luciProject string) common.RunID {
	cur, err := CurrentRun(ctx, luciProject)
	if err != nil {
		panic(err)
	}
	return cur
}

// LoadCurrentAndWaitlist loads the current submission slot and the waitlist.
func LoadCurrentAndWaitlist(ctx context.Context, runID common.RunID) (current common.RunID, waitlist common.RunIDs, err error) {
	q, err := loadQueue(ctx, runID.LUCIProject())
	if err != nil {
		return "", nil, err
	}
	return q.Current, q.Waitlist, nil
}
