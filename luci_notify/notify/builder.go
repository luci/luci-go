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

package notify

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/logging"
)

// Builder represents the state of the last build seen from a particular
// builder in order to implement certain notification triggers (i.e. on change).
type Builder struct {
	// ID is the builder's canonical ID (e.g. buildbucket/bucket/name).
	ID string `gae:"$id"`

	// Status is current status of the builder.
	// It is updated every time a new build completes.
	Status buildbucket.Status

	// StatusRevision can be used to decide whether Status should be updated.
	// It is the revision of the codebase that's associated with the build
	// that caused a change of Status.
	StatusRevision string

	// StatusTime can be used to decide whether Status should be updated.
	// It is computed as the commit time of the revision associated
	// with the build that caused a change of Status.
	StatusTime time.Time
}

// StatusUnknown is used in the LookupBuilder return value
// if builder status is unknown.
var StatusUnknown buildbucket.Status = -1

// NewBuilder creates a new builder from an ID, a build, and optionally a commit.
func NewBuilder(id string, status buildbucket.Status, commit *gitiles.Commit) *Builder {
	return &Builder{
		ID:               id,
		Status:           status,
		StatusRevision:   commit.Commit,
		StatusTime: time.Time(commit.Committer.Time),
	}
}

// LookupBuilder returns a "previous" builder state.
//
// If no "previous" build is found in the datastore, then returns a Builder
// with Status==StatusUnknown in order to make explicit that we
// have never recorded information about this builder before.
//
// It will also update the Builder in the datastore according to build.
func LookupBuilder(c context.Context, id string, build *buildbucket.Build, commit *gitiles.Commit) (*Builder, error) {
	prev := &Builder{ID: id}
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, prev); {
		case err == datastore.ErrNoSuchEntity:
			prev.Status = StatusUnknown
			prev.StatusTime = time.Time{}
			logging.Debugf(c, "found no builder %q", prev.ID)

		case err != nil:
			return err

		// If there's been no status change, don't bother updating.
		case build.Status == prev.Status:
			return nil

		// If it's a status change and from a different revision, but
		// the commit is older than what we've seen, don't bother.
		case prev.StatusTime.After(time.Time(commit.Committer.Time)):
			logging.Debugf(c,
				"build %d (%s) from commit at %s, is not new",
				build.ID, build.Status, commit.Committer.Time)
			return nil
		}

		// Note that we update even if the commit revision is the same. This
		// is because we want to compute on_change against the latest status
		// for this builder when we do actually see a revision change.
		return datastore.Put(c, NewBuilder(id, build.Status, commit))
	}, nil)
	if err != nil {
		return nil, err
	}
	return prev, nil
}
