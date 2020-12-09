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
	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	run "go.chromium.org/luci/cv/internal/runmanager"
)

// RunOwner keeps tracks of all open (active or pending) Runs for a user.
type RunOwner struct {
	_kind string `gae:"$kind,RunOwner"`

	// ID is the user identity.
	ID identity.Identity `gae:"$id"`
	// ActiveRuns are all Runs triggered by this user that are active.
	ActiveRuns []run.ID `gae:",noindex"`
	// PendingRuns are all Runs triggered by this user that are
	// yet-to-be-launched (i.e. quota doesn't permit).
	PendingRuns []run.ID `gae:",noindex"`
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
	ID     changelist.CLID `gae:"$id"`
	Run    *datastore.Key  `gae:"$parent"`
	Detail *changelist.Snapshot
}
