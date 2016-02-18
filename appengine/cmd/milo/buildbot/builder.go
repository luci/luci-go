// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/luci/luci-go/appengine/cmd/milo/model"
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
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

// getBuilds fetches all of the recent builds from the back end.
func getBuilds(c context.Context, masterName, builderName string) ([]*resp.BuildRef, error) {
	result := []*resp.BuildRef{}
	bs, err := model.GetBuilds(c, []model.BuildRoot{
		model.GetBuildRoot(c, masterName, builderName)}, 25)
	if err != nil {
		return nil, err
	}
	for _, b := range bs[0] {
		mb := &resp.MiloBuild{
			Summary: resp.BuildComponent{
				Started: b.ExecutionTime.Format(time.RFC3339),
				// TODO(hinoka/martiniss): Also get the real finished time and duration.
				Finished: b.ExecutionTime.Format(time.RFC3339),
				Status: func(status string) resp.Status {
					switch status {
					case "SUCCESS":
						return resp.Success
					case "FAILURE":
						return resp.Failure
					default:
						// TODO(hinoka): Also implement the other status types.
						return resp.InfraFailure
					}
				}(b.UserStatus),
				// TODO(martiniss): Implement summary text.
				Text: []string{"Coming Soon...."},
			},
			// TODO(hinoka/martiniss): Also get the repo so it's a real sourcestamp so
			// that the commit can be linkable.
			SourceStamp: &resp.SourceStamp{
				Commit: resp.Commit{
					Revision: b.Revisions[0].Digest,
				},
			},
		}
		result = append(result, &resp.BuildRef{
			URL: func(url string) string {
				r, err := regexp.Compile(".*/builds/(\\d+)/.*")
				if err != nil {
					panic(err)
				}
				return r.FindStringSubmatch(url)[1]
			}(b.BuildLogKey),
			Build: mb,
		})
	}
	return result, nil
}

// getCurrentBuild extracts a build from a map of current builds, and translates
// it into the milo version of the build.
func getCurrentBuild(bMap buildMap, builder string, buildNum int) *resp.BuildRef {
	b := bMap[builderRef{builder, buildNum}]
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
func getCurrentBuilds(master *buildbotMaster, builderName string) []*resp.BuildRef {
	b := master.Builders[builderName]
	results := make([]*resp.BuildRef, len(b.Currentbuilds))
	bMap := createRunningBuildMap(master)
	for i, bn := range b.Currentbuilds {
		results[i] = getCurrentBuild(bMap, builderName, bn)
	}
	return results
}

// builderImpl is the implementation for getting a milo builder page from buildbot.
// This gets:
// * Current Builds from querying the master json from buildbot
// * Recent Builds from a cron job that backfills the recent builds.
func builderImpl(c context.Context, masterName, builderName string) (*resp.MiloBuilder, error) {
	result := &resp.MiloBuilder{}
	master, err := getMaster(c, masterName)
	if err != nil {
		return nil, fmt.Errorf("Cannot find master %s\n%s", masterName, err.Error())
	}

	_, ok := master.Builders[builderName]
	if !ok {
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
	currentBuilds := getCurrentBuilds(master, builderName)
	fmt.Fprintf(os.Stderr, "Number of current builds: %d\n", len(currentBuilds))
	result.CurrentBuilds = currentBuilds
	for _, fb := range recentBuilds {
		if fb != nil {
			result.FinishedBuilds = append(result.FinishedBuilds, fb)
		}
	}
	return result, nil
}
