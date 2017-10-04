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
	// It is computed as the creation time of the build that caused
	// a change of Status.
	StatusTime time.Time
}

// NewBuilder constructs a new Builder.
//
// This is intended to maintain a consistent interface to datastore models and
// mimics the behavior of NewProject and NewNotifier.
func NewBuilder(id string, build *buildbucket.Build) *Builder {
	return &Builder{
		ID:         id,
		Status:     build.Status,
		StatusTime: build.CreationTime,
	}
}

// StatusUnknown is used in the LookupBuilder return value
// if builder status is unknown.
var StatusUnknown buildbucket.Status = -1

// LookupBuilder returns a "previous" builder state.
//
// If no "previous" build is found in the datastore, then returns a Builder
// with Status==StatusUnknown in order to make explicit that we
// have never recorded information about this builder before.
//
// It will also update the Builder in the datastore according to build.
func LookupBuilder(c context.Context, id string, build *buildbucket.Build) (*Builder, error) {
	prev := &Builder{ID: id}
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, prev); {
		case err == datastore.ErrNoSuchEntity:
			prev.Status = StatusUnknown
			prev.StatusTime = time.Time{}
			logging.Debugf(c, "found no builder %q", prev.ID)

		case err != nil:
			return err

		case prev.StatusTime.After(build.CreationTime):
			// Don't update the datastore if the new build is not newer.
			logging.Debugf(c, "build %d (%s) created at %s, is not new", build.ID, build.Status, build.CreationTime)
			return nil
		}

		return datastore.Put(c, NewBuilder(prev.ID, build))
	}, nil)
	if err != nil {
		return nil, err
	}
	return prev, nil
}
