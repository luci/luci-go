// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/api/resp"
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

// mergeText merges buildbot summary texts, which sometimes separates
// words that should be merged together, this combines them into a single
// line.
func mergeText(text []string) []string {
	result := make([]string, 0, len(text))
	merge := false
	for _, line := range text {
		if merge {
			merge = false
			result[len(result)-1] += " " + line
			continue
		}
		result = append(result, line)
		switch line {
		case "build", "failed", "exception":
			merge = true
		default:
			merge = false
		}
	}

	// We can remove error messages about the step "steps" if it's part of a longer
	// message because this step is an artifact of running on recipes and it's
	// not important to users.
	if len(result) > 1 {
		switch result[0] {
		case "failed steps", "exception steps":
			result = result[1:]
		}
	}
	return result
}

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
		Text:     mergeText(b.Text),
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
	q := datastore.NewQuery("buildbotBuild")
	q = q.Eq("finished", finished)
	q = q.Eq("master", masterName)
	q = q.Eq("builder", builderName)
	if limit != 0 {
		q = q.Limit(int32(limit))
	}
	q = q.Order("-number")
	buildbots := []*buildbotBuild{}
	err := getBuildQueryBatcher(c).GetAll(c, q, &buildbots)
	if err != nil {
		return nil, err
	}
	for _, b := range buildbots {
		result = append(result, getBuildSummary(b))
	}
	return result, nil
}

var errMasterNotFound = errors.New(
	"Either the request resource was not found or you have insufficient permissions")
var errNotAuth = errors.New("You are not authenticated, try logging in")

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
	// currentBuilds is presented in reversed order, so flip it
	for i, j := 0, len(currentBuilds)-1; i < j; i, j = i+1, j-1 {
		currentBuilds[i], currentBuilds[j] = currentBuilds[j], currentBuilds[i]
	}
	result.CurrentBuilds = currentBuilds

	for _, fb := range finishedBuilds {
		if fb != nil {
			result.FinishedBuilds = append(result.FinishedBuilds, fb)
		}
	}
	return result, nil
}
