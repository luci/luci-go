// Copyright 2019 The LUCI Authors.
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
package module

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/server/router"
)

var (
	// numStreams72hrs is the number of streams between 49 and 72 hours old,
	// tagged with archival_state.
	numStreams72hrs = metric.NewInt(
		"logdog/stats/log_stream_state_72hrs",
		"Number of streams created in the last 72 hours (2hr window)",
		nil,
		field.String("project"),
		field.String("archival_state"),
	)

	// numStreams24hrs is the number of streams in the last 24 hours.
	numStreams24hrs = metric.NewInt(
		"logdog/stats/log_stream_state_24hrs",
		"Number of streams created 24 hours ago (2hr window)",
		nil,
		field.String("project"),
		field.String("archival_state"),
	)

	totalShards = int64(30)
)

type queryStat struct {
	start  time.Duration
	end    time.Duration
	metric metric.Int
}

var metrics = map[string]queryStat{
	"24hrs": {-24 * time.Hour, -22 * time.Hour, numStreams24hrs},
	"72hrs": {-72 * time.Hour, -70 * time.Hour, numStreams72hrs},
}

// getProjectNamespaces returns a list of all of the namespaces in the
// datastore that begin with "luci.".
//
// This is done by issuing a datastore query for kind "__namespace__". The
// resulting keys will have IDs for the namespaces, namely:
//	- The default namespace will have integer ID 1.
//	- Other namespaces will have string IDs.
func getProjectNamespaces(c context.Context) ([]string, error) {
	q := datastore.NewQuery("__namespace__").KeysOnly(true)

	// Query our datastore for the full set of namespaces.
	var namespaceKeys []*datastore.Key
	if err := datastore.GetAll(c, q, &namespaceKeys); err != nil {
		return nil, errors.Annotate(err, "enumerating namespaces").Err()
	}

	nmap := make(map[string]bool)
	namespaces := make([]string, 0, len(namespaceKeys))
	for _, nk := range namespaceKeys {
		if !strings.HasPrefix(nk.StringID(), coordinator.ProjectNamespacePrefix) {
			continue
		}
		nmap[nk.StringID()] = true
		namespaces = append(namespaces, nk.StringID())
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

// queryResult is the result of a batch of stream state query.
type queryResult struct {
	lock             sync.Mutex
	notArchived      int64
	archiveTasked    int64
	archivedPartial  int64
	archivedComplete int64
}

// add adds the contents of s into r.  This is goroutine safe.
func (r *queryResult) add(s *queryResult) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.notArchived += s.notArchived
	r.archiveTasked += s.archiveTasked
	r.archivedPartial += s.archivedPartial
	r.archivedComplete += s.archivedComplete
}

func (r *queryResult) String() string {
	return fmt.Sprintf("NA: %d, Tasked: %d, Partial: %d, Complete: %d", r.notArchived, r.archiveTasked, r.archivedPartial, r.archivedComplete)
}

// doShardedQueryStat launches a batch of queries, at different shard indices.
func doShardedQueryStat(c context.Context, ns string, stat queryStat) error {
	results := &queryResult{}
	if err := parallel.FanOutIn(func(ch chan<- func() error) {
		for i := int64(0); i < totalShards; i++ {
			i := i
			ch <- func() error {
				result, err := doQueryStat(c, ns, stat, i)
				if err != nil {
					return errors.Annotate(err, "while launching index %d: %s", i, result).Err()
				}
				logging.Infof(c, "For index %d got %s", i, result)
				results.add(result)
				return nil
			}
		}
	}); err != nil {
		return err
	}

	// Report
	stat.metric.Set(c, results.notArchived, ns, "not_archived")
	stat.metric.Set(c, results.archiveTasked, ns, "archive_tasked")
	stat.metric.Set(c, results.archivedPartial, ns, "archived_partial")
	stat.metric.Set(c, results.archivedComplete, ns, "archived_complete")
	logging.Infof(c, "Stat %s Project %s stat: %s", stat.metric.Info().Name, ns, results)
	return nil
}

// doQueryStat runs a single query containing a time range, with a certain index.
func doQueryStat(c context.Context, ns string, stat queryStat, index int64) (*queryResult, error) {
	// We shard a large query into smaller time blocks.
	shardSize := time.Duration((int64(stat.end) - int64(stat.start)) / totalShards)
	startOffset := time.Duration(int64(shardSize) * index)
	now := clock.Now(c)
	start := now.Add(stat.start).Add(startOffset)
	end := start.Add(shardSize)

	// Gather stats for this namespace.
	nc, cancel := context.WithTimeout(c, 5*time.Minute)
	defer cancel()
	// Make a projection query, it is cheaper.
	q := datastore.NewQuery("LogStreamState").Gte("Created", start).Lt("Created", end)
	q = q.Project(coordinator.ArchivalStateKey)
	logging.Debugf(c, "Running query for %s (%s) at index %d: %v", ns, stat.metric.Info().Name, index, q)

	result := &queryResult{}
	return result, datastore.RunBatch(nc, 512, q, func(state *datastore.PropertyMap) error {
		asRaws := state.Slice(coordinator.ArchivalStateKey)
		if len(asRaws) != 1 {
			logging.Errorf(c, "%v for %v has the wrong size", asRaws, state)
		}
		asRawProp := asRaws[0]
		asRawInt, ok := asRawProp.Value().(int64)
		if !ok {
			logging.Errorf(c, "%v and %v are not archival states (ints)", asRawProp, asRawProp.Value())
			return datastore.Stop
		}
		as := coordinator.ArchivalState(asRawInt)

		switch as {
		case coordinator.NotArchived:
			result.notArchived++
		case coordinator.ArchiveTasked:
			result.archiveTasked++
		case coordinator.ArchivedPartial:
			result.archivedPartial++
		case coordinator.ArchivedComplete:
			result.archivedComplete++
		default:
			panic("impossible")
		}
		return nil
	})
}

// cronStatsNSHandler gathers metrics about a metric and namespace within logdog.
func cronStatsNSHandler(ctx *router.Context) {
	s := ctx.Params.ByName("stat")
	qs, ok := metrics[s]
	if !ok {
		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}
	ns := ctx.Params.ByName("namespace")
	c := info.MustNamespace(ctx.Context, ns)
	err := doShardedQueryStat(c, ns, qs)
	if err != nil {
		errors.Log(c, err)
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	}
}

// cronStatsHandler gathers metrics about the state of LogDog and sends it to tsmon.
// This collects all namespaces, and fires off one task per namespace x metric combination.
//
// This gathers the following stats:
// * Number of unarchived streams between 70-72hr old (after creation).
// * Number of unarchived streams between 22-24hr old (after creation).
func cronStatsHandler(ctx *router.Context) {
	namespaces, err := getProjectNamespaces(ctx.Context)
	if err != nil {
		errors.Log(ctx.Context, err)
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	for s := range metrics {
		for _, ns := range namespaces {
			u := fmt.Sprintf("/admin/cron/stats/%s/%s", s, ns)
			t := taskqueue.NewPOSTTask(u, url.Values{})
			taskqueue.Add(ctx.Context, "default", t)
		}
	}
}
