// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common/miloerror"
	"golang.org/x/net/context"
)

// builderRef is used for keying specific builds in a master json.
type builderRef struct {
	builder  string
	buildNum int
}

// buildMap contains all of the current build within a master json.  We use this
// because buildbot returns all current builds as within the slaves portion, whereas
// it's eaiser to map thenm by builders instead.
type buildMap map[builderRef]*buildbotBuild

func getBuildSummary(b *buildbotBuild) *resp.BuildSummary {
	started, finished, duration := parseTimes(b.Times)
	return &resp.BuildSummary{
		Link: &resp.Link{
			URL:   fmt.Sprintf("%d", b.Number),
			Label: fmt.Sprintf("#%d", b.Number),
		},
		Status: b.toStatus(),
		ExecutionTime: resp.Interval{
			Started:  started,
			Finished: finished,
			Duration: duration,
		},
		Text:     b.Text,
		Blame:    blame(b),
		Revision: b.Sourcestamp.Revision,
	}
}

// getBuilds fetches all of the recent builds from the .  Note that
// getBuilds() does not perform ACL checks.
func getBuilds(
	c context.Context, masterName, builderName string, finished bool, limit int) (
	[]*resp.BuildSummary, error) {

	// TODO(hinoka): Builder specific structs.
	result := []*resp.BuildSummary{}
	q := ds.NewQuery("buildbotBuild")
	q = q.Eq("finished", finished)
	q = q.Eq("master", masterName)
	q = q.Eq("builder", builderName)
	if limit != 0 {
		q = q.Limit(int32(limit))
	}
	q = q.Order("-number")
	buildbots := []*buildbotBuild{}
	err := ds.GetAll(c, q, &buildbots)
	if err != nil {
		return nil, err
	}
	for _, b := range buildbots {
		result = append(result, getBuildSummary(b))
	}
	return result, nil
}

var errMasterNotFound = miloerror.Error{
	Message: "Master not found",
	Code:    http.StatusNotFound,
}

// builderImpl is the implementation for getting a milo builder page from buildbot.
// This gets:
// * Current Builds from querying the master json from the datastore.
// * Recent Builds from a cron job that backfills the recent builds.
func builderImpl(c context.Context, masterName, builderName string, limit int) (*resp.Builder, error) {
	result := &resp.Builder{
		Name: builderName,
	}
	master, t, err := getMasterJSON(c, masterName)
	if err != nil {
		return nil, err
	}
	if clock.Now(c).Sub(t) > 2*time.Minute {
		warning := fmt.Sprintf(
			"WARNING: Master data is stale (last updated %s)", t)
		logging.Warningf(c, warning)
		result.Warning = warning
	}

	p, ok := master.Builders[builderName]
	if !ok {
		// This long block is just to return a good error message when an invalid
		// buildbot builder is specified.
		keys := make([]string, 0, len(master.Builders))
		for k := range master.Builders {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		avail := strings.Join(keys, "\n")
		return nil, fmt.Errorf(
			"Cannot find builder %s in master %s.\nAvailable builders: \n%s",
			builderName, masterName, avail)
	}
	// Extract pending builds out of the master json.
	result.PendingBuilds = make([]*resp.BuildSummary, len(p.PendingBuildStates))
	logging.Debugf(c, "Number of pending builds: %d", len(p.PendingBuildStates))
	for i, pb := range p.PendingBuildStates {
		start := time.Unix(int64(pb.SubmittedAt), 0)
		result.PendingBuilds[i] = &resp.BuildSummary{
			PendingTime: resp.Interval{
				Started:  start,
				Duration: time.Now().Sub(start),
			},
		}
		result.PendingBuilds[i].Blame = make([]*resp.Commit, len(pb.Source.Changes))
		for j, cm := range pb.Source.Changes {
			result.PendingBuilds[i].Blame[j] = &resp.Commit{
				AuthorEmail: cm.Who,
				CommitURL:   cm.Revlink,
			}
		}
	}

	// This is CPU bound anyways, so there's no need to do this in parallel.
	finishedBuilds, err := getBuilds(c, masterName, builderName, true, limit)
	if err != nil {
		return nil, err
	}
	currentBuilds, err := getBuilds(c, masterName, builderName, false, 0)
	if err != nil {
		return nil, err
	}
	result.CurrentBuilds = currentBuilds
	for _, fb := range finishedBuilds {
		if fb != nil {
			result.FinishedBuilds = append(result.FinishedBuilds, fb)
		}
	}
	return result, nil
}
