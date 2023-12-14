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
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

const (
	// RunCLKind is the Datastore entity kind for RunCL.
	RunCLKind = "RunCL"
	// RunLogKind is the Datastore entity kind for RunLog.
	RunLogKind = "RunLog"

	// MaxTryjobs limits maximum number of tryjobs that a single Run can track.
	//
	// Because all Run's tryjobs' results are kept in a Run.Tryjobs,
	// a single Datastore entity size limit of 1 MiB applies.
	//
	// 1024 is chosen because:
	//   * it is a magnitude above what was observed in practice before;
	//   * although stored tryjob results are user-influenced,
	//     they should be highly compressible, so even at 1KiB each,
	//     1024 tryjob results should amount to much less than 1 MiB.
	MaxTryjobs = 1024
)

// Run is an entity that contains high-level information about a CV Run.
//
// In production, Run entities are created only by the "runcreator" package.
//
// Detailed information about CLs and Tryjobs are stored in its child entities.
type Run struct {
	// $kind must match common.RunKind.
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
	// ModeDefinition is the definition for non-standard run mode.
	//
	// It is supplied in the project config. Note that ModeDefinition is fixed
	// after the run creation and wouldn't be changed even if the mode definition
	// has changed in the project config unless the mode name has changed. In that
	// case, the run would be cancelled and a new run with new mode name would be
	// created.
	ModeDefinition *cfgpb.Mode
	// Status describes the status of this Run.
	Status Status
	// EVersion is the entity version.
	//
	// It increments by one upon every successful modification.
	EVersion int64 `gae:",noindex"`
	// CreateTime is the timestamp when this Run was created.
	//
	// This is the timestamp of the last vote, on a Gerrit CL, that triggers this Run.
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
	// CreatedBy is the identity that creates this Run.
	//
	// For dry run and full run, it is the identity of the user who *first* makes
	// the Gerrit vote that triggers this run.  If the Run contains multiple CLs
	// (i.e. `combine_cls` is enabled), it is the identity of the user who
	// triggers the CL which has the latest triggering timestamp. For new
	// patchset run, it is the  the identity of the user who uploads the new
	// patchset.
	CreatedBy identity.Identity `gae:",noindex"`
	// ConfigGroupID is ID of the ConfigGroup that is used by this Run.
	//
	// RunManager may update the ConfigGroup in the middle of the Run if it is
	// notified that a new version of Config has been imported into CV.
	ConfigGroupID prjcfg.ConfigGroupID `gae:",noindex"`
	// CLs are IDs of all CLs involved in this Run.
	//
	// The index of Runs by CL is provided via RunCL's `IndexedID` field.
	CLs common.CLIDs `gae:",noindex"`
	// OriginCL is the CL in `CLs` that triggers this Run in the combined mode.
	//
	// For example, for a stack of 2 CLs, if the top CL gains a vote that triggers
	// a Dry Run for the stack, the top CL is the origin CL.
	OriginCL common.CLID `gae:",noindex"`
	// Options are Run-specific additions on top of LUCI project config.
	Options *Options
	// CancellationReasons are the reasons for cancelling a Run.
	//
	// Typically, only one reason will be available. But it's possible that Run
	// Manager received multiple cancellation requested at the approximately
	// the same time.
	//
	// Reasons should not contain duplication and empty reason.
	CancellationReasons []string `gae:",noindex"`
	// OngoingLongOps tracks long operations currently happening.
	OngoingLongOps *OngoingLongOps
	// Submission is the state of Run Submission.
	//
	// If set, Submission is in progress or has completed.
	Submission *Submission
	// Tryjobs is the state of the Run tryjobs.
	Tryjobs *Tryjobs
	// LatestCLsRefresh is the latest time when Run Manager scheduled async
	// refresh of CLs.
	LatestCLsRefresh time.Time `gae:",noindex"`
	// LatestTryjobsRefresh is the latest time when Run Manager scheduled async
	// refresh of Tryjobs.
	LatestTryjobsRefresh time.Time `gae:",noindex"`
	// DepRuns is a slice of Runs that this Run depends on.
	//
	// If this is set and mode == FullRun, this run
	// - will not start before all the deps start.
	// - will be canceled if any of the deps is canceled or failed.
	// - will be submitted only after all the deps are submitted.
	DepRuns common.RunIDs

	// QuotaExhaustionMsgLongOpRequested is set true when the current run
	// already has a pending msg posted.
	QuotaExhaustionMsgLongOpRequested bool `gae:",noindex"`
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

// RunLog is an immutable entry for meaningful changes to a Run's state.
//
// RunLog entity is written
type RunLog struct {
	_kind string `gae:"$kind,RunLog"`

	// ID is the value Run.EVersion which was saved transactionally with the
	// creation of the RunLog entity.
	//
	// Thus, ordering by ID (default Datastore ordering) will automatically
	// provide semantically chronological order.
	ID  int64          `gae:"$id"`
	Run *datastore.Key `gae:"$parent"`
	// Entries record what happened to the Run.
	//
	// There is always at least one.
	// Ordered from logically oldest to newest.
	//
	// Entries are stored as a protobuf in order to expose them using Admin API
	// with ease.
	// This is however opaque to Datastore, and so it has a downside of inability
	// to index or filter by kinds of LogEntry within the Datastore itself.
	// However, total number of LogEntries per Run should be <<1000 and the
	// intended consumption are humans, therefore indexing isn't a deal breaker.
	Entries *LogEntries
}
