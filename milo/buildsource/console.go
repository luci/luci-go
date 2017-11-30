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

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/data/stringset"
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
	err := parallel.WorkPool(16, func(ch chan<- func() error) {
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

// GetConsoleSummariesFromIDs returns a list of console summaries from the datastore
// using a slice of console IDs as input.
//
// This list of console summaries directly corresponds to the input list of
// console IDs.
func GetConsoleSummariesFromIDs(c context.Context, consoleIDs []common.ConsoleID) (
	map[common.ConsoleID]ui.ConsoleSummary, error) {

	// Get the console definitions and builders, then rearrange them into console summaries.
	defs, err := common.GetConsoles(c, consoleIDs)
	if err != nil {
		return nil, err
	}
	return GetConsoleSummariesFromDefs(c, defs)
}

// GetConsoleSummariesFromDefs returns a list of console summaries from the datastore
// using a slice of console definitions as input.
//
// This list of console summaries directly corresponds to the input list of
// console definition entities.
func GetConsoleSummariesFromDefs(c context.Context, defs []*common.Console) (map[common.ConsoleID]ui.ConsoleSummary, error) {
	builders := stringset.New(len(defs) * 10) // We'll start with approx 10 builders per console.
	for _, def := range defs {
		for _, builder := range def.Builders {
			builders.Add(builder)
		}
	}
	bs := make([]*model.BuilderSummary, 0, builders.Len())
	builders.Iter(func(builderID string) bool {
		bs = append(bs, &model.BuilderSummary{BuilderID: builderID})
		return true
	})
	if err := datastore.Get(c, bs); err != nil {
		// Return an error only if we encouter an error other than datastore.ErrNoSuchEntity.
		if err = common.ReplaceNSEWith(err.(errors.MultiError), nil); err != nil {
			return nil, err
		}
	}
	// Rearrange the result into a map, for easy access later.
	bsMap := make(map[string]*model.BuilderSummary, len(bs))
	for _, b := range bs {
		bsMap[b.BuilderID] = b
	}

	// Now rearrange the builders into their respective consoles
	summaries := make(map[common.ConsoleID]ui.ConsoleSummary, len(defs))
	for _, def := range defs {
		ariaLabel := fmt.Sprintf("Console %s in project %s", def.ID, def.Project())
		summary := ui.ConsoleSummary{
			Builders: make([]*model.BuilderSummary, len(def.Builders)),
			Name: ui.NewLink(
				def.ID, fmt.Sprintf("/p/%s/g/%s/console", def.Project(), def.ID), ariaLabel),
		}
		for i, builderID := range def.Builders {
			if builder, ok := bsMap[builderID]; ok {
				summary.Builders[i] = builder
			} else {
				// This should never happen.
				panic(fmt.Sprintf("%s disappeared somehow", builderID))
			}
		}
		summaries[def.ConsoleID()] = summary
	}
	return summaries, nil
}
