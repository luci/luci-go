// Copyright 2016 The LUCI Authors.
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

package buildbot

import (
	"fmt"
	"strings"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/sync/parallel"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common/model"

	"golang.org/x/net/context"
)

// getFullBuilds fetches all of the recent builds from the datastore.
func getFullBuilds(c context.Context, masterName, builderName string, finished bool) ([]*buildbotBuild, error) {
	// TODO(hinoka): Builder specific structs.
	q := ds.NewQuery("buildbotBuild")
	q = q.Eq("finished", finished)
	q = q.Eq("master", masterName)
	q = q.Eq("builder", builderName)
	q = q.Order("-number")
	q.Finalize()
	// Ignore the cursor, we don't need it.
	buildbots, _, err := runBuildsQuery(c, q, 25)
	return buildbots, err
}

// GetConsoleBuilds takes commits and builders and returns a matrix of
// resp.ConsoleBuild objects.  The expected format of the result
// is [len(commits)][len(builders)]*resp.ConsoleBuild.  The first level matches
// 1:1 to a commit, and the second level matches 1:1 to a builder.
func GetConsoleBuilds(
	c context.Context, builders []resp.BuilderRef, commits []string) (
	[][]*resp.ConsoleBuild, error) {

	results := make([][]*resp.ConsoleBuild, len(commits))
	for i := range results {
		results[i] = make([]*resp.ConsoleBuild, len(builders))
	}
	// HACK(hinoka): This fetches 25 full builds and then filters them. Replace this
	// with something more reasonable.
	// This is kind of a hack but it's okay for now.
	err := parallel.FanOutIn(func(taskC chan<- func() error) {
		for i, builder := range builders {
			i := i
			builder := builder
			builderComponents := strings.SplitN(builder.Name, "/", 2)
			if len(builderComponents) != 2 {
				taskC <- func() error {
					return fmt.Errorf("%s is an invalid builder name", builder.Name)
				}
				return
			}
			master := builderComponents[0]
			builderName := builderComponents[1]
			taskC <- func() error {
				t1 := clock.Now(c)
				builds, err := getFullBuilds(c, master, builderName, true)
				if err != nil {
					return err
				}
				t2 := clock.Now(c)
				var currentStatus *model.Status
				for j, commit := range commits {
					for _, build := range builds {
						if build.Sourcestamp.Revision == commit {
							results[j][i] = &resp.ConsoleBuild{
								Link: resp.NewLink(
									strings.Join(build.Text, " "),
									fmt.Sprintf("/buildbot/%s/%s/%d", master, builderName, build.Number),
								),
								Status: build.toStatus(),
							}
							currentStatus = &results[j][i].Status
						}
					}
					if currentStatus != nil && results[j][i] == nil {
						results[j][i] = &resp.ConsoleBuild{Status: *currentStatus}
					}
				}
				log.Debugf(c,
					"Builder %s took %s to query, %s to compute.", builderName,
					t2.Sub(t1), clock.Since(c, t2))
				return nil
			}
		}
	})
	return results, err
}
