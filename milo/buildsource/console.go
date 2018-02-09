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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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

	// IMPORTANT: Maps BuilderID -> Column BuilderID (see ConsoleRow comments).
	// This is indexed based off the first builder name, even if the second
	// builder name is the one we end up using.
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
//
// projectID is the project being served in the current request.
func GetConsoleSummariesFromIDs(c context.Context, consoleIDs []common.ConsoleID, projectID string) (
	map[common.ConsoleID]*ui.ConsoleSummary, error) {

	// Get the console definitions and builders, then rearrange them into console summaries.
	defs, err := common.GetConsoles(c, consoleIDs)
	if err != nil {
		return nil, err
	}
	return GetConsoleSummariesFromDefs(c, defs, projectID)
}

// GetConsoleSummariesFromDefs returns a map of consoleID -> summary from the
// datastore using a slice of console definitions as input.
//
// This expects all builders in all consoles coming from the same projectID.
func GetConsoleSummariesFromDefs(c context.Context, consoleEnts []*common.Console, projectID string) (
	map[common.ConsoleID]*ui.ConsoleSummary, error) {

	// Maps consoleID -> console config definition.
	consoles := make(map[common.ConsoleID]*config.Console, len(consoleEnts))

	// Maps the BuilderID to the per-console pointer-to-summary in the summaries
	// map. Note that builders with multiple builderIDs in the same console will
	// all map to the same BuilderSummary.
	columns := map[BuilderID]map[common.ConsoleID]*model.BuilderSummary{}

	// The return result.
	summaries := map[common.ConsoleID]*ui.ConsoleSummary{}

	for _, ent := range consoleEnts {
		cid := ent.ConsoleID()
		consoles[cid] = &ent.Def

		summaries[cid] = &ui.ConsoleSummary{
			Builders: make([]*model.BuilderSummary, len(ent.Def.Builders)),
			Name: ui.NewLink(
				ent.ID,
				fmt.Sprintf("/p/%s/g/%s/console", ent.ProjectID(), ent.ID),
				fmt.Sprintf("Console %s in project %s", ent.ID, ent.ProjectID()),
			),
		}

		for i, column := range ent.Def.Builders {
			s := &model.BuilderSummary{
				BuilderID: column.Name[0],
				ProjectID: projectID,
			}
			summaries[cid].Builders[i] = s
			for _, rawName := range column.Name {
				name := BuilderID(rawName)
				// Find/populate the BuilderID -> {console: summary}
				colMap, ok := columns[name]
				if !ok {
					colMap = map[common.ConsoleID]*model.BuilderSummary{}
					columns[name] = colMap
				}

				colMap[cid] = s
			}
		}
	}

	// Now grab ALL THE DATA.
	bs := make([]*model.BuilderSummary, 0, len(columns))
	for builderID := range columns {
		bs = append(bs, &model.BuilderSummary{
			BuilderID: string(builderID),
			// TODO: change builder ID format to include project id.
			ProjectID: projectID,
		})
	}
	if err := datastore.Get(c, bs); err != nil {
		me := err.(errors.MultiError)
		lme := errors.NewLazyMultiError(len(me))
		for i, ierr := range me {
			if ierr == datastore.ErrNoSuchEntity {
				logging.Infof(c, "Missing builder: %s", bs[i].BuilderID)
				ierr = nil  // ignore ErrNoSuchEntity
				bs[i] = nil // nil out the BuilderSummary, want to skip this below
			}
			lme.Assign(i, ierr)
		}

		// Return an error only if we encouter an error other than datastore.ErrNoSuchEntity.
		if err := lme.Get(); err != nil {
			return nil, err
		}
	}

	// Now we have the mapping from BuilderID -> summary, and ALL THE DATA, map
	// the data back into the summaries.
	for _, summary := range bs {
		if summary == nil { // We got ErrNoSuchEntity above
			continue
		}

		for cid, curSummary := range columns[BuilderID(summary.BuilderID)] {
			cons := consoles[cid]

			// If this console doesn't show experimental builds, skip all summaries of
			// experimental builds.
			if !cons.IncludeExperimentalBuilds && summary.LastFinishedExperimental {
				continue
			}

			// If the new summary's build was created before the current summary's
			// build, skip it.
			if summary.LastFinishedCreated.Before((*curSummary).LastFinishedCreated) {
				continue
			}

			// Looks like this is the best summary for this slot so far, so save it.
			*curSummary = *summary
		}
	}

	return summaries, nil
}
