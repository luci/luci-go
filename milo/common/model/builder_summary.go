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
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/logging"
)

// BuilderSummary holds builder state for the purpose of representing e.g. header consoles.
type BuilderSummary struct {
	// BuilderID is the global identifier for the builder that this Build belongs to, i.e.:
	//   "buildbot/<mastername>/<buildername>"
	//   "buildbucket/<bucketname>/<buildername>"
	// Matches field in BuildSummary.
	BuilderID string `gae:"$id"`

	// The LUCI project ID associated with this build. This is used for generating links.
	ProjectID string

	// LastFinishedCreated is the time the last finished build was created.
	LastFinishedCreated time.Time

	// LastFinishedStatus is the status of last finished build on builder.
	LastFinishedStatus Status

	// LastFinishedBuildID is the BuildID of the BuildSummary associated with last finished build on
	// the builder.
	LastFinishedBuildID string

	// Consoles lists consoles of which this builder is part.
	// Elements of this list should be of the form
	// <common.Console.GetProjectID()>/<common.Console.ID>.
	Consoles []string // indexed on this

	// Ignore unrecognized fields and strip in future writes.
	_ datastore.PropertyMap `gae:"-,extra"`
}

// LastFinishedBuildIDLink returns a link to the last finished build.
func (b *BuilderSummary) LastFinishedBuildIDLink() string {
	return getLinkFromBuildID(b.LastFinishedBuildID, b.ProjectID)
}

// UpdateBuilderForBuild updates the appropriate BuilderSummary for the
// provided BuildSummary, if needed.
// In particular, a BuilderSummary is updated with a BuildSummary if the latter is marked complete
// and has a more recent creation time than the one stored in the BuilderSummary.
// If there is no existing BuilderSummary for the BuildSummary provided, one is created.
// Idempotent.
// c must have a current datastore transaction.
func UpdateBuilderForBuild(c context.Context, build *BuildSummary) error {
	if datastore.CurrentTransaction(c) == nil {
		panic("UpdateBuilderForBuild was called outside of a transaction")
	}

	if !build.Summary.Status.Terminal() {
		// this build did not complete yet.
		// There is no new info for the builder.
		// Do not even bother starting a transaction.
		return nil
	}

	// Get or create the relevant BuilderSummary.
	builder := &BuilderSummary{BuilderID: build.BuilderID}
	switch err := datastore.Get(c, builder); {
	case err == datastore.ErrNoSuchEntity:
		logging.Warningf(c, "creating new BuilderSummary for BuilderID: %s", build.BuilderID)
	case err != nil:
		return err
	case build.Created.Before(builder.LastFinishedCreated):
		logging.Warningf(c, "message for build %s is out of order", build.BuildID)
		return nil
	}

	// Overwrite previous value of ProjectID in case a builder has moved to
	// another project.
	builder.ProjectID = build.ProjectID
	builder.LastFinishedCreated = build.Created
	builder.LastFinishedStatus = build.Summary.Status
	builder.LastFinishedBuildID = build.BuildID
	return datastore.Put(c, builder)
}
