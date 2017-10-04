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
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
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
	result := make([]string, 0, len(text))
	merge := false
	for _, line := range text {
		if merge {
			merge = false
			result[len(result)-1] += " " + line
			continue
		}
		result = append(result, line)
		switch line {
		case "build", "failed", "exception":
			merge = true
		default:
			merge = false
		}
	}

	// We can remove error messages about the step "steps" if it's part of a longer
	// message because this step is an artifact of running on recipes and it's
	// not important to users.
	if len(result) > 1 {
		switch result[0] {
		case "failed steps", "exception steps":
			result = result[1:]
		}
	}
	return result
}

func getBuildSummary(b *buildbot.Build) *resp.BuildSummary {
	return &resp.BuildSummary{
		Link:   resp.NewLink(fmt.Sprintf("#%d", b.Number), fmt.Sprintf("%d", b.Number)),
		Status: b.Status(),
		ExecutionTime: resp.Interval{
			Started:  b.Times.Start.Time,
			Finished: b.Times.Finish.Time,
			Duration: b.Times.Duration(),
		},
		Text:     mergeText(b.Text),
		Blame:    blame(b),
		Revision: b.Sourcestamp.Revision,
	}
}

func summarizeSlavePool(
	baseURL string, slaves []string, slaveMap map[string]*buildbot.Slave) *resp.MachinePool {

	mp := &resp.MachinePool{
		Total: len(slaves),
		Bots:  make([]resp.Bot, 0, len(slaves)),
	}
	for _, slaveName := range slaves {
		slave, ok := slaveMap[slaveName]
		bot := resp.Bot{
			Name: *resp.NewLink(
				slaveName,
				fmt.Sprintf("%s/buildslaves/%s", baseURL, slaveName),
			),
		}
		switch {
		case !ok:
			// This shouldn't happen
		case !slave.Connected:
			bot.Status = resp.Disconnected
			mp.Disconnected++
		case len(slave.RunningbuildsMap) > 0:
			bot.Status = resp.Busy
			mp.Busy++
		default:
			bot.Status = resp.Idle
			mp.Idle++
		}
		mp.Bots = append(mp.Bots, bot)
	}
	return mp
}

// GetBuilder is the implementation for getting a milo builder page from
// buildbot.
func GetBuilder(c context.Context, masterName, builderName string, limit int, cursor string) (*resp.Builder, error) {
	if err := buildstore.CanAccessMaster(c, masterName); err != nil {
		return nil, err
	}
	result := &resp.Builder{
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
	result.PendingBuilds = make([]*resp.BuildSummary, len(builder.PendingBuildStates))
	result.PendingBuildNum = builder.PendingBuilds
	logging.Debugf(c, "Number of pending builds: %d", len(builder.PendingBuildStates))
	for i, pb := range builder.PendingBuildStates {
		start := time.Unix(int64(pb.SubmittedAt), 0).UTC()
		result.PendingBuilds[i] = &resp.BuildSummary{
			PendingTime: resp.Interval{
				Started:  start,
				Duration: clock.Now(c).UTC().Sub(start),
			},
			Blame: make([]*resp.Commit, len(pb.Source.Changes)),
		}
		for j, cm := range pb.Source.Changes {
			result.PendingBuilds[i].Blame[j] = &resp.Commit{
				AuthorEmail: cm.Who,
				CommitURL:   cm.Revlink,
			}
		}
	}

	baseURL := "https://build.chromium.org/p/"
	if master.Internal {
		baseURL = "https://uberchromegw.corp.google.com/i/"
	}
	result.MachinePool = summarizeSlavePool(baseURL+master.Name, builder.Slaves, master.Slaves)

	return result, parallel.FanOutIn(func(work chan<- func() error) {
		q := buildstore.Query{
			Master:  masterName,
			Builder: builderName,
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
			result.FinishedBuilds = make([]*resp.BuildSummary, len(res.Builds))
			for i, b := range res.Builds {
				result.FinishedBuilds[i] = getBuildSummary(b)
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
			result.CurrentBuilds = make([]*resp.BuildSummary, len(res.Builds))
			for i, b := range res.Builds {
				// currentBuilds is presented in reversed order, so flip it
				result.CurrentBuilds[len(res.Builds)-i-1] = getBuildSummary(b)
			}
			return err
		}
	})
}

// GetAllBuilders returns a resp.Module object containing all known masters
// and builders.
func GetAllBuilders(c context.Context) (*resp.CIService, error) {
	result := &resp.CIService{Name: "Buildbot"}
	masters, err := buildstore.AllMasters(c, true)
	if err != nil {
		return nil, err
	}

	// Add each builder from each master m into the result.
	for _, m := range masters {
		ml := resp.BuilderGroup{Name: m.Name}
		// Sort the builder listing.
		builders := make([]string, 0, len(m.Builders))
		for b := range m.Builders {
			builders = append(builders, b)
		}
		sort.Strings(builders)
		for _, b := range builders {
			// Go templates escapes this for us, and also
			// slashes are not allowed in builder names.
			ml.Builders = append(ml.Builders, *resp.NewLink(
				b, fmt.Sprintf("/buildbot/%s/%s", m.Name, b)))
		}
		result.BuilderGroups = append(result.BuilderGroups, ml)
	}
	return result, nil
}
