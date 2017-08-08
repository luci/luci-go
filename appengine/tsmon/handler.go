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

package tsmon

import (
	"net/http"

	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers installs HTTP handlers for tsmon routes.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET("/internal/cron/ts_mon/housekeeping", base, housekeepingHandler)
}

// housekeepingHandler is an HTTP handler that should be run every minute by
// cron on App Engine.  It assigns task numbers to datastore entries, and runs
// any global metric callbacks.
func housekeepingHandler(c *router.Context) {
	if !info.IsDevAppServer(c.Context) && c.Request.Header.Get("X-Appengine-Cron") != "true" {
		c.Writer.WriteHeader(http.StatusForbidden)
		http.Error(c.Writer, "request not made from cron", http.StatusForbidden)
		return
	}

	if err := assignTaskNumbers(c.Context); err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
	}

	tsmon.GetState(c.Context).RunGlobalCallbacks(c.Context)
}

// assignTaskNumbers does some housekeeping on the datastore entries for App
// Engine instances - assigning unique task numbers to those without ones set,
// and expiring old entities.
func assignTaskNumbers(c context.Context) error {
	c = info.MustNamespace(c, instanceNamespace)

	now := clock.Now(c)
	expiredTime := now.Add(-instanceExpirationTimeout)

	usedTaskNums := map[int]struct{}{}
	totalExpired := 0

	expiredKeys := make([]*ds.Key, 0, ds.Raw(c).Constraints().QueryBatchSize)
	var unassigned []*instance

	// expireInstanceBatch processes the set of instances in "expiredKeys",
	// deletes them, and clears the list for the next iteration.
	//
	// We do this in batches to handle large numbers without inflating memory
	// requirements. If there are any timeouts or problems, this will also enable
	// us to iteratively chip away at the problem.
	expireInstanceBatch := func(c context.Context) error {
		if len(expiredKeys) == 0 {
			return nil
		}

		logging.Debugf(c, "Expiring %d instance(s)", len(expiredKeys))
		if err := ds.Delete(c, expiredKeys); err != nil {
			logging.WithError(err).Errorf(c, "Failed to expire instances.")
			return err
		}

		// Clear the instances list for next round.
		totalExpired += len(expiredKeys)
		expiredKeys = expiredKeys[:0]
		return nil
	}

	// Query all instances from datastore.
	q := ds.NewQuery("Instance")

	b := ds.Batcher{
		Callback: expireInstanceBatch,
	}
	if err := b.Run(c, q, func(i *instance) {
		if i.TaskNum >= 0 {
			usedTaskNums[i.TaskNum] = struct{}{}
		}
		if i.LastUpdated.Before(expiredTime) {
			expiredKeys = append(expiredKeys, ds.NewKey(c, "Instance", i.ID, 0, nil))
			logging.Debugf(c, "Expiring %s task_num %d, inactive since %s",
				i.ID, i.TaskNum, i.LastUpdated.String())
		} else if i.TaskNum < 0 {
			unassigned = append(unassigned, i)
		}
	}); err != nil {
		logging.WithError(err).Errorf(c, "Failed to get Instance entities from datastore")
		return err
	}

	// Final expiration round.
	if err := expireInstanceBatch(c); err != nil {
		logging.WithError(err).Debugf(c, "Failed to expire final instance batch.")
		return err
	}

	logging.Debugf(c, "Found %d expired and %d unassigned instances",
		totalExpired, len(unassigned))

	// Assign task numbers to those that don't have one assigned yet.
	nextNum := gapFinder(usedTaskNums)
	for _, i := range unassigned {
		i.TaskNum = nextNum()
		logging.Debugf(c, "Assigned %s task_num %d", i.ID, i.TaskNum)
	}

	// Update all the entities in datastore.
	if err := ds.Put(c, unassigned); err != nil {
		logging.WithError(err).Errorf(c, "Failed to update task numbers")
		return err
	}
	return nil
}

func gapFinder(used map[int]struct{}) func() int {
	next := 0
	return func() int {
		for {
			n := next
			next++
			_, has := used[n]
			if !has {
				return n
			}
		}
	}
}
