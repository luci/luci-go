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
	"context"
	"encoding/hex"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
)

// ConsoleRow is one row of a particular console.
//
// It has the git commit for the row, as well as a mapping of column index to
// the Builds associated with it for this commit. The columns are defined by the
// order of the Builder messages in the Console config message (one column per
// Builder message).
//
// Builds is a map since most commit rows have a small subset of the available
// builders.
type ConsoleRow struct {
	Commit string
	Builds map[int][]*model.BuildSummary
}

// GetConsoleRows returns a row-oriented collection of BuildSummary
// objects. Each row corresponds to the similarly-indexed commit in the
// `commits` slice.
func GetConsoleRows(c context.Context, project string, console *projectconfigpb.Console, commits []string) ([]*ConsoleRow, error) {
	rawCommits := make([][]byte, len(commits))
	for i, c := range commits {
		var err error
		if rawCommits[i], err = hex.DecodeString(c); err != nil {
			return nil, errors.Fmt("bad commit[%d]: %q: %w", i, c, err)
		}
	}

	// Maps all builderIDs to the indexes of the columns it appears in.
	columnMap := map[string][]int{}
	for columnIdx, b := range console.Builders {
		bidString := utils.LegacyBuilderIDString(b.Id)
		columnMap[bidString] = append(columnMap[bidString], columnIdx)
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
			r := &ConsoleRow{Commit: commits[i]}
			ret[i] = r
			ch <- func() error {
				fullQ := q.Eq("ManifestKeys", partialKey.AddRevision(rawCommits[i]))
				return datastore.Run(c, fullQ, func(bs *model.BuildSummary) {
					if bs.Experimental && !console.IncludeExperimentalBuilds {
						return
					}
					if columnIdxs, ok := columnMap[bs.BuilderID]; ok {
						if r.Builds == nil {
							r.Builds = map[int][]*model.BuildSummary{}
						}
						for _, columnIdx := range columnIdxs {
							r.Builds[columnIdx] = append(r.Builds[columnIdx], bs)
						}
					}
				})
			}
		}
	})

	return ret, err
}

// GetConsoleSummariesFromDefs returns a map of consoleID -> summary from the
// datastore using a slice of console definitions as input.
//
// This expects all builders in all consoles coming from the same projectID.
func GetConsoleSummariesFromDefs(c context.Context, consoleEnts []*projectconfig.Console, projectID string) (
	map[projectconfig.ConsoleID]*ui.BuilderSummaryGroup, error) {

	// Maps consoleID -> console config definition.
	consoles := make(map[projectconfig.ConsoleID]*projectconfigpb.Console, len(consoleEnts))

	// Maps the BuilderID to the per-console pointer-to-summary in the summaries
	// map. Note that builders with multiple builderIDs in the same console will
	// all map to the same BuilderSummary.
	columns := map[BuilderID]map[projectconfig.ConsoleID][]*model.BuilderSummary{}

	// The return result.
	summaries := map[projectconfig.ConsoleID]*ui.BuilderSummaryGroup{}

	for _, ent := range consoleEnts {
		cid := ent.ConsoleID()
		consoles[cid] = ent.Def

		summaries[cid] = &ui.BuilderSummaryGroup{
			Builders: make([]*model.BuilderSummary, len(ent.Def.Builders)),
			Name: ui.NewLink(
				ent.ID,
				fmt.Sprintf("/p/%s/g/%s/console", ent.ProjectID(), ent.ID),
				fmt.Sprintf("Console %s in project %s", ent.ID, ent.ProjectID()),
			),
		}

		for i, column := range ent.Def.Builders {
			s := &model.BuilderSummary{
				BuilderID: utils.LegacyBuilderIDString(column.Id),
				ProjectID: projectID,
			}
			summaries[cid].Builders[i] = s
			name := BuilderID(utils.LegacyBuilderIDString(column.Id))
			// Find/populate the BuilderID -> {console: summary}
			colMap, ok := columns[name]
			if !ok {
				colMap = map[projectconfig.ConsoleID][]*model.BuilderSummary{}
				columns[name] = colMap
			}

			colMap[cid] = append(colMap[cid], s)
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

		// Return an error only if we encounter an error other than datastore.ErrNoSuchEntity.
		if err := lme.Get(); err != nil {
			return nil, err
		}
	}

	// Now we have the mapping from BuilderID -> summaries, and ALL THE DATA, map
	// the data back into the summaries.
	for _, summary := range bs {
		if summary == nil { // We got ErrNoSuchEntity above
			continue
		}

		for cid, curSummaries := range columns[BuilderID(summary.BuilderID)] {
			cons := consoles[cid]

			// If this console doesn't show experimental builds, skip all summaries of
			// experimental builds.
			if !cons.IncludeExperimentalBuilds && summary.LastFinishedExperimental {
				continue
			}

			for _, curSummary := range curSummaries {
				// If the new summary's build was created before the current summary's
				// build, skip it.
				if summary.LastFinishedCreated.Before((*curSummary).LastFinishedCreated) {
					continue
				}

				// Looks like this is the best summary for this slot so far, so save it.
				*curSummary = *summary
			}
		}
	}

	return summaries, nil
}
