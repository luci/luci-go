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
	"math"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
)

// pendingBuild holds information about builds that are still in progress for a builder.
type pendingBuild struct {
	BuildKey *datastore.Key
	Status Status
}

// BuilderSummary holds builder state for the purpose of representing e.g. header consoles.
type BuilderSummary struct {
	// Global identifier for the builder that this Build belongs to, i.e.:
	//   "buildbot/<mastername>/<buildername>"
	//   "buildbucket/<bucketname>/<buildername>"
	// Matches field in BuildSummary.
	BuilderID string

	// Status of last finished build on builder.
	LastFinishedStatus Status

	// ID of last finished build on builder.
	LastFinishedID *datastore.Key

	// Consoles of which this builder is part.
	Consoles []string // indexed on this

	// Builds that are currently still in progress.
	InProgress []pendingBuild // derive pending/running counts
}

// Update updates the provided BuilderSummary with the provided BuildSummary, if applicable.
// In particular, a BuilderSummary is updated with a BuildSummary if the latter is marked complete
// and has a more recent creation time than the one stored in the BuilderSummary.
func Update(c context.Context, builder *BuilderSummary, build *BuildSummary) error {
	if builder.BuilderID != build.BuilderID {
		panic(fmt.Sprintf(
			"updating wrong builder %s for build %v (should be %s)",
			builder.BuilderID, build.BuildKey, build.BuilderID))
	}

	// Update builder's InProgress with given build.
	if cont := builder.updateInProgressAndContinue(build); !cont {
		return nil
	}

	// Get build creation info to update builder's last complete build state if applicable.
	last := &BuildSummary{BuildKey: builder.LastFinishedID}
	if err := datastore.Get(c, last); err != nil {
		return err
	}

	if build.Created.Before(last.Created) {
		return nil
	}

	builder.LastFinishedStatus = build.Summary.Status
	builder.LastFinishedID = build.BuildKey
	return nil
}

// updateInProgressAndContinue updates the InProgress builds, and returns whether the overall update
// needs additional processing.
// In particular, a build that has not terminated doesn't update builder last finished build state.
func (b *BuilderSummary) updateInProgressAndContinue(build *BuildSummary) bool {
	bk := build.BuildKey
	st := build.Summary.Status

	// Get indices relevant to affected build. If the build was InProgress, replace it with the new
	// state. Otherwise, append.
	var i  = 0
	for _, bld := range b.InProgress {
		if bld.BuildKey == bk {
			break
		}
		i++
	}
	k := int(math.Min(float64(i+1), float64(len(b.InProgress))))

	pb := pendingBuild{
		BuildKey: bk,
		Status: build.Summary.Status,
	}

	// Update actual InProgress array and return if further builder update is needed.
	switch {
	// If the build hasn't finished, just update the InProgress status.
	case !st.Terminal():
		b.InProgress = append(
			b.InProgress[:i],
			append([]pendingBuild{pb}, b.InProgress[k:]...)...)
		return false

	// Otherwise, the build is no longer InProgress for the builder so delete build.
	case st.Terminal() && i < len(b.InProgress):
		b.InProgress = append(b.InProgress[:i], b.InProgress[k:]...)
		return true

	// A build shouldn't finish before it's marked InProgress.
	case st.Terminal() && i >= len(b.InProgress):
		panic(fmt.Sprintf("finished build %v that was not in-progress for builder %s", bk, b.BuilderID))

	// The above cases partition the space of possibilities.
	default:
		panic("impossible")
	}
}
