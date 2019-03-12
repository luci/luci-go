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
	"net/http"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
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
		"Number of streams created in the last 72-48 hours",
		nil,
		field.String("project"),
		field.String("archival_state"),
	)

	// numStreams24hrs is the number of streams in the last 24 hours.
	numStreams24hrs = metric.NewInt(
		"logdog/stats/log_stream_state_24hrs",
		"Number of streams created in the last 24 hours",
		nil,
		field.String("project"),
		field.String("archival_state"),
	)
)

// getDatastoreNamespaces returns a list of all of the namespaces in the
// datastore.
//
// This is done by issuing a datastore query for kind "__namespace__". The
// resulting keys will have IDs for the namespaces, namely:
//	- The default namespace will have integer ID 1.
//	- Other namespaces will have string IDs.
func getDatastoreNamespaces(c context.Context) ([]string, error) {
	q := datastore.NewQuery("__namespace__").KeysOnly(true)

	// Query our datastore for the full set of namespaces.
	var namespaceKeys []*datastore.Key
	if err := datastore.GetAll(c, q, &namespaceKeys); err != nil {
		return nil, errors.Annotate(err, "enumerating namespaces").Err()
	}

	namespaces := make([]string, 0, len(namespaceKeys))
	for _, nk := range namespaceKeys {
		// Add our namespace ID. For the default namespace, the key will have an
		// integer ID of 1, so StringID will correctly be an empty string.
		namespaces = append(namespaces, nk.StringID())
	}
	return namespaces, nil
}

type queryStat struct {
	start     time.Duration
	end       time.Duration
	metric    metric.Int
	namespace string
}

func doQueryStat(c context.Context, ns string, stat queryStat) error {
	now := clock.Now(c)
	start := now.Add(stat.start)
	end := now.Add(stat.end)
	// Make a projection query, it is cheaper.
	q := datastore.NewQuery("LogStreamState").Gt("Created", start).Lt("Created", end)
	q = q.Project(coordinator.ArchivalStateKey)

	notArchived := int64(0)
	archiveTasked := int64(0)
	archivedPartial := int64(0)
	archivedComplete := int64(0)

	// Gather stats for this namespace.
	if err := datastore.RunBatch(c, 128, q, func(state *datastore.PropertyMap) error {
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
			notArchived++
		case coordinator.ArchiveTasked:
			archiveTasked++
		case coordinator.ArchivedPartial:
			archivedPartial++
		case coordinator.ArchivedComplete:
			archivedComplete++
		default:
			panic("impossible")
		}
		return nil
	}); err != nil {
		logging.WithError(err).Errorf(c, "did not complete datastore query for %s", ns)
		return err
	}

	// Report
	stat.metric.Set(c, notArchived, ns, "not_archived")
	stat.metric.Set(c, archiveTasked, ns, "archive_tasked")
	stat.metric.Set(c, archivedPartial, ns, "archived_partial")
	stat.metric.Set(c, archivedComplete, ns, "archived_complete")
	logging.Infof(c, "Stat %s Project %s stat: NA %d, Tasked %d, Partial %d, Complete %d",
		stat.metric.Info().Name, ns, notArchived, archiveTasked, archivedPartial, archivedComplete)
	return nil
}

// cronStatsHandler gathers metrics about the state of LogDog
// and sends it to tsmon.
//
// This gathers the following stats:
// * Number of unarchived streams between 48-72hr old (after creation).
func cronStatsHandler(ctx *router.Context) {
	namespaces, err := getDatastoreNamespaces(ctx.Context)
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "error while running cron handler")
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	}

	queryStats := []queryStat{
		{-72 * time.Hour, -49 * time.Hour, numStreams72hrs, ""},
		{-24 * time.Hour, 0 * time.Hour, numStreams24hrs, ""},
	}

	if err := parallel.FanOutIn(func(ch chan<- func() error) {
		for _, stat := range queryStats {
			// Do one query per namespace.
			for _, ns := range namespaces {
				c := info.MustNamespace(ctx.Context, ns)
				stat := stat
				ch <- func() error {
					return doQueryStat(c, ns, stat)
				}
			}
		}
	}); err != nil {
		logging.WithError(err).Errorf(ctx.Context, "error while running cron handler")
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	}
}
