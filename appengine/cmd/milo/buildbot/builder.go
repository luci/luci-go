// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	log "github.com/luci/luci-go/common/logging"
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

// createRunningBuildMap extracts all of the running builds in a master json
// from the various slaves and dumps it into a map for easy reference.
func createRunningBuildMap(master *buildbotMaster) buildMap {
	result := buildMap{}
	for _, slave := range master.Slaves {
		for _, build := range slave.Runningbuilds {
			result[builderRef{build.Buildername, build.Number}] = &build
		}
	}
	return result
}

// getBuilds fetches all of the recent builds from the datastore.
func getBuilds(c context.Context, masterName, builderName string) ([]*resp.BuildRef, error) {
	// TODO(hinoka): Builder specific structs.
	result := []*resp.BuildRef{}
	ds := datastore.Get(c)
	q := datastore.NewQuery("buildbotBuild")
	q = q.Eq("finished", true)
	q = q.Eq("master", masterName)
	q = q.Eq("builder", builderName)
	q = q.Limit(25) // TODO(hinoka): This should be adjustable
	q = q.Order("-number")
	buildbots := []*buildbotBuild{}
	err := ds.GetAll(q, &buildbots)
	if err != nil {
		return nil, err
	}
	for _, b := range buildbots {
		result = append(result, &resp.BuildRef{
			URL:   fmt.Sprintf("%d", b.Number),
			Label: fmt.Sprintf("#%d", b.Number),
			Build: &resp.MiloBuild{
				// We only need the summary for the build view.
				Summary: summary(b),
				SourceStamp: &resp.SourceStamp{
					Commit: resp.Commit{
						Revision: b.Sourcestamp.Revision,
					},
				},
			},
		})
	}
	return result, nil
}

// getCurrentBuild extracts a build from a map of current builds, and translates
// it into the milo version of the build.
func getCurrentBuild(c context.Context, bMap buildMap, builder string, buildNum int) *resp.BuildRef {
	b, ok := bMap[builderRef{builder, buildNum}]
	if !ok {
		log.Warningf(c, "Could not find %s/%d in builder map:\n %s", builder, buildNum, bMap)
		return nil
	}
	return &resp.BuildRef{
		Build: &resp.MiloBuild{
			Summary:       summary(b),
			Components:    components(b),
			PropertyGroup: properties(b),
			Blame:         []*resp.Commit{},
		},
		URL: fmt.Sprintf("%d", buildNum),
	}
}

// getCurrentBuilds extracts the list of all the current builds from a master json
// from the slaves' runningBuilds portion.
func getCurrentBuilds(c context.Context, master *buildbotMaster, builderName string) []*resp.BuildRef {
	b := master.Builders[builderName]
	results := []*resp.BuildRef{}
	bMap := createRunningBuildMap(master)
	for _, bn := range b.Currentbuilds {
		cb := getCurrentBuild(c, bMap, builderName, bn)
		if cb != nil {
			results = append(results, cb)
		}
	}
	return results
}

// builderImpl is the implementation for getting a milo builder page from buildbot.
// This gets:
// * Current Builds from querying the master json from the datastore.
// * Recent Builds from a cron job that backfills the recent builds.
func builderImpl(c context.Context, masterName, builderName string) (*resp.MiloBuilder, error) {
	result := &resp.MiloBuilder{}
	master, internal, t, err := getMasterJSON(c, masterName)
	if internal {
		// TODO(hinoka): Implement ACL support and remove this.
		return nil, fmt.Errorf("Internal masters are not yet supported.")
	}
	if err != nil {
		return nil, fmt.Errorf("Cannot find master %s\n%s", masterName, err.Error())
	}
	// TODO(hinoka): Warning check for data >2 min stale.
	t = t

	s, _ := json.Marshal(master)
	log.Debugf(c, "Master: %s", s)

	_, ok := master.Builders[builderName]
	if !ok {
		// This long block is just to return a good error message when an invalid
		// buildbot builder is specified.
		keys := make([]string, len(master.Builders))
		i := 0
		for k := range master.Builders {
			keys[i] = k
			i++
		}
		return nil, fmt.Errorf(
			"Cannot find builder %s in master %s.\nAvailable builders: %s",
			builderName, masterName, keys)
	}

	recentBuilds, err := getBuilds(c, masterName, builderName)
	if err != nil {
		return nil, err // Or maybe not?
	}
	currentBuilds := getCurrentBuilds(c, master, builderName)
	fmt.Fprintf(os.Stderr, "Number of current builds: %d\n", len(currentBuilds))
	result.CurrentBuilds = currentBuilds
	for _, fb := range recentBuilds {
		// Yes recent builds is synonymous with finished builds.
		// TODO(hinoka): Implement limits.
		if fb != nil {
			result.FinishedBuilds = append(result.FinishedBuilds, fb)
		}
	}
	return result, nil
}
