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

	// StatusTime can be used to decide whether Status should be updated.
	// It is computed as either the creation time of the build that caused
	// a change of Status, or the commit time of the revision associated
	// with the that build. The former definition is used if
	// StatusRevision==nil, meaning this builder has no one repository
	// associated with it.
	StatusTime time.Time

	// StatusRevision can be used to decided whether a Status should be updated.
	// It is the revision of the codebase that's associated with the build
	// that last caused a change of Status.
	StatusRevision string
}

// StatusUnknown is used in the LookupBuilder return value
// if builder status is unknown.
var StatusUnknown buildbucket.Status = -1

// NewBuilder creates a new builder from an ID, a build, and optionally a commit.
func NewBuilder(id string, build *buildbucket.Build, commit *gitiles.Commit) *Builder {
	builder := &Builder{
		ID:     id,
		Status: build.Status,
	}
	if commit != nil {
		builder.StatusTime = time.Time(commit.Committer.Time)
		builder.StatusRevision = commit.Commit
	} else {
		builder.StatusTime = build.CreationTime
	}
	return builder
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

		// If we have revision information, but the revision isn't new...
		//
		// ...if it's the same revision...
		case commit != nil && commit.Commit == prev.StatusRevision:
			return nil

		// ...or if it's an older revision.
		case commit != nil && commit.Commit != prev.StatusRevision && prev.StatusTime.After(time.Time(commit.Committer.Time)):
			logging.Debugf(c, "build %d (%s) from commit at %s, is not new", build.ID, build.Status, commit.Committer.Time)
			return nil

		// If we're in the fallback case (i.e. no revision information) check against creation time.
		case commit == nil && prev.StatusRevision == "" && prev.StatusTime.After(build.CreationTime):
			// Don't update the datastore if the new build is not newer.
			logging.Debugf(c, "build %d (%s) created at %s, is not new", build.ID, build.Status, build.CreationTime)
			return nil
		}

		return datastore.Put(c, NewBuilder(id, build, commit))
	}, nil)
	if err != nil {
		return nil, err
	}
	return prev, nil
}
