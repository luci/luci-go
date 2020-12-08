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

package impl

import (
	"time"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// CV will be dead on 2336-10-19T17:46:40Z (10^10s after 2020-01-01T00:00:00Z).
var endOfTheWorld = time.Date(2336, 10, 19, 17, 46, 40, 0, time.UTC)

// RunID is an unique RunID to identify a Run in CV.
//
// RunID is a '/' separated string with following three parts:
//  1. The LUCI Project that this Run belongs to.
//  2. `endOfTheWorld` - triggering timestamp in ms precision. It will be left
//     padded with zero to 13-digit long. For non-API triggered Run, the
//     triggering timestamp is the timestamp of the vote that triggers this CV
//     Run.
//  3. A hex digest string that uniquely identifying the set of CLs that this
//     Run involves (a.k.a cl_group_hash).
type RunID string

// Mode dictates the behavior of this Run.
type Mode string

const (
	// DryRun triggers all defined Tryjobs but doesn't submit.
	DryRun Mode = "DryRun"
	// FullRun is DryRun + submit.
	FullRun Mode = "FullRun"
)

// Run contains high-level information of a CV Run.
//
// Detail Information is stored its child entities.
type Run struct {
	_kind  string                `gae:"$kind,Run"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is the RunID generated at triggering time.
	//
	// See doc for type `RunID` about the format
	ID RunID `gae:"$id"`
	// Mode dictates the behavior of this Run.
	Mode Mode `gae:",noindex"`
	// Status describes the status of this Run.
	Status Status `gae:",noindex"`
	// EVersion is the entity version.
	//
	// It increments by one upon every successful modification.
	EVersion int `gae:",noindex"`
	// CreateTime is the timestamp when this Run was created.
	CreateTime time.Time `gae:",noindex"`
	// StartTime is the timestamp when this Run was started.
	StartTime time.Time `gae:",noindex"`
	// UpdateTime is the timestamp when this entity was last updated.
	UpdateTime time.Time `gae:",noindex"`
	// EndTime is the timestamp when this Run completed.
	EndTime time.Time `gae:",noindex"`
	// Owner is the owner of this Run.
	//
	// Currently, it is the same as owner of the CL.
	Owner identity.Identity `gae:",noindex"`
	// ACLAllowList is a list of chrome-infra-auth groups whose member are
	// authorized to trigger this Run.
	//
	// `Owner` MUST be a member for at least one of these group. CV will check
	// periodically and wil stop the Run if Owner lose its access.
	ACLAllowList []string `gae:",noindex"`
	// ConfigGroupID is ID of the ConfigGroup that is used by this Run.
	//
	// RunManager may update the ConfigGroup in the middle of the Run if it is
	// notified a new version of Config has been imported into CV.
	ConfigGroupID config.ConfigGroupID `gae:",noindex"`
	// TryjobQuota is remaining quota availabe to retry failed Tryjobs.
	TryjobQuota int `gae:",noindex"`

	// TODO(yiwzhang): Define GerritAction (including posting comments and
	// removing CQ labels)
}

// RunOwner keeps tracks of all open (active or pending) Runs for a user.
type RunOwner struct {
	_kind string `gae:"$kind,RunOwner"`

	// ID is the user identity.
	ID identity.Identity `gae:"$id"`
	// ActiveRuns are all Runs launched by this user that are active.
	ActiveRuns []RunID `gae:",noindex"`
	// PendingRuns are all Runs launched by this user that are still pending
	// (i.e. quota doesn't premit).
	PendingRuns []RunID `gae:",noindex"`
}

// RunCL is a snapshot of the CL when this Run was triggered.
type RunCL struct {
	_kind string `gae:"$kind,RunCL"`

	// ID is the CL internal ID.
	ID     changelist.CLID     `gae:"$id"`
	RunID  *datastore.Key      `gae:"$parent"`
	Detail changelist.Snapshot `gae:",noindex"`
}

// RunTryjob contains Run-specific Tryjob information.
//
// All detailed information is stored in `Tryjob` entity.
type RunTryjob struct {
	_kind string `gae:"$kind,RunTryjob"`

	// ID is the Tryjob internal ID.
	ID    tryjob.ID      `gae:"$id"`
	RunID *datastore.Key `gae:"$parent"`
	// Status is the Tryjob Status
	Status tryjob.Status `gae:",noindex"`
	// UpdateTime is the timestamp when this entity was last updated.
	UpdateTime time.Time `gae:",noindex"`
	// Builder is the builder definition of this Tryjob.
	Builder configpb.Verifiers_Tryjob_Builder
	// Experimental tells whether this Tryjob is experimental.
	//
	// See doc of `experiment_percentage` in config proto for what experimental
	// means within CV context.
	Experimental bool `gae:",noindex"`
	// Quota is the remaining quota availabe to retry failed Tryjobs.
	//
	// It differs from the `TryjobQuota` property in `Run` entity as it only
	// applies to this Tryjob while the latter one applies to all Tryjobs
	// launched for this Run.
	Quota int `gae:",noindex"`
	// RetryConfig is the retry configuration for this Tryjob.
	//
	// It is used to determine the weight of different type of tryjob failure.
	RetryConfig configpb.Verifiers_Tryjob_RetryConfig
	// WaitingOn are the IDs of all Tryjobs that this Tryjob is waiting on.
	//
	// In other word, this Tryjob won't be triggered unless all Tryjobs in this
	// slice report success status.
	WaitingOn []tryjob.ID
	// PrevRetries are the IDs of all retries of this Tryjobs.
	PrevRetries []tryjob.ID
}
