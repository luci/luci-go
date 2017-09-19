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
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_notify/buildbucket"
	"go.chromium.org/luci/server/auth"
)

// BuilderState represents the state of the last build seen from a particular
// builder in order to implement certain notification triggers (i.e. on change).
type BuilderState struct {
	// ID is the builder's canonical ID (e.g. buildbucket/bucket/name).
	ID string `gae:"$id"`

	// LastBuildTime is the creation timestamp of the last known build for a builder.
	LastBuildTime int64

	// LastBuildResult is the build result of the last known build for a builder.
	LastBuildResult string
}

// NewBuilderState constructs a new BuilderState.
//
// This is intended to maintain a consistent interface to datastore models and
// mimics the behavior of NewProject and NewNotifier.
func NewBuilderState(id string, build *buildbucket.BuildInfo) *BuilderState {
	return &BuilderState{
		ID:              id,
		LastBuildTime:   build.Build.CreatedTs,
		LastBuildResult: build.Build.Result,
	}
}

// UpdateDatastore puts the BuilderState into the datastore.
func (s *BuilderState) UpdateDatastore(c context.Context) error {
	if err := datastore.Put(c, s); err != nil {
		return errors.Annotate(err, "saving %s state", s.ID).Err()
	}
	return nil
}

// findPreviousBuild uses buildbucket's search API to try to find a "previous" build for a given build.
//
// The results are returned by updating the information inside of a given BuilderState.
func findPreviousBuild(c context.Context, build *buildbucket.BuildInfo, state *BuilderState) {
	// Setup buildbucket API
	transport, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		logging.WithError(err).Warningf(c, "failed to get RPC transport")
		return
	}
	svc, err := bbapi.New(&http.Client{Transport: transport})
	if err != nil {
		logging.WithError(err).Warningf(c, "failed to create buildbucket client")
		return
	}
	svc.BasePath = fmt.Sprintf("https://%s/_ah/api/buildbucket/v1/", build.Hostname)
	// Search buildbucket in pages until we find a build from this builder, or we reach the end.
	logging.Debugf(c, "Searching for `builder:%s` in bucket `%s`", build.Parameters.BuilderName, build.Build.Bucket)
	res, err := svc.Search().
		Status("COMPLETED").
		Bucket(build.Build.Bucket).
		Tag(fmt.Sprintf("builder:%s", build.Parameters.BuilderName)).
		Context(c).
		Do()
	if err != nil {
		logging.WithError(err).Warningf(c, "failed to search buildbucket")
		return
	}
	for _, b := range res.Builds {
		if b.CreatedTs < build.Build.CreatedTs {
			// Populate the state with the newest build retrieved.
			logging.Debugf(c, "Found old build from time %d vs. %d", b.CreatedTs, build.Build.CreatedTs)
			state.LastBuildTime = b.CreatedTs
			state.LastBuildResult = b.Result
			break
		}
	}
}

// LookupBuilderState returns a "previous" build for `build` as a BuilderState.
//
// This function tries its hardest to not return anything. It first checks the
// datastore, and if that fails, it tries the buildbucket search API. If that
// fails, we are left with a default which is conservative (e.g. the last result
// is "UNKNOWN" is that comparisons against current builds always fail, and we notify
// on change).
//
// It will also update the builder state in the datastore appropriately.
func LookupBuilderState(c context.Context, build *buildbucket.BuildInfo) *BuilderState {
	state := &BuilderState{
		ID:              build.GetBuilderID(),
		LastBuildResult: "UNKNOWN",
	}
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, state); err != nil {
			return err
		}
		// Don't update the datastore if the new build is actually older.
		if build.Build.CreatedTs <= state.LastBuildTime {
			logging.Debugf(c, "found old build: %d %s", build.Build.CreatedTs, build.Build.Result)
			return nil
		}
		return NewBuilderState(state.ID, build).UpdateDatastore(c)
	}, nil)
	if err == datastore.ErrNoSuchEntity {
		findPreviousBuild(c, build, state)
	}
	logging.Debugf(c, "Got state: {%s, %s, %d}", state.ID, state.LastBuildResult, state.LastBuildTime)
	return state
}
