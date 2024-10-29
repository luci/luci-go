// Copyright 2017 The LUCI Authors.
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

package config

import (
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

// Builder represents the state of the last build seen from a particular
// builder in order to implement certain notification triggers (i.e. on change).
type Builder struct {
	// ProjectKey is a datastore key to this Builder's project. Note that this key
	// is a parent key, effectively making the Builder a child of a specific project.
	ProjectKey *datastore.Key `gae:"$parent"`

	// ID is the builder's canonical ID (e.g. <bucket>/<name>).
	ID string `gae:"$id"`

	// Repository is the repository this builder is tracking and the repository that
	// Revision is valid for.
	Repository string

	// Notifications is Notifications proto message, containing Notification messages
	// associated with this builder. Each notification contains information about who
	// to notify, and different settings on how to notify them.
	Notifications *notifypb.Notifications

	// Status is current status of the builder.
	// It is updated every time a new build has a new status and either
	//   1) the new build has a newer revision than StatusRevision, or
	//   2) the new build's revision == StatusRevision, but it has a newer
	//      creation time.
	Status buildbucketpb.Status

	// BuildTime is computed as the creation time of the most recent build encountered.
	// It can be used to decide whether Status and this Builder should be updated.
	BuildTime time.Time

	// Revision is the revision of the codebase that's associated with the most
	// recent build encountered. It can be used to decide whether Status should be
	// updated.
	Revision string

	// GitilesCommits are the gitiles commits checked out by the most recent build
	// encountered that had a non-empty checkout. It can also be used to compute a
	// blamelist.
	GitilesCommits *notifypb.GitilesCommits `gae:",legacy"`

	// Extra and unrecognized fields will be loaded without issue but not saved.
	_ datastore.PropertyMap `gae:"-,extra"`
}
