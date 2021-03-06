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
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// RunKind is the Datastore entity kind for Run.
const RunKind = "Run"

// RunKind is the Datastore entity kind for RunCL.
const RunCLKind = "RunCL"

// Run is an entity that contains high-level information about a CV Run.
//
// In production, Run entities are created only by the "runcreator" package.
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
	// has the latest triggering timestamp.
	Owner identity.Identity `gae:",noindex"`
	// ConfigGroupID is ID of the ConfigGroup that is used by this Run.
	//
	// RunManager may update the ConfigGroup in the middle of the Run if it is
	// notified that a new version of Config has been imported into CV.
	ConfigGroupID prjcfg.ConfigGroupID `gae:",noindex"`
	// CLs are IDs of all CLs involved in this Run.
	CLs common.CLIDs `gae:",noindex"`
	// Options are Run-specific additions on top of LUCI project config.
	Options *Options
	// Submission is the current state of Run Submission.
	//
	// If set, Submission is in progress or has completed.
	Submission *Submission

	// LatestCLsRefresh is the latest time when Run Manager scheduled async
	// refresh of CLs.
	LatestCLsRefresh time.Time `gae:",noindex"`

	// TODO(yiwzhang): Define
	//  * RemainingTryjobQuota: Run-level Tryjob quota.

	// CQAttemptKey is what CQDaemon exports to BigQuery as Attempt's key.
	//
	// In CQDaemon's source, it's equivalent to GerritAttempt.attempt_key_hash.
	CQDAttemptKey string
	// FinalizedByCQD is true iff the Run was finalized by CQDaemon, which
	// includes submitting CLs and/or removing CQ votes and sending BQ row.
	FinalizedByCQD bool `gae:",noindex"`
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

// RunCL is an immutable snapshot of a CL at the time of the Run start.
type RunCL struct {
	_kind string `gae:"$kind,RunCL"`

	ID         common.CLID           `gae:"$id"`
	Run        *datastore.Key        `gae:"$parent"`
	ExternalID changelist.ExternalID `gae:",noindex"`
	Detail     *changelist.Snapshot
	Trigger    *Trigger

	// IndexedID is a copy of ID to get an index on just the CLID,
	// as the primary automatic index is on (Run(parent), ID).
	IndexedID common.CLID

	// TODO(tandrii): add field of (ExternalID + "/" + patchset) to support for
	// direct searches for gerrit/host/change/patchset.
}
