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

package model

import (
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
)

type ErrBuildMessageOutOfOrder struct {
	error
}

// pendingBuild holds information about builds that are still in progress for a builder.
type pendingBuild struct {
	// BuildKey is the string representation of *datastore.Key BuildKey.
	BuildKey string

	// Status is the status of the build whose key's string representation is above.
	Status Status
}

// BuilderSummary holds builder state for the purpose of representing e.g. header consoles.
type BuilderSummary struct {
	// BuilderID is the global identifier for the builder that this Build belongs to, i.e.:
	//   "buildbot/<mastername>/<buildername>"
	//   "buildbucket/<bucketname>/<buildername>"
	// Matches field in BuildSummary.
	BuilderID string `gae:"$id"`

	// LastFinishedStatus is the status of last finished build on builder.
	LastFinishedStatus Status

	// LastFinishedID is the parent key of the BuildSummary associated with last finished build on the
	// builder.
	LastFinishedID *datastore.Key

	// Consoles lists consoles of which this builder is part.
	Consoles []string // indexed on this

	// InProgress tracks builds that are currently still in progress.
	InProgress []pendingBuild // derive pending/running counts
}

// GetInProgress returns the pending builds (internally represented as an array) as a map.
func (b *BuilderSummary) GetInProgress() map[string]Status {
	bp := make(map[string]Status, len(b.InProgress))
	for _, bld := range b.InProgress {
		bp[bld.BuildKey] = bld.Status
	}
	return bp
}

// SetInProgress stores the given map of pending builds back into the internal array representation.
func (b *BuilderSummary) SetInProgress(bp map[string]Status) {
	b.InProgress = make([]pendingBuild, 0, len(bp))
	for bk, bs := range bp {
		b.InProgress = append(b.InProgress, pendingBuild{bk, bs})
	}
}

// Update updates the provided BuilderSummary with the provided BuildSummary, if applicable.
// In particular, a BuilderSummary is updated with a BuildSummary if the latter is marked complete
// and has a more recent creation time than the one stored in the BuilderSummary.
func (b *BuilderSummary) Update(c context.Context, build *BuildSummary) error {
	if b.BuilderID != build.BuilderID {
		return fmt.Errorf(
			"updating wrong builder %s for build %v (should be %s)",
			b.BuilderID, build.BuildKey, build.BuilderID)
	}

	// Update builder's InProgress with given build.
	// The only kind of error we /should/ get is if a terminal build message arrives before any
	// pending build messages. In that case, we still want to update the builder's last finished build
	// info if applicable, and re-raise the error after. Otherwise, return the error immediately.
	updateErr := b.updateInProgress(build)
	if updateErr != nil {
		if _, ok := updateErr.(ErrBuildMessageOutOfOrder); !ok {
			return updateErr
		}
	}
	if !build.Summary.Status.Terminal() {
		return nil
	}

	// Check if we can bail from updating builder's last complete build state.
	if b.LastFinishedID != nil {
		last := &BuildSummary{BuildKey: b.LastFinishedID}
		if err := datastore.Get(c, last); err != nil {
			return err
		}

		// TODO(jchinlee): Backfilled builds have the wrong creation time; use revision comparison.
		if build.Created.Before(last.Created) {
			return updateErr
		}
	}

	b.LastFinishedStatus = build.Summary.Status
	b.LastFinishedID = build.BuildKey

	// Re-raise update error if applicable.
	return updateErr
}

// updateInProgress updates the InProgress builds, and returns whether the overall update
// needs additional processing.
// In particular, a build that has not terminated doesn't update builder last finished build state.
func (b *BuilderSummary) updateInProgress(build *BuildSummary) error {
	bk := build.BuildKey.String()
	st := build.Summary.Status

	bp := b.GetInProgress()
	if st.Terminal() {
		// If we have a terminal state, the build is no longer InProgress so remove it.
		if _, ok := bp[bk]; !ok {
			// If the build was never InProgress, that's an error. This should never happen, as earlier
			// parts of the pipeline take care of it, so throw an error.
			return ErrBuildMessageOutOfOrder{
				fmt.Errorf("finished build %v that was not in-progress for builder %s", bk, b.BuilderID)}
		}

		delete(bp, bk)
	} else {
		// Otherwise just update in the map, whether it's a newly pending or existing pending build.
		bp[bk] = st
	}

	b.SetInProgress(bp)
	return nil
}
