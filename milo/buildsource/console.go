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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// ConsoleRow is one row of a particular console.
//
// It has the git commit for the row, as well as a mapping of BuilderID to any
// BuildSummaries which reported using this commit.
type ConsoleRow struct {
	Commit string
	Builds map[BuilderID][]*model.BuildSummary
}

// GetConsoleRows returns a row-oriented collection of BuildSummary
// objects. Each row corresponds to the similarly-indexed commit in the
// `commits` slice.
func GetConsoleRows(c context.Context, project string, console *common.Console, commits, builders []string) ([]*ConsoleRow, error) {
	rawCommits := make([][]byte, len(commits))
	for i, c := range commits {
		var err error
		if rawCommits[i], err = hex.DecodeString(c); err != nil {
			return nil, errors.Annotate(err, "bad commit[%d]: %q", i, c).Err()
		}
	}

	builderSet := stringset.NewFromSlice(builders...)

	ret := make([]*ConsoleRow, len(commits))
	url := console.RepoURL
	// HACK(iannucci): This little hack should be removed when console definitions
	// no longer use a manifest name of "REVISION". REVISION was used to index the
	// 'got_revision' value before manifests were implemented.
	if console.ManifestName == "REVISION" {
		url = ""
	}
	partialKey := model.NewPartialManifestKey(project, console.ID, console.ManifestName, url)
	q := datastore.NewQuery("BuildSummary")
	err := parallel.WorkPool(4, func(ch chan<- func() error) {
		for i := range rawCommits {
			i := i
			r := &ConsoleRow{Commit: commits[i]}
			ret[i] = r
			ch <- func() error {
				fullQ := q.Eq("ManifestKeys", partialKey.AddRevision(rawCommits[i]))
				return datastore.Run(c, fullQ, func(bs *model.BuildSummary) {
					if builderSet.Has(bs.BuilderID) {
						bid := BuilderID(bs.BuilderID)
						if r.Builds == nil {
							r.Builds = map[BuilderID][]*model.BuildSummary{}
						}
						r.Builds[bid] = append(r.Builds[bid], bs)
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
				summary, err := GetConsoleSummary(c, id)
				if err == nil {
					summaries[i] = *summary
				}
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
func GetConsoleSummary(c context.Context, consoleID string) (*ui.ConsoleSummary, error) {
	summary := &ui.ConsoleSummary{}
	// It's safe to SplitN because we assume the ID has already been validated.
	consoleComponents := strings.SplitN(consoleID, "/", 2)
	project := consoleComponents[0]
	ID := consoleComponents[1]

	// Set Name label.
	ariaLabel := fmt.Sprintf("Console %s in project %s", ID, project)
	summary.Name = ui.NewLink(ID, fmt.Sprintf("/p/%s/consoles/%s", project, ID), ariaLabel)

	// Fetch the config first.
	def, err := common.GetConsole(c, project, ID)
	if err != nil {
		return nil, errors.Annotate(err, "getting %s", consoleID).Err()
	}

	// Fetch the data from datastore.
	builders := make([]*model.BuilderSummary, len(def.Builders))
	for i, builderID := range def.Builders {
		builders[i] = &model.BuilderSummary{BuilderID: builderID}
	}
	err = datastore.Get(c, builders)
	if err != nil {
		// Return an error only if we encouter an error other than datastore.ErrNoSuchEntity.
		me := err.(errors.MultiError)
		newME := errors.MultiError{}
		for i, ierr := range me {
			if ierr == datastore.ErrNoSuchEntity {
				builders[i] = &model.BuilderSummary{BuilderID: def.Builders[i]}
			} else {
				newME = append(newME, ierr)
			}
		}
		if newME.First() != nil {
			return nil, newME
		}
	}

	summary.Builders = builders
	return summary, nil
}
