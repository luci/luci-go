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
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
)

// InvalidBuilderIDURL is returned if a BuilderID cannot be parsed and a URL generated.
const InvalidBuilderIDURL = "#invalid-builder-id"

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

// Name returns the builder name (i.e. stripped of bucket and master).
func (b *BuilderSummary) Name() string {
	parts := strings.Split(b.BuilderID, "/")
	if len(parts) != 3 {
		return b.BuilderID
	}

	return parts[2]
}

// SelfLink returns a link to the associated builder.
// Depends on routes.go.
func (b *BuilderSummary) SelfLink() string {
	parts := strings.Split(b.BuilderID, "/")
	if len(parts) != 3 {
		return InvalidBuilderIDURL
	}

	switch source := parts[0]; source {
	case "buildbot":
		return fmt.Sprintf("/buildbot/%s/%s", parts[1], parts[2])
	case "buildbucket":
		return fmt.Sprintf("/p/%s/builders/%s/%s", b.ProjectID, parts[1], parts[2])
	default:
		return InvalidBuilderIDURL
	}
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

type builderHistory struct {
	// BuilderSummary associated with this builder.
	Builder BuilderSummary

	// Number of pending builds in this builder.
	NumPending int

	// Number of running builds in this builder.
	NumRunning int

	// Up to last N build summaries from this builder.
	LastNBuilds []BuildSummary
}

// FetchPendingFailedTag indicates that fetching a builder's pending builds failed.
var FetchPendingFailedTag = errors.BoolTag{Key: errors.NewTagKey("Failed to fetch pending builds")}

// FetchRunningFailedTag indicates that fetching a builder's running builds failed.
var FetchRunningFailedTag = errors.BoolTag{Key: errors.NewTagKey("Failed to fetch running builds")}

// FetchLastNFailedTag indicates that fetching a builder's most recent completed builds failed.
var FetchLastNFailedTag = errors.BoolTag{Key: errors.NewTagKey("Failed to fetch last N builds")}

// GetBuilderHistories gets the recent histories for the builders in the given project.
func GetBuilderHistories(c context.Context, projectName string, limit int) ([]builderHistory, error) {
	// Get builders and sort by BuilderID for stable order.
	builders := []*BuilderSummary{}
	q := datastore.NewQuery("BuilderSummary").Eq("ProjectID", projectName)
	err := datastore.GetAll(c, q, &builders)
	if err != nil {
		return []builderHistory{}, err
	}
	sort.Slice(
		builders,
		func(i, j int) bool {
			return strings.Compare(builders[i].BuilderID, builders[j].BuilderID) < 0
		},
	)

	// Populate the recent histories.
	hists := make([]builderHistory, len(builders))
	err = parallel.WorkPool(4, func(ch chan<- func() error) {
		for i, builder := range builders {
			i := i
			builder := builder
			ch <- func() error {
				hist, err := getHistory(c, builder, limit)
				if err != nil {
					logging.Errorf(c, "error populating history for builder %s: %v", builder.BuilderID, err)
					return err
				}
				hists[i] = hist
				return nil
			}
		}
	})

	return hists, err
}

// Get the recent history of the given builder.
// Depends on status.go for filtering finished builds.
func getHistory(c context.Context, builder *BuilderSummary, limit int) (builderHistory, error) {
	id := builder.BuilderID

	//  Set up queries for pending, running, and last {limit} builds.
	p := datastore.NewQuery("BuildSummary").
		Eq("BuilderID", id).
		Eq("Summary.Status", NotRun)

	r := datastore.NewQuery("BuildSummary").
		Eq("BuilderID", id).
		Eq("Summary.Status", Running)

	l := datastore.NewQuery("BuildSummary").
		Eq("BuilderID", id).
		Order("-Created")

	hist := builderHistory{Builder: *builder}
	annot := errors.Reason("failed to fetch history for builder %s", id)
	hasErr := false

	// Get pending and running builds.
	pending, running := []*BuildSummary{}, []*BuildSummary{}
	if err := datastore.GetAll(c, p, &pending); err != nil {
		logging.Errorf(c, "error fetching pending builds for builder %s: %v", id, err)
		annot = annot.Tag(FetchPendingFailedTag)
		hasErr = true
	}
	if err := datastore.GetAll(c, r, &running); err != nil {
		logging.Errorf(c, "error fetching running builds for builder %s: %v", id, err)
		annot = annot.Tag(FetchRunningFailedTag)
		hasErr = true
	}

	// Get last {limit} builds.
	last := make([]BuildSummary, 0, limit)
	var i int
	err := datastore.Run(c, l, func(b *BuildSummary) error {
		if b.Summary.Status.Terminal() {
			i++
			last = append(last, *b)
		}

		if i > limit {
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(c, "error fetching last %d builds for builder %s: %v", limit, id, err)
		annot = annot.Tag(FetchLastNFailedTag)
		hasErr = true
	}

	// Set hist fields and return.
	hist.NumPending = len(pending)
	hist.NumRunning = len(running)
	hist.LastNBuilds = last

	if hasErr {
		return hist, annot.Err()
	}
	return hist, nil
}
