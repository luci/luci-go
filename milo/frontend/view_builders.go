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

package frontend

import (
	"sort"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
)

// BuildersRelativeHandler is responsible for rendering a builders console page according to project.
// Presently only relative time is handled, i.e. last builds without correlation between builders),
// and no filtering by console has been implemented.
//
// The builders console page by relative time is defined in
// ./appengine/templates/pages/builders_relative_time.html.
func BuildersRelativeHandler(c *router.Context, projectID, console string) {
	limit := 30
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}

	hists, err := getBuilderHistories(c.Context, projectID, console, limit)
	if err != nil {
		ErrorHandler(c, err)
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/builders_relative_time.html", templates.Args{
		"Builders": hists,
	})
}

// builderHistory stores the recent history of a builder.
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
	RecentBuilds []*model.BuildSummary
}

// getBuilderHistories gets the recent histories for the builders in the given project.
func getBuilderHistories(c context.Context, project, console string, limit int) ([]*builderHistory, error) {
	builders, err := getBuildersForProject(c, project, console)
	if err != nil {
		return nil, err
	}

	// Populate the recent histories.
	hists := make([]*builderHistory, len(builders))
	err = parallel.WorkPool(16, func(ch chan<- func() error) {
		for i, builder := range builders {
			i := i
			builder := builder
			ch <- func() error {
				hist, err := getHistory(c, builder, project, limit)
				if err != nil {
					return errors.Annotate(
						err, "error populating history for builder %s", builder).Err()
				}
				hists[i] = hist
				return nil
			}
		}
	})

	if err != nil {
		return nil, err
	}
	return hists, nil
}

// getBuildersForProject gets the sorted builder IDs associated with the given project.
func getBuildersForProject(c context.Context, project, console string) ([]string, error) {
	var cons []*common.Console

	// Get consoles for project and extract builders into set.
	projKey := datastore.MakeKey(c, "Project", project)
	if console == "" {
		q := datastore.NewQuery("Console").Ancestor(projKey)
		if err := datastore.GetAll(c, q, &cons); err != nil {
			return nil, errors.Annotate(
				err, "error getting consoles for project %s: %v", project).Err()
		}
	} else {
		con := common.Console{Parent: projKey, ID: console}
		switch err := datastore.Get(c, &con); err {
		case nil:
		case datastore.ErrNoSuchEntity:
			return nil, errors.Annotate(
				err, "error getting console %s in project %s", console, project).
				Tag(common.CodeNotFound).Err()
		default:
			return nil, errors.Annotate(
				err, "error getting console %s in project %s", console, project).Err()
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
	sort.Strings(builders)
	return builders, nil
}

// getHistory gets the recent history of the given builder.
// Depends on status.go for filtering finished builds.
func getHistory(c context.Context, builderID, project string, limit int) (*builderHistory, error) {
	hist := builderHistory{
		BuilderID:    builderID,
		BuilderLink:  model.BuilderIDLink(builderID, project),
		RecentBuilds: make([]*model.BuildSummary, 0, limit),
	}

	// Do fetches.
	return &hist, parallel.FanOutIn(func(fetch chan<- func() error) {
		// Pending builds
		fetch <- func() error {
			q := datastore.NewQuery("BuildSummary").
				Eq("BuilderID", builderID).
				Eq("Summary.Status", model.NotRun)
			pending, err := datastore.Count(c, q)
			hist.NumPending = int(pending)
			return err
		}

		// Running builds
		fetch <- func() error {
			q := datastore.NewQuery("BuildSummary").
				Eq("BuilderID", builderID).
				Eq("Summary.Status", model.Running)
			running, err := datastore.Count(c, q)
			hist.NumRunning = int(running)
			return err
		}

		// Last {limit} builds
		fetch <- func() error {
			q := datastore.NewQuery("BuildSummary").
				Eq("BuilderID", builderID).
				Order("-Created")
			return datastore.Run(c, q, func(b *model.BuildSummary) error {
				if !b.Summary.Status.Terminal() {
					return nil
				}

				hist.RecentBuilds = append(hist.RecentBuilds, b)
				if len(hist.RecentBuilds) >= limit {
					return datastore.Stop
				}
				return nil
			})
		}
	})
}
