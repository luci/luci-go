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
	// It is computed as the creation time of the build that caused a change
	// of Status.
	StatusTime time.Time

	// StatusRevision can be used to decide whether Status should be updated.
	// It is the revision of the codebase that's associated with the build
	// that caused a change of Status.
	StatusRevision string
}

// StatusUnknown is used in the LookupBuilder return value
// if builder status is unknown.
var StatusUnknown buildbucket.Status = -1

// NewBuilder creates a new builder from an ID, a build, and optionally a commit.
func NewBuilder(id, revision string, build *buildbucket.Build) *Builder {
	return &Builder{
		ID:               id,
		Status:           build.Status,
		StatusTime:       build.CreationTime,
		StatusRevision:   revision,
	}
}

// WithBuilder Gets a Builder from the datastore and executes a function which
// with that Builder as a parameter.
//
// The function executes within a datastore transaction, so it can perform
// additional datastore operations with a transactional guarantee.
func WithBuilder(c context.Context, id string, f func(*Builder) error) error {
	prev := &Builder{ID: id}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, prev); {
		case err == datastore.ErrNoSuchEntity:
			prev.Status = StatusUnknown
			prev.StatusTime = time.Time{}
			logging.Debugf(c, "found no builder %q", prev.ID)
		case err != nil:
			return err
		}
		return f(prev)
	}, nil)
}
