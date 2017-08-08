// Copyright 2015 The LUCI Authors.
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

package tumble

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"golang.org/x/net/context"
)

const (
	baseURL             = "/internal/" + baseName
	fireAllTasksURL     = baseURL + "/fire_all_tasks"
	processShardPattern = baseURL + "/process_shard/:shard_id/at/:timestamp"

	transientHTTPHeader = "X-LUCI-Tumble-Transient"
)

// Service is an instance of a Tumble service. It installs its handlers into an
// HTTP router and services Tumble request tasks.
type Service struct {
	// Namespaces is a function that returns the datastore namespaces that Tumble
	// will poll.
	//
	// If nil, Tumble will be executed against all namespaces registered in the
	// datastore.
	Namespaces func(context.Context) ([]string, error)
}

// InstallHandlers installs http handlers.
//
// 'base' is usually gaemiddleware.BaseProd(), but can also be its derivative
// if something else it needed in the context.
func (s *Service) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	// GET so that this can be invoked from cron
	r.GET(fireAllTasksURL, base.Extend(gaemiddleware.RequireCron), s.FireAllTasksHandler)
	r.POST(processShardPattern, base.Extend(gaemiddleware.RequireTaskQueue(baseName)),
		func(ctx *router.Context) {
			loop := ctx.Request.URL.Query().Get("single") == ""
			s.ProcessShardHandler(ctx, loop)
		})
}

// FireAllTasksHandler is an HTTP handler that expects `logging` and `luci/gae`
// services to be installed into the context.
//
// FireAllTasksHandler verifies that it was called within an Appengine Cron
// request, and then invokes the FireAllTasks function.
func (s *Service) FireAllTasksHandler(c *router.Context) {
	if err := s.FireAllTasks(c.Context); err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(c.Writer, "fire_all_tasks failed: %s", err)
	} else {
		c.Writer.Write([]byte("ok"))
	}
}

// FireAllTasks searches for work in all namespaces, and fires off a process
// task for any shards it finds that have at least one Mutation present to
// ensure that no work languishes forever. This may not be needed in
// a constantly-loaded system with good tumble key distribution.
func (s *Service) FireAllTasks(c context.Context) error {
	cfg := getConfig(c)

	// Generate a list of all shards.
	allShards := make([]taskShard, 0, cfg.NumShards)
	for i := uint64(0); i < cfg.NumShards; i++ {
		allShards = append(allShards, taskShard{i, minTS})
	}

	namespaces, err := s.getNamespaces(c, cfg)
	if err != nil {
		return err
	}

	// Probe each namespace in parallel. Each probe function reports its own
	// errors, so the work pool will never return any non-nil error response.
	var errCount, taskCount counter
	_ = parallel.WorkPool(cfg.NumGoroutines, func(ch chan<- func() error) {
		for _, ns := range namespaces {
			ns := ns
			ch <- func() error {
				s.fireAllTasksForNamespace(c, cfg, ns, allShards, &errCount, &taskCount)
				return nil
			}
		}
	})
	if errCount > 0 {
		logging.Errorf(c, "Encountered %d error(s).", errCount)
		return errors.New("errors were encountered while probing for tasks")
	}

	logging.Debugf(c, "Successfully probed %d namespace(s) and fired %d tasks(s).",
		len(namespaces), taskCount)
	return err
}

func (s *Service) fireAllTasksForNamespace(c context.Context, cfg *Config, ns string, allShards []taskShard,
	errCount, taskCount *counter) {

	// Enter the supplied namespace.
	logging.Infof(c, "Firing all tasks for namespace %q", ns)
	c = info.MustNamespace(c, ns)
	if ns != "" {
		c = logging.SetField(c, "namespace", ns)
	}

	// First, check if the namespace has *any* Mutations.
	q := ds.NewQuery("tumble.Mutation").KeysOnly(true).Limit(1)
	switch amt, err := ds.Count(c, q); {
	case err != nil:
		logging.WithError(err).Errorf(c, "Error querying for Mutations")
		errCount.inc()
		return

	case amt == 0:
		logging.Infof(c, "No Mutations registered for this namespace.")
		return
	}

	// We have at least one Mutation for this namespace. Iterate through all
	// shards and dispatch a processing task for each one that has Mutations.
	//
	// Track shards that we find work for. After scanning is complete, fire off
	// tasks for all identified shards.
	triggerShards := make(map[taskShard]struct{}, len(allShards))
	for _, shrd := range allShards {
		amt, err := ds.Count(c, processShardQuery(c, cfg, shrd.shard).Limit(1))
		if err != nil {
			logging.Fields{
				logging.ErrorKey: err,
				"shard":          shrd.shard,
			}.Errorf(c, "Error querying for shards")
			errCount.inc()
			break
		}
		if amt > 0 {
			logging.Infof(c, "Found work in shard [%d]", shrd.shard)
			triggerShards[shrd] = struct{}{}
		}
	}

	// Fire tasks for shards with identified work.
	if len(triggerShards) > 0 {
		logging.Infof(c, "Firing tasks for %d tasked shard(s).", len(triggerShards))
		if !fireTasks(c, cfg, triggerShards, false) {
			logging.Errorf(c, "Failed to fire tasks.")
			errCount.inc()
		} else {
			taskCount.add(len(triggerShards))
		}
	} else {
		logging.Infof(c, "No tasked shards were found.")
	}
}

