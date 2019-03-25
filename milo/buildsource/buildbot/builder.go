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
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// builderRef is used for keying specific builds in a master json.
type builderRef struct {
	builder  string
	buildNum int
}

// buildMap contains all of the current build within a master json.  We use this
// because buildbot returns all current builds as within the slaves portion, whereas
// it's eaiser to map thenm by builders instead.
type buildMap map[builderRef]*buildbot.Build

// mergeText merges buildbot summary texts, which sometimes separates
// words that should be merged together, this combines them into a single
// line.
func mergeText(text []string) []string {
	merged := make([]string, 0, len(text))
	merge := false
	for _, line := range text {
		if merge {
			merge = false
			merged[len(merged)-1] += " " + line
			continue
		}
		merged = append(merged, line)
		switch line {
		case "build", "failed", "exception":
			merge = true
		default:
			merge = false
		}
	}

	// Filter out special cased statuses to make the presentation look cleaner.
	result := make([]string, 0, len(merged))
	for _, item := range merged {
		switch item {
		case "failed steps", "failed Failure reason":
			// Ignore because they're recipe artifacts.
		default:
			result = append(result, item)
		}
	}
	return result
}

func summarizeSlavePool(
	c context.Context, baseURL string, slaves []string, slaveMap map[string]*buildbot.Slave) *ui.MachinePool {

	bots := make([]model.Bot, len(slaves))
	for i, slaveName := range slaves {
		bot := model.Bot{
			Name: slaveName,
			URL:  fmt.Sprintf("%s/buildslaves/%s", baseURL, slaveName),
		}

		slave, ok := slaveMap[slaveName]
		switch {
		case !ok:
			// This shouldn't happen.  Just log an error and count it as offline.
			logging.Errorf(c, "Slave %s not found", slaveName)
			fallthrough
		case !slave.Connected:
			bot.Status = model.Offline
		case len(slave.RunningbuildsMap) > 0:
			bot.Status = model.Busy
		default:
			bot.Status = model.Idle
		}
		bots[i] = bot
	}
	return ui.NewMachinePool(c, &model.BotPool{Bots: bots})
}

// GetBuilder is the implementation for getting a milo builder page from
// buildbot.
func GetBuilder(c context.Context, masterName, builderName string, limit int, cursor string) (*ui.Builder, error) {
	if err := buildstore.CanAccessMaster(c, masterName); err != nil {
		return nil, err
	}
	result := &ui.Builder{
		Name: builderName,
	}
	master, err := buildstore.GetMaster(c, masterName, false)
	if err != nil {
		return nil, err
	}
	if clock.Now(c).Sub(master.Modified) > 2*time.Minute {
		warning := fmt.Sprintf(
			"WARNING: buildbotMasterEntry data is stale (last updated %s)", master.Modified)
		logging.Warningf(c, warning)
		result.Warning = warning
	}

	builder, ok := master.Builders[builderName]
	if !ok {
		// This long block is just to return a good error message when an invalid
		// buildbot builder is specified.
		keys := make([]string, 0, len(master.Builders))
		for k := range master.Builders {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// TODO(iannucci): add error-info-helper tags to give the error page enough
		// information to render link-to-master and link-to-builder.
		builders := strings.Join(keys, "\n")
		return nil, errors.Reason(
			"Cannot find builder %q in master %q.\nAvailable builders: \n%s",
			builderName, masterName, builders,
		).Tag(common.CodeNotFound).Err()
	}

	// Extract pending builds out of the master.
	result.PendingBuilds = make([]*ui.BuildSummary, len(builder.PendingBuildStates))
	result.PendingBuildNum = builder.PendingBuilds
	logging.Debugf(c, "Number of pending builds: %d", len(builder.PendingBuildStates))
	for i, pb := range builder.PendingBuildStates {
		start := time.Unix(int64(pb.SubmittedAt), 0).UTC()
		result.PendingBuilds[i] = &ui.BuildSummary{
			PendingTime: ui.NewInterval(c, start, time.Time{}),
			Blame:       make([]*ui.Commit, len(pb.Source.Changes)),
		}
		for j, cm := range pb.Source.Changes {
			result.PendingBuilds[i].Blame[j] = &ui.Commit{
				AuthorEmail: cm.Who,
				CommitURL:   cm.Revlink,
			}
		}
	}

	baseURL := "https://build.chromium.org/p/"
	if master.Internal {
		baseURL = "https://uberchromegw.corp.google.com/i/"
	}
	result.MachinePool = summarizeSlavePool(c, baseURL+master.Name, builder.Slaves, master.Slaves)

	return result, parallel.FanOutIn(func(work chan<- func() error) {
		q := buildstore.Query{
			Master:  masterName,
			Builder: builderName,

			NoAnnotationFetch: true,
		}
		work <- func() error {
			q := q
			q.Limit = limit
			q.Cursor = cursor
			q.Finished = buildstore.Yes
			res, err := buildstore.GetBuilds(c, q)
			if err != nil {
				return err
			}
			result.NextCursor = res.NextCursor
			result.PrevCursor = res.PrevCursor
			result.FinishedBuilds = make([]*ui.BuildSummary, len(res.Builds))
			for i, b := range res.Builds {
				result.FinishedBuilds[i] = renderBuild(c, b, false).BuildSummary()
			}
			return err
		}
		work <- func() error {
			q := q
			q.Finished = buildstore.No
			res, err := buildstore.GetBuilds(c, q)
			if err != nil {
				return err
			}
			result.CurrentBuilds = make([]*ui.BuildSummary, len(res.Builds))
			for i, b := range res.Builds {
				// currentBuilds is presented in reversed order, so flip it
				result.CurrentBuilds[len(res.Builds)-i-1] = renderBuild(c, b, false).BuildSummary()
			}
			return err
		}
	})
}

// CIService returns a *ui.CIService containing all known masters and builders.
func CIService(c context.Context) (*ui.CIService, error) {
	result := &ui.CIService{Name: "Buildbot"}
	masters, err := buildstore.AllMasters(c, true)
	if err != nil {
		return nil, err
	}

	// Add each builder from each master m into the result.
	for _, m := range masters {
		ml := ui.BuilderGroup{Name: m.Name}
		// Sort the builder listing.
		builders := make([]string, 0, len(m.Builders))
		for b := range m.Builders {
			builders = append(builders, b)
		}
		sort.Strings(builders)
		for _, b := range builders {
			// Go templates escapes this for us, and also
			// slashes are not allowed in builder names.
			ml.Builders = append(ml.Builders, *ui.NewLink(
				b, fmt.Sprintf("/buildbot/%s/%s", m.Name, b),
				fmt.Sprintf("buildbot builder %s on master %s", b, m.Name)))
		}
		result.BuilderGroups = append(result.BuilderGroups, ml)
	}
	return result, nil
}
