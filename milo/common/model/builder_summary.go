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
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// BuildMessageOutOfOrderTag indicates that a terminal build message arrived before any pending
// build messages.
var BuildMessageOutOfOrderTag = errors.BoolTag{
	Key: errors.NewTagKey("Got out of order build messages."),
}

// pendingBuild holds information about builds that are still in progress for a builder.
type pendingBuild struct {
	// BuildID is the ID of a Build.
	BuildID string

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

	// LastFinishedCreated is the time the last finished build was created.
	LastFinishedCreated time.Time

	// LastFinishedStatus is the status of last finished build on builder.
	LastFinishedStatus Status

	// LastFinishedBuildID is the BuildID of the BuildSummary associated with last finished build on
	// the builder.
	LastFinishedBuildID string

	// Consoles lists consoles of which this builder is part.
	// Elements of this list should be of the form
	// <common.Console.GetProjectName()>/<common.Console.ID>.
	Consoles []string // indexed on this

	// InProgress tracks builds that are currently still in progress.
	InProgress []pendingBuild // derive pending/running counts
}

// GetInProgress returns the pending builds (internally represented as an array) as a map.
func (b *BuilderSummary) GetInProgress() map[string]Status {
	bp := make(map[string]Status, len(b.InProgress))
	for _, bld := range b.InProgress {
		bp[bld.BuildID] = bld.Status
	}
	return bp
}

// SetInProgress stores the given map of pending builds back into the internal array representation.
func (b *BuilderSummary) SetInProgress(bp map[string]Status) {
	b.InProgress = make([]pendingBuild, 0, len(bp))
	for id, bs := range bp {
		b.InProgress = append(b.InProgress, pendingBuild{id, bs})
	}
}

// UpdateBuilderForBuild updates the appropriate BuilderSummary for the provided BuildSummary.
// In particular, a BuilderSummary is updated with a BuildSummary if the latter is marked complete
// and has a more recent creation time than the one stored in the BuilderSummary.
// If there is no existing BuilderSummary for the BuildSummary provided, one is created.
func UpdateBuilderForBuild(c context.Context, build *BuildSummary) error {
	// Get or create the relevant BuilderSummary.
	builder := BuilderSummary{BuilderID: build.BuilderID}
	switch err := datastore.Get(c, &builder); err {
	case nil:
	case datastore.ErrNoSuchEntity:
		logging.Warningf(c, "creating new BuilderSummary for BuilderID: %s", build.BuilderID)
		datastore.Put(c, &builder)
	default:
		return err
	}

	// Update BuilderSummary with new BuildSummary. If there's an BuildMessageOutOfOrderTag, we
	// still want to try to update, so in fact, just log and ignore.
	switch err := builder.Update(c, build); {
	case err == nil:
	case BuildMessageOutOfOrderTag.In(err):
		logging.WithError(err).Warningf(c, "got out of order build messages, ignoring")
	default:
		return err
	}

	return datastore.Put(c, &builder)
}

// Update updates the given BuilderSummary with the provided BuildSummary.
// In particular, a BuilderSummary is updated with a BuildSummary if the latter is marked complete
// and has a more recent creation time than the one stored in the BuilderSummary.
func (b *BuilderSummary) Update(c context.Context, build *BuildSummary) error {
	if b.BuilderID != build.BuilderID {
		return fmt.Errorf(
			"updating wrong builder %s for build %v (should be %s)",
			b.BuilderID, build.BuildID, build.BuilderID)
	}

	// Update console strings list.
	b.Consoles = make([]string, 0, len(build.consoles))
	for _, con := range build.consoles {
		b.Consoles = append(b.Consoles, fmt.Sprintf("%s/%s", con.GetProjectName(), con.ID))
	}

	// Update builder's InProgress with given build.
	// The only kind of error we /should/ get is if a terminal build message arrives before any
	// pending build messages. In that case, we still want to update the builder's last finished build
	// info if applicable, and re-raise the error after. Otherwise, return the error immediately.
	err := b.updateInProgress(build)
	if err != nil && !BuildMessageOutOfOrderTag.In(err) {
		return err
	}

	if !build.Summary.Status.Terminal() {
		return nil
	}

	// Check if we can bail from updating builder's last complete build state.
	// TODO(jchinlee): Backfilled builds have the wrong creation time; use revision comparison.
	if b.LastFinishedBuildID != "" && build.Created.Before(b.LastFinishedCreated) {
		return err
	}

	b.LastFinishedCreated = build.Created
	b.LastFinishedStatus = build.Summary.Status
	b.LastFinishedBuildID = build.BuildID

	// Re-raise update error if applicable.
	return err
}

// updateInProgress updates the InProgress builds, and returns whether the overall update
// needs additional processing.
// In particular, a build that has not terminated doesn't update builder last finished build state.
func (b *BuilderSummary) updateInProgress(build *BuildSummary) error {
	bid := build.BuildID
	st := build.Summary.Status

	bp := b.GetInProgress()
	if st.Terminal() {
		// If we have a terminal state, the build is no longer InProgress so remove it.
		if _, ok := bp[bid]; !ok {
			// If the build was never InProgress, that's an error. This should never happen, as earlier
			// parts of the pipeline take care of it, so throw an error.
			return errors.Reason(
				"finished build %v that was not in-progress for builder %s", bid, b.BuilderID).Tag(
				BuildMessageOutOfOrderTag).Err()
		}

		delete(bp, bid)
	} else {
		// Otherwise just update in the map, whether it's a newly pending or existing pending build.
		bp[bid] = st
	}

	b.SetInProgress(bp)
	return nil
}
