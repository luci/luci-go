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
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/internal/model/milostatus"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
)

// InvalidBuilderIDURL is returned if a BuilderID cannot be parsed and a URL generated.
const InvalidBuilderIDURL = "#invalid-builder-id"

// BuilderSummaryKind is the name of the datastore entity kind of the
// entity represented by the BuilderSummary struct.
const BuilderSummaryKind = "BuilderSummary"

// BuilderSummary holds builder state for the purpose of representing e.g. header consoles.
type BuilderSummary struct {
	// BuilderID is the global identifier for the builder that this Build belongs to, i.e.:
	//   "buildbucket/<bucketname>/<buildername>"
	// Matches field in BuildSummary.
	BuilderID string `gae:"$id"`

	// The LUCI project ID associated with this build. This is used for generating links.
	ProjectID string

	// LastFinishedCreated is the time the last finished build was created.
	LastFinishedCreated time.Time

	// LastFinishedStatus is the status of last finished build on builder.
	LastFinishedStatus milostatus.Status

	// LastFinishedCritical is the criticality of last finished build on builder.
	LastFinishedCritical buildbucketpb.Trinary

	// LastFinishedBuildID is the BuildID of the BuildSummary associated with last finished build on
	// the builder.
	LastFinishedBuildID string

	// LastFinishedExperimental indicates if the last build on this builder was
	// marked as experimental.
	LastFinishedExperimental bool

	// Ignore unrecognized fields and strip in future writes.
	_ datastore.PropertyMap `gae:"-,extra"`
}

// LastFinishedBuildIDLink returns a link to the last finished build.
func (b *BuilderSummary) LastFinishedBuildIDLink() string {
	return buildIDLink(b.LastFinishedBuildID, b.ProjectID)
}

// SelfLink returns a link to the associated builder.
func (b *BuilderSummary) SelfLink() string {
	if b == nil {
		return InvalidBuilderIDURL
	}
	return BuilderIDLink(b.BuilderID, b.ProjectID)
}

// BuilderIDLink gets a builder link from builder ID and project.
// Depends on routes.go.
func BuilderIDLink(b, project string) string {
	parts := strings.Split(b, "/")
	if len(parts) != 3 {
		return InvalidBuilderIDURL
	}

	if parts[0] == "buildbucket" {
		return fmt.Sprintf("/p/%s/builders/%s/%s", project, parts[1], parts[2])
	}
	return InvalidBuilderIDURL
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
	builder.LastFinishedBuildID = build.BuildID
	builder.LastFinishedCreated = build.Created
	builder.LastFinishedExperimental = build.Experimental
	builder.LastFinishedStatus = build.Summary.Status
	builder.LastFinishedCritical = build.Critical
	return datastore.Put(c, builder)
}
