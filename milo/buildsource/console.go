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
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
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

// GetConsolePreview returns a map of builders to their most recent build.
func GetConsolePreview(c context.Context, def *common.Console) (ConsolePreview, error) {
	builds := make([]*model.BuildSummary, len(def.Builders))
	err := parallel.WorkPool(4, func(ch chan<- func() error) {
		for i, b := range def.Builders {
			i := i
			b := b
			ch <- func() error {
				q := datastore.NewQuery("BuildSummary").Eq("BuilderID", b).Order("-Summary.End").Limit(1)
				return datastore.Run(c, q, func(bs *model.BuildSummary) {
					builds[i] = bs
				})
			}
		}
	})
	if err != nil {
		return nil, err
	}
	preview := make(ConsolePreview, len(def.Builders))
	for i := 0; i < len(builds); i++ {
		preview[BuilderID(def.Builders[i])] = builds[i]
	}
	return preview, nil
}

// GetConsoleSummaries returns a list of console summaries from the datastore.
//
// This list of console summaries directly corresponds to the input list of
// console IDs.
func GetConsoleSummaries(c context.Context, consoleIDs []string) ([]resp.ConsoleSummary, error) {
	summaries := make([]resp.ConsoleSummary, len(consoleIDs))
	err := parallel.WorkPool(4, func(ch chan<- func() error) {
		for i, id := range consoleIDs {
			i := i
			id := id
			ch <- func() error {
				q := datastore.NewQuery("BuilderSummary").Eq("Consoles", id)
				return datastore.Run(c, q, func(bs *model.BuilderSummary) {
					// It's safe to do this because we assume the ID has already been validated.
					summaries[i].Name.Label = strings.SplitN(id, "/", 2)[1]
					summaries[i].Name.URL = "/console/" + id
					summaries[i].Builders = append(summaries[i].Builders, bs)
				})
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return summaries, nil
}
