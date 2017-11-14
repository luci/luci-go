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

package buildsource

import (
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// ConsoleRow is one row of a particular console.
//
// It has the git commit for the row, as well as a mapping of BuilderID to any
// BuildSummaries which reported using this commit.
//
// For Builder definitions which had more than one BuilderID assoaciated with
// them, they're indexed by the first BuilderID in the console's definition. So
// if the Console has a Builder with name: "A", and name: "B", only "A" will
// show up the Builds map (but if you look at the []BuildSummary, you'll see
// builds from both "A" and "B").
type ConsoleRow struct {
	Commit string
	Builds map[BuilderID][]*model.BuildSummary
}

// GetConsoleRows returns a row-oriented collection of BuildSummary
// objects. Each row corresponds to the similarly-indexed commit in the
// `commits` slice.
func GetConsoleRows(c context.Context, project string, console *config.Console, commits []string) ([]*ConsoleRow, error) {
	rawCommits := make([][]byte, len(commits))
	for i, c := range commits {
		var err error
		if rawCommits[i], err = hex.DecodeString(c); err != nil {
			return nil, errors.Annotate(err, "bad commit[%d]: %q", i, c).Err()
		}
	}

	// Maps BuilderID -> Column BuilderID (see ConsoleRow).
	columnMap := map[string]BuilderID{}
	for _, b := range console.Builders {
		columnID := b.Name[0]
		columnMap[columnID] = BuilderID(columnID)
		for _, n := range b.Name[1:] {
			columnMap[n] = BuilderID(columnID)
		}
	}

	ret := make([]*ConsoleRow, len(commits))
	url := console.RepoUrl
	// HACK(iannucci): This little hack should be removed when console definitions
	// no longer use a manifest name of "REVISION". REVISION was used to index the
	// 'got_revision' value before manifests were implemented.
	if console.ManifestName == "REVISION" {
		url = ""
	}
	partialKey := model.NewPartialManifestKey(project, console.Id, console.ManifestName, url)
	q := datastore.NewQuery("BuildSummary")
	err := parallel.WorkPool(4, func(ch chan<- func() error) {
		for i := range rawCommits {
			i := i
			r := &ConsoleRow{Commit: commits[i]}
			ret[i] = r
			ch <- func() error {
				fullQ := q.Eq("ManifestKeys", partialKey.AddRevision(rawCommits[i]))
				return datastore.Run(c, fullQ, func(bs *model.BuildSummary) {
					if bs.Experimental && !console.IncludeExperimentalBuilds {
						return
					}
					if i, ok := columnMap[bs.BuilderID]; ok {
						if r.Builds == nil {
							r.Builds = map[BuilderID][]*model.BuildSummary{}
						}
						r.Builds[i] = append(r.Builds[i], bs)
					}
				})
			}
		}
	})

	return ret, err
}

// ConsolePreview is mapping of builder IDs to each builder's latest build.
//
// This reflects a console preview, which is a console table except rendered
// with only the builder's latest build.
type ConsolePreview map[BuilderID]*model.BuildSummary

// GetConsoleSummaries returns a list of console summaries from the datastore.
//
// This list of console summaries directly corresponds to the input list of
// console IDs.
func GetConsoleSummaries(c context.Context, consoleIDs []string) ([]ui.ConsoleSummary, error) {
	summaries := make([]ui.ConsoleSummary, len(consoleIDs))
	err := parallel.WorkPool(4, func(ch chan<- func() error) {
		for i, id := range consoleIDs {
			i := i
			id := id
			ch <- func() error {
				var err error
				summaries[i], err = GetConsoleSummary(c, id)
				return err
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

// GetConsoleSummary returns a single console summary from the datastore.
func GetConsoleSummary(c context.Context, consoleID string) (ui.ConsoleSummary, error) {
	summary := ui.ConsoleSummary{}
	// It's safe to SplitN because we assume the ID has already been validated.
	consoleComponents := strings.SplitN(consoleID, "/", 2)
	project := consoleComponents[0]
	id := consoleComponents[1]

	// Set Name label.
	ariaLabel := fmt.Sprintf("Console %s in project %s", id, project)
	summary.Name = ui.NewLink(id, fmt.Sprintf("/p/%s/consoles/%s", project, id), ariaLabel)

	// Fetch the config first.
	def, err := common.GetConsole(c, project, id)
	if err != nil {
		return summary, errors.Annotate(err, "getting %s", consoleID).Err()
	}

	// Fetch the data from datastore.
	summary.Builders = make([]*model.BuilderSummary, len(def.Builders))
	for i, builderID := range def.Builders {
		summary.Builders[i] = &model.BuilderSummary{BuilderID: builderID}
	}
	if err = datastore.Get(c, summary.Builders); err != nil {
		// Return an error only if we encouter an error other than datastore.ErrNoSuchEntity.
		me := err.(errors.MultiError)
		lme := errors.NewLazyMultiError(len(me))
		for i, ierr := range me {
			if ierr == datastore.ErrNoSuchEntity {
				summary.Builders[i] = &model.BuilderSummary{BuilderID: def.Builders[i]}
			} else {
				lme.Assign(i, ierr)
			}
		}
		err = lme.Get()
	}

	return summary, err
}
