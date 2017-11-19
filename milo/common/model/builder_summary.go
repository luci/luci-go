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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/common"
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
	return buildIDLink(b.LastFinishedBuildID, b.ProjectID)
}

// SelfLink returns a link to the associated builder.
func (b *BuilderSummary) SelfLink() string {
	return builderIDLink(b.BuilderID, b.ProjectID)
}

// builderIDLink gets a builder link from builder ID and project.
// Depends on routes.go.
func builderIDLink(b, project string) string {
	parts := strings.Split(b, "/")
	if len(parts) != 3 {
		return InvalidBuilderIDURL
	}

	switch source := parts[0]; source {
	case "buildbot":
		return fmt.Sprintf("/buildbot/%s/%s", parts[1], parts[2])
	case "buildbucket":
		return fmt.Sprintf("/p/%s/builders/%s/%s", project, parts[1], parts[2])
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
	// ID of Builder associated with this builder.
	BuilderID string

	// Link associated with this builder.
	BuilderLink string

	// Number of pending builds in this builder.
	NumPending int

	// Number of running builds in this builder.
	NumRunning int

	// Recent build summaries from this builder.
	RecentBuilds []*BuildSummary
}

// FetchPendingFailedTag indicates that fetching a builder's pending builds failed.
var FetchPendingFailedTag = errors.BoolTag{Key: errors.NewTagKey("Failed to fetch pending builds")}

// FetchRunningFailedTag indicates that fetching a builder's running builds failed.
var FetchRunningFailedTag = errors.BoolTag{Key: errors.NewTagKey("Failed to fetch running builds")}

// FetchLastNFailedTag indicates that fetching a builder's most recent completed builds failed.
var FetchLastNFailedTag = errors.BoolTag{Key: errors.NewTagKey("Failed to fetch last N builds")}

// GetBuilderHistories gets the recent histories for the builders in the given project.
func GetBuilderHistories(c context.Context, project, view string, limit int) ([]builderHistory, error) {
	builders, err := getBuildersForProject(c, project, view)
	if err != nil {
		return []builderHistory{}, err
	}

	// Populate the recent histories.
	hists := make([]builderHistory, len(builders))
	err = parallel.WorkPool(16, func(ch chan<- func() error) {
		for i, builder := range builders {
			i := i
			builder := builder
			ch <- func() error {
				hist, err := getHistory(c, builder, project, limit)
				if err != nil {
					logging.Errorf(c, "error populating history for builder %s: %v", builder, err)
					return err
				}
				hists[i] = *hist
				return nil
			}
		}
	})

	return hists, err
}

// getBuildersForProject gets the sorted builder IDs associated with the given project.
func getBuildersForProject(c context.Context, project, view string) ([]string, error) {
	var cons []*common.Console

	// Get consoles for project and extract builders into set.
	projKey := datastore.MakeKey(c, "Project", project)
	if view == "" {
		q := datastore.NewQuery("Console").Ancestor(projKey)
		if err := datastore.GetAll(c, q, &cons); err != nil {
			return nil, errors.Annotate(
				err, fmt.Sprintf("error getting consoles for project %s: %v", project)).Err()
		}
	} else {
		con := common.Console{Parent: projKey, ID: view}
		if err := datastore.Get(c, &con); err != nil {
			return nil, errors.Annotate(
				err, fmt.Sprintf("error getting console %s in project %s", view, project)).Err()
		}
		cons = append(cons, &con)
	}

	bSet := stringset.New(0)
	for _, con := range cons {
		for _, builder := range con.Builders {
			bSet.Add(builder)
		}
	}

	// Get sorted builders.
	builders := bSet.ToSlice()
	sort.Slice(
		builders,
		func(i, j int) bool {
			return strings.Compare(builders[i], builders[j]) < 0
		},
	)
	return builders, nil
}

// getHistory gets the recent history of the given builder.
// Depends on status.go for filtering finished builds.
func getHistory(c context.Context, builderID, project string, limit int) (*builderHistory, error) {
	hist := builderHistory{
		BuilderID:    builderID,
		BuilderLink:  builderIDLink(builderID, project),
		RecentBuilds: make([]*BuildSummary, 0, limit),
	}

	// Do fetches.
	err := parallel.FanOutIn(func(fetch chan<- func() error) {
		// Pending builds
		fetch <- func() error {
			pending, err := datastore.Count(
				c,
				datastore.NewQuery("BuildSummary").Eq("BuilderID", builderID).Eq("Summary.Status", NotRun))
			hist.NumPending = int(pending)
			return err
		}

		// Running builds
		fetch <- func() error {
			running, err := datastore.Count(
				c,
				datastore.NewQuery("BuildSummary").Eq("BuilderID", builderID).Eq("Summary.Status", Running))
			hist.NumRunning = int(running)
			return err
		}

		// Last {limit} builds
		fetch <- func() error {
			l := datastore.NewQuery("BuildSummary").
				Eq("BuilderID", builderID).
				Order("-Created")
			err := datastore.Run(c, l, func(b *BuildSummary) error {
				if !b.Summary.Status.Terminal() {
					return nil
				}

				hist.RecentBuilds = append(hist.RecentBuilds, b)
				if len(hist.RecentBuilds) >= limit {
					return datastore.Stop
				}
				return nil
			})
			return err
		}
	})

	if err != nil {
		return nil, err
	}
	return &hist, nil
}
