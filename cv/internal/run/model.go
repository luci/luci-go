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

package run

import (
	"context"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
)

// RunKind is the Datastore entity kind for Run.
const RunKind = "Run"

// Mode dictates the behavior of this Run.
type Mode string

const (
	// DryRun triggers all defined Tryjobs, but doesn't submit.
	DryRun Mode = "DryRun"
	// FullRun is DryRun followed by submit.
	FullRun Mode = "FullRun"
)

// Run is an entity that contains high-level information about a CV Run.
//
// Detailed information about CLs and Tryjobs are stored in its child entities.
type Run struct {
	// $kind must match RunKind.
	_kind  string                `gae:"$kind,Run"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is the RunID generated at triggering time.
	//
	// See doc for type `common.RunID` about the format.
	ID common.RunID `gae:"$id"`
	// CreationOperationID is a string used to de-dup Run creation attempts.
	CreationOperationID string `gae:",noindex"`
	// Mode dictates the behavior of this Run.
	Mode Mode `gae:",noindex"`
	// Status describes the status of this Run.
	Status Status
	// EVersion is the entity version.
	//
	// It increments by one upon every successful modification.
	EVersion int `gae:",noindex"`
	// CreateTime is the timestamp when this Run was created.
	//
	// For API triggered Run, the CreateTime is when CV processes the request.
	// For non-API triggered Run, the CreateTime is the timestamp of the last
	// vote on a Gerrit CL that triggers this Run.
	CreateTime time.Time `gae:",noindex"`
	// StartTime is the timestamp when this Run was started.
	StartTime time.Time `gae:",noindex"`
	// UpdateTime is the timestamp when this entity was last updated.
	UpdateTime time.Time `gae:",noindex"`
	// EndTime is the timestamp when this Run has completed.
	EndTime time.Time `gae:",noindex"`
	// Owner is the identity of the owner of this Run.
	//
	// Currently, it is the same as owner of the CL. If `combine_cls` is
	// enabled for the ConfigGroup used by this Run, the owner is the CL which
	// has latest triggering timestamp.
	Owner identity.Identity `gae:",noindex"`
	// ConfigGroupID is ID of the ConfigGroup that is used by this Run.
	//
	// RunManager may update the ConfigGroup in the middle of the Run if it is
	// notified that a new version of Config has been imported into CV.
	ConfigGroupID config.ConfigGroupID `gae:",noindex"`
	// CLs are IDs of all CLs invovled in this Run.
	CLs []common.CLID `gae:",noindex"`
	// TODO(yiwzhang): Define
	//  * GerritAction (including posting comments and removing CQ labels).
	//  * RemainingTryjobQuota: Run-level Tryjob quota.
}

// Mutate mutates the Run by executing `mut`.
//
// It ensures EVersion and UpdateTime are correctly updated if `mut` has
// changed the Run.
func (r *Run) Mutate(ctx context.Context, mut func(*Run) (updated bool)) (updated bool) {
	prevEV := r.EVersion
	updated = mut(r)
	if !updated {
		return false
	}
	r.EVersion = prevEV + 1
	r.UpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
	return true
}

// RunOwner keeps tracks of all open (active or pending) Runs for a user.
type RunOwner struct {
	_kind string `gae:"$kind,RunOwner"`

	// ID is the user identity.
	ID identity.Identity `gae:"$id"`
	// ActiveRuns are all Runs triggered by this user that are active.
	ActiveRuns common.RunIDs `gae:",noindex"`
	// PendingRuns are all Runs triggered by this user that are
	// yet-to-be-launched (i.e. quota doesn't permit).
	PendingRuns common.RunIDs `gae:",noindex"`
}

// RunCL is the snapshot of a CL involved in this Run.
//
// TODO(yiwzhang): Figure out if RunCL needs to be updated in the middle
// of the Run, because CV might need this for removing votes (new votes
// may come in between) and for avoiding posting duplicated comments.
// Alternatively, CV could always re-query Gerrit right before those
// operations so that there's no need for updating the snapshot.
type RunCL struct {
	_kind string `gae:"$kind,RunCL"`

	// ID is the CL internal ID.
	ID      common.CLID    `gae:"$id"`
	Run     *datastore.Key `gae:"$parent"`
	Detail  *changelist.Snapshot
	Trigger *Trigger
}

// LoadMulti is datastore.Get(ctx, runs) which parallelizes & batches, staying
// within Datatstore limits even for large number of Runs.
//
// TODO(tandrii): generalize it via reflection or once Go finally gets Generics.
// The same code also exists in changelist package.
func LoadMulti(ctx context.Context, runs []*Run) (err error) {
	ctx, span := trace.StartSpan(ctx, "go.chromium.org/luci/cv/internal/run/LoadMulti")
	defer func() { span.End(err) }()

	// maxBatchSize limits how many Runs can be loaded at once.
	//
	// Datastore hard limit is 1000 [1], but actual Lookup API [2]
	// may have to be called multiple times until all results are fetched [3].
	// So, be conservative here.
	// [1] https://cloud.google.com/datastore/docs/concepts/limits
	// [2] https://godoc.org/cloud.google.com/go/datastore#Client.GetMulti
	// [3] https://github.com/googleapis/google-cloud-go/blob/e38884163dd4acca7c10dcf5f21831c69af5bc18/datastore/datastore.go#L430
	const maxBatchSize = 512

	concurrency := 1 + len(runs)/maxBatchSize
	if concurrency > 16 {
		concurrency = 16
	}

	lazyErrors := errors.NewLazyMultiError(len(runs))
	perr := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for i := 0; i < len(runs); i += maxBatchSize {
			i := i
			batch := runs[i:]
			if len(batch) > maxBatchSize {
				batch = batch[:maxBatchSize]
			}
			work <- func() error {
				err := datastore.Get(ctx, batch)
				if err == nil {
					return nil
				}
				if merr, ok := err.(errors.MultiError); ok {
					for j, err := range merr {
						if err != nil {
							lazyErrors.Assign(i+j, err)
						}
					}
					return nil
				}
				return err // singular error only.
			}
		}
	})
	switch {
	case perr != nil:
		err = common.MostSevereError(err)
	default:
		err = lazyErrors.Get()
	}
	return
}
