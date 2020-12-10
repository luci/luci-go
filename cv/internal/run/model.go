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

	"go.chromium.org/luci/cv/internal/config"
)

// CV will be dead on 2336-10-19T17:46:40Z (10^10s after 2020-01-01T00:00:00Z).
var endOfTheWorld = time.Date(2336, 10, 19, 17, 46, 40, 0, time.UTC)

// ID is an unique ID to identify a Run in CV.
//
// ID is a '/' separated string with following three parts:
//  1. The LUCI Project that this Run belongs to.
//  2. (`endOfTheWorld` - CreateTime) in ms precision, left-padded with zeros
//     to 13 digits. See `Run.CreateTime` Doc.
//  3. A hex digest string that uniquely identifying the set of CLs involved in
//     this Run (a.k.a cl_group_hash).
type ID string

// Mode dictates the behavior of this Run.
type Mode string

const (
	// DryRun triggers all defined Tryjobs but doesn't submit.
	DryRun Mode = "DryRun"
	// FullRun is DryRun + submit.
	FullRun Mode = "FullRun"
)

// Run is an entity contains high-level information of a CV Run.
//
// Detail information about CL and Tryjobs are stored in its child entities.
type Run struct {
	_kind  string                `gae:"$kind,Run"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is the RunID generated at triggering time.
	//
	// See doc for type `RunID` about the format
	ID ID `gae:"$id"`
	// Mode dictates the behavior of this Run.
	Mode Mode `gae:",noindex"`
	// Status describes the status of this Run.
	Status Status `gae:",noindex"`
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
	// Currently, it is the same as owner of the CL. If `combine_cls` is enabled
	// for the ConfigGroup used by this Run, the owner is the CL which has latest
	// triggering timestamp.
	Owner identity.Identity `gae:",noindex"`
	// ConfigGroupID is ID of the ConfigGroup that is used by this Run.
	//
	// RunManager may update the ConfigGroup in the middle of the Run if it is
	// notified that a new version of Config has been imported into CV.
	ConfigGroupID config.ConfigGroupID `gae:",noindex"`
	// TODO(yiwzhang): Define
	//  * GerritAction (including posting comments and removing CQ labels).
	//  * RemainingTryjobQuota: Run-level Tryjob quota.
}