func (s *Service) getNamespaces(c context.Context, cfg *Config) ([]string, error) {
	// Get the set of namespaces to handle.
	nsFn := s.Namespaces
	if nsFn == nil {
		nsFn = getDatastoreNamespaces
	}

	namespaces, err := nsFn(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to enumerate namespaces.")
		return nil, err
	}
	return namespaces, nil
}

// ProcessShardHandler is an HTTP handler that expects `logging` and `luci/gae`
// services to be installed into the context.
//
// ProcessShardHandler verifies that its being run as a taskqueue task and that
// the following parameters exist and are well-formed:
//   * timestamp: decimal-encoded UNIX/UTC timestamp in seconds.
//   * shard_id: decimal-encoded shard identifier.
//
// ProcessShardHandler then invokes ProcessShard with the parsed parameters. It
// runs in the namespace of the task which scheduled it and processes mutations
// for that namespace.
func (s *Service) ProcessShardHandler(ctx *router.Context, loop bool) {
	c, rw, p := ctx.Context, ctx.Writer, ctx.Params

	tstampStr := p.ByName("timestamp")
	sidStr := p.ByName("shard_id")

	tstamp, err := strconv.ParseInt(tstampStr, 10, 64)
	if err != nil {
		logging.Errorf(c, "bad timestamp %q", tstampStr)
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "bad timestamp")
		return
	}

	sid, err := strconv.ParseUint(sidStr, 10, 64)
	if err != nil {
		logging.Errorf(c, "bad shardID %q", tstampStr)
		rw.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(rw, "bad shardID")
		return
	}

	cfg := getConfig(c)

	logging.Infof(c, "Processing tasks in namespace %q", info.GetNamespace(c))
	err = processShard(c, cfg, time.Unix(tstamp, 0).UTC(), sid, loop)
	if err != nil {
		logging.Errorf(c, "failure! %s", err)

		if transient.Tag.In(err) {
			rw.Header().Add(transientHTTPHeader, "true")
		}
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "error: %s", err)
	} else {
		rw.Write([]byte("ok"))
	}
}

// getDatastoreNamespaces returns a list of all of the namespaces in the
// datastore.
//
// This is done by issuing a datastore query for kind "__namespace__". The
// resulting keys will have IDs for the namespaces, namely:
//	- The default namespace will have integer ID 1.
//	- Other namespaces will have string IDs.
func getDatastoreNamespaces(c context.Context) ([]string, error) {
	q := ds.NewQuery("__namespace__").KeysOnly(true)

	// Query our datastore for the full set of namespaces.
	var namespaceKeys []*ds.Key
	if err := ds.GetAll(c, q, &namespaceKeys); err != nil {
		logging.WithError(err).Errorf(c, "Failed to execute namespace query.")
		return nil, err
	}

	namespaces := make([]string, 0, len(namespaceKeys))
	for _, nk := range namespaceKeys {
		// Add our namespace ID. For the default namespace, the key will have an
		// integer ID of 1, so StringID will correctly be an empty string.
		namespaces = append(namespaces, nk.StringID())
	}
	return namespaces, nil
}

// processURL creates a new url for a process shard taskqueue task, including
// the given timestamp and shard number.
func processURL(ts timestamp, shard uint64, ns string, loop bool) string {
	v := strings.NewReplacer(
		":shard_id", fmt.Sprint(shard),
		":timestamp", strconv.FormatInt(int64(ts), 10),
	).Replace(processShardPattern)

	// Append our namespace query parameter. This is cosmetic, and the default
	// namespace will have this query parameter omitted.
	query := url.Values{}
	if ns != "" {
		query.Set("ns", ns)
	}
	if !loop {
		query.Set("single", "1")
	}
	if len(query) > 0 {
		v += "?" + query.Encode()
	}
	return v
}
