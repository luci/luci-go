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
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_notify/buildbucket"
)

// Builder represents the state of the last build seen from a particular
// builder in order to implement certain notification triggers (i.e. on change).
type Builder struct {
	// ID is the builder's canonical ID (e.g. buildbucket/bucket/name).
	ID string `gae:"$id"`

	// LastBuildTime is the creation timestamp of the last known build for a builder.
	LastBuildTime int64

	// LastBuildResult is the build result of the last known build for a builder.
	LastBuildResult string
}

// NewBuilder constructs a new Builder.
//
// This is intended to maintain a consistent interface to datastore models and
// mimics the behavior of NewProject and NewNotifier.
func NewBuilder(id string, build *buildbucket.BuildInfo) *Builder {
	return &Builder{
		ID:              id,
		LastBuildTime:   build.Build.CreatedTs,
		LastBuildResult: build.Build.Result,
	}
}

// LookupBuilder returns a "previous" build for `build` as a Builder.
//
// If no "previous" build is found in the datastore, then we return a Builder
// whose last build result is UNKNOWN in order to make explicit that we have
// never recorded information about this builder before.
//
// It will also update the Builder in the datastore appropriately.
func LookupBuilder(c context.Context, build *buildbucket.BuildInfo) (*Builder, error) {
	builder := &Builder{
		ID:              build.BuilderID(),
		LastBuildResult: "UNKNOWN",
	}
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		err := datastore.Get(c, builder)
		switch err {
		case nil:
			// Don't update the datastore if the new build is actually older.
			if build.Build.CreatedTs <= builder.LastBuildTime {
				logging.Debugf(c, "found old build: %d %s", build.Build.CreatedTs, build.Build.Result)
				return nil
			}
		case datastore.ErrNoSuchEntity:
			logging.Debugf(c, "found no builder %q", builder.ID)
		default:
			return err
		}
		return datastore.Put(c, NewBuilder(builder.ID, build))
	}, nil)
	if err != nil {
		return nil, err
	}
	return builder, nil
}
