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
	"context"
	"fmt"
	"sort"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/buildbucket/deprecated"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
)

// BuildersRelativeHandler is responsible for rendering a builders list page according to project.
// Presently only relative time is handled, i.e. last builds without correlation between builders),
// and no filtering by group has been implemented.
//
// The builders list page by relative time is defined in
// ./appengine/templates/pages/builders_relative_time.html.
func BuildersRelativeHandler(c *router.Context, projectID, group string) error {
	limit := 30
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}

	var buildersFromConfig, buildersFromSwarmbucket []string
	err := parallel.FanOutIn(func(ch chan<- func() error) {
		// This grab builders from the project config, defined in luci-milo.cfg.
		ch <- func() (err error) {
			buildersFromConfig, err = getBuildersForProject(c.Context, projectID, group)
			if err != nil {
				return err
			}
			buildersFromConfig, err = filterAuthorizedBuilders(c.Context, buildersFromConfig)
			return err
		}
		// TODO(hinoka): Support implicit groups mode by bucket.
		if group != "" {
			return
		}
		// This grab builders from swarmbucket, defined in cr-buildbucket.cfg.
		// We do this when the user requests to see all builders in a project.
		ch <- func() error {
			// This call does it's own ACL checks.
			builders, err := buildbucket.GetBuilders(c.Context)
			if err != nil {
				return err
			}
			for _, bBucket := range builders.Buckets {
				// This uses v1 style bucket names, which are luci.project.bucket.
				project, bucket := deprecated.BucketNameToV2(bBucket.Name)
				if project != projectID {
					continue
				}
				for _, builder := range bBucket.Builders {
					bid := buildbucket.BuilderID{
						buildbucketpb.BuilderID{
							Project: project,
							Bucket:  bucket,
							Builder: builder.Name,
						},
					}
					buildersFromSwarmbucket = append(buildersFromSwarmbucket, bid.String())
				}
			}
			return nil
		}
	})
	builders := common.MergeStrings(buildersFromConfig, buildersFromSwarmbucket)

	if len(builders) == 0 {
		return errors.New("No such project or group.", common.CodeNotFound)
	}

	// Get the histories.
	hists, err := getBuilderHistories(c.Context, builders, projectID, limit)
	if err != nil {
		return err
	}

	templates.MustRender(c.Context, c.Writer, "pages/builders_relative_time.html", templates.Args{
		"Builders": hists,
	})
	return nil
}

// filterAuthorizedBuilders filters out builders that the user does not have access to.
func filterAuthorizedBuilders(c context.Context, builders []string) ([]string, error) {
	buckets := stringset.New(0)
	for _, b := range builders {
		id := buildsource.BuilderID(b)
		buildType, bucket, _, err := id.Split()
		if err != nil {
			logging.Warningf(c, "found malformed builder ID %q", id)
			continue
		}
		if buildType == "buildbucket" {
			buckets.Add(bucket)
		}
	}
	perms, err := common.BucketPermissions(c, buckets.ToSlice()...)
	if err != nil {
		return nil, err
	}
	okBuilders := make([]string, 0, len(builders))
	for _, b := range builders {
		id := buildsource.BuilderID(b)
		buildType, bucket, _, err := id.Split()
		if err != nil {
			continue
		}
		if buildType != "buildbucket" || perms.Can(bucket, access.AccessBucket) {
			okBuilders = append(okBuilders, b)
		}
	}
	return okBuilders, nil
}

// builderHistory stores the recent history of a builder.
type builderHistory struct {
	// ID of Builder associated with this builder.
	BuilderID buildsource.BuilderID

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
func getBuilderHistories(c context.Context, builders []string, project string, limit int) ([]*builderHistory, error) {
	// Populate the recent histories.
	hists := make([]*builderHistory, len(builders))
	err := parallel.WorkPool(16, func(ch chan<- func() error) {
		for i, builder := range builders {
			i := i
			builder := builder
			ch <- func() error {
				hist, err := getHistory(c, buildsource.BuilderID(builder), project, limit)
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

	// for all buildbot builders, we don't have pending BuildSummary entities,
	// so load pending counts from buildstore.
	var buildbotBuilders []string // "<master>/<builder>" strings
	var buildbotHists []*builderHistory
	for _, h := range hists {
		backend, master, builder, err := h.BuilderID.Split()
		if backend == "buildbot" && err == nil {
			buildbotHists = append(buildbotHists, h)
			buildbotBuilders = append(buildbotBuilders, fmt.Sprintf("%s/%s", master, builder))
		}
	}
	if len(buildbotBuilders) > 0 {
		pendingCounts, err := buildstore.GetPendingCounts(c, buildbotBuilders)
		if err != nil {
			return nil, err
		}
		for i, count := range pendingCounts {
			buildbotHists[i].NumPending = count
		}
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
				err, "error getting consoles for project %s", project).Err()
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
// If the builder starts with "buildbot/", does not load pending builds.
func getHistory(c context.Context, builderID buildsource.BuilderID, project string, limit int) (*builderHistory, error) {
	hist := builderHistory{
		BuilderID:    builderID,
		BuilderLink:  builderID.SelfLink(project),
		RecentBuilds: make([]*model.BuildSummary, 0, limit),
	}

	// Do fetches.
	return &hist, parallel.FanOutIn(func(fetch chan<- func() error) {
		// Get BuilderSummary for builderID, because the passed project may differ
		// from the BuilderSummary's actual project (e.g. a chrome builder that also
		// appears in the chromium console will pass "chromium" as the project and
		// fail to link it). If no BuilderSummary is found, fall back on passed
		// project.
		fetch <- func() error {
			matchingBuilder := model.BuilderSummary{BuilderID: string(builderID)}
			switch err := datastore.Get(c, &matchingBuilder); err {
			case nil:
				hist.BuilderLink = matchingBuilder.SelfLink()
			case datastore.ErrNoSuchEntity:
				// fall back
			default:
				return err
			}
			return nil
		}

		if !builderID.Buildbot() {
			// Pending builds
			fetch <- func() error {
				q := datastore.NewQuery("BuildSummary").
					Eq("BuilderID", builderID).
					Eq("Summary.Status", model.NotRun)
				pending, err := datastore.Count(c, q)
				hist.NumPending = int(pending)
				return err
			}
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
