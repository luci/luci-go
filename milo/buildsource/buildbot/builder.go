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
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/api/resp"
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
	started, finished, duration := parseTimes(nil, b.Times)
	return &resp.BuildSummary{
		Link:   resp.NewLink(fmt.Sprintf("#%d", b.Number), fmt.Sprintf("%d", b.Number)),
		Status: b.Status(),
		ExecutionTime: resp.Interval{
			Started:  started,
			Finished: finished,
			Duration: duration,
		},
		Text:     mergeText(b.Text),
		Blame:    blame(b),
		Revision: b.Sourcestamp.Revision,
	}
}

// getBuilds fetches all of the recent builds from the .  Note that
// getBuilds() does not perform ACL checks.
func getBuilds(
	c context.Context, masterName, builderName string, finished bool, limit int, cursor datastore.Cursor) (
	[]*resp.BuildSummary, datastore.Cursor, error) {

	// TODO(hinoka): Builder specific structs.
	result := []*resp.BuildSummary{}
	q := datastore.NewQuery("buildbotBuild")
	q = q.Eq("finished", finished)
	q = q.Eq("master", masterName)
	q = q.Eq("builder", builderName)
	q = q.Order("-number")
	if cursor != nil {
		q = q.Start(cursor)
	}
	q = q.Limit(int32(limit))
	buildbots, nextCursor, err := runBuildsQuery(c, q)
	if err != nil {
		return nil, nil, err
	}
	for _, b := range buildbots {
		result = append(result, getBuildSummary(b))
	}
	return result, nextCursor, nil
}

// maybeSetGetCursor is a cheesy way to implement bidirectional paging with forward-only
// datastore cursor by creating a mapping of nextCursor -> thisCursor
// in memcache.  maybeSetGetCursor stores the future mapping, then returns prevCursor
// in the mapping for thisCursor -> prevCursor, if available.
func maybeSetGetCursor(c context.Context, thisCursor, nextCursor datastore.Cursor, limit int) (datastore.Cursor, bool) {
	key := func(c datastore.Cursor) string {
		// Memcache key limit is 250 bytes, hash our cursor to get under this limit.
		blob := sha1.Sum([]byte(c.String()))
		return fmt.Sprintf("v2:cursors:buildbot_builders:%d:%s", limit, base64.StdEncoding.EncodeToString(blob[:]))
	}
	// Set the next cursor to this cursor mapping, if available.
	if nextCursor != nil {
		item := memcache.NewItem(c, key(nextCursor))
		if thisCursor == nil {
			// Make sure we know it exists, just empty
			item.SetValue([]byte{})
		} else {
			item.SetValue([]byte(thisCursor.String()))
		}
		item.SetExpiration(24 * time.Hour)
		memcache.Set(c, item)
	}
	// Try to get the last cursor, if valid and available.
	if thisCursor == nil {
		return nil, false
	}
	if item, err := memcache.GetKey(c, key(thisCursor)); err == nil {
		if len(item.Value()) == 0 {
			return nil, true
		}
		if prevCursor, err := datastore.DecodeCursor(c, string(item.Value())); err == nil {
			return prevCursor, true
		}
	}
	return nil, false
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
//
// This gets:
//   * Current Builds from querying the master json from the datastore.
//   * Recent Builds from a cron job that backfills the recent builds.
func GetBuilder(c context.Context, masterName, builderName string, limit int, cursor datastore.Cursor) (*resp.Builder, error) {
	result := &resp.Builder{
		Name: builderName,
	}
	master, internal, t, err := getMasterJSON(c, masterName)
	if err != nil {
		return nil, err
	}
	if clock.Now(c).Sub(t) > 2*time.Minute {
		warning := fmt.Sprintf(
			"WARNING: Master data is stale (last updated %s)", t)
		logging.Warningf(c, warning)
		result.Warning = warning
	}

	p, ok := master.Builders[builderName]
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
	// Extract pending builds out of the master json.
	result.PendingBuilds = make([]*resp.BuildSummary, len(p.PendingBuildStates))
	result.PendingBuildNum = p.PendingBuilds
	logging.Debugf(c, "Number of pending builds: %d", len(p.PendingBuildStates))
	for i, pb := range p.PendingBuildStates {
		start := time.Unix(int64(pb.SubmittedAt), 0).UTC()
		result.PendingBuilds[i] = &resp.BuildSummary{
			PendingTime: resp.Interval{
				Started:  start,
				Duration: clock.Now(c).UTC().Sub(start),
			},
		}
		result.PendingBuilds[i].Blame = make([]*resp.Commit, len(pb.Source.Changes))
		for j, cm := range pb.Source.Changes {
			result.PendingBuilds[i].Blame[j] = &resp.Commit{
				AuthorEmail: cm.Who,
				CommitURL:   cm.Revlink,
			}
		}
	}

	baseURL := "https://build.chromium.org/p/"
	if internal {
		baseURL = "https://uberchromegw.corp.google.com/i/"
	}
	result.MachinePool = summarizeSlavePool(baseURL+master.Name, p.Slaves, master.Slaves)

	// This is CPU bound anyways, so there's no need to do this in parallel.
	finishedBuilds, nextCursor, err := getBuilds(c, masterName, builderName, true, limit, cursor)
	if err != nil {
		return nil, err
	}
	if prevCursor, ok := maybeSetGetCursor(c, cursor, nextCursor, limit); ok {
		if prevCursor == nil {
			// Magic string to signal display prev without cursor
			result.PrevCursor = "EMPTY"
		} else {
			result.PrevCursor = prevCursor.String()
		}
	}
	if nextCursor != nil {
		result.NextCursor = nextCursor.String()
	}
	// Cursor is not needed for current builds.
	currentBuilds, _, err := getBuilds(c, masterName, builderName, false, 0, nil)
	if err != nil {
		return nil, err
	}
	// currentBuilds is presented in reversed order, so flip it
	for i, j := 0, len(currentBuilds)-1; i < j; i, j = i+1, j-1 {
		currentBuilds[i], currentBuilds[j] = currentBuilds[j], currentBuilds[i]
	}
	result.CurrentBuilds = currentBuilds

	for _, fb := range finishedBuilds {
		if fb != nil {
			result.FinishedBuilds = append(result.FinishedBuilds, fb)
		}
	}
	return result, nil
}
