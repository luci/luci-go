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

// Command taskwatcher polls swarming tasks, calls
// luci.resultdb.rpc.v1.Deriver.DeriveChromiumInvocation and logs results.
//
// It is experimental.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func main() {
	mathrand.SeedRandomly()

	w := &watcher{
		tags: strpair.Map{},
	}
	flag.StringVar(&w.resultDBHost, "resultdb-host", chromeinfra.ResultDBHost, text.Doc(`
		ResultDB hostname.
	`))
	flag.Var(luciflag.StringPairs(w.tags), "tag", text.Doc(`
		Colon-separated key-value tags to use when searching for tasks, e.g. pool:Chrome.
	`))
	flag.StringVar(&w.swarmingHost, "swarming-host", "", text.Doc(`
		Swarming hostname, e.g. chromium-swarm.appspot.com.
	`))

	maxConcurrent := flag.Int("max-concurrent", 50, "Maximum concurrent RPCs to ResultDB")

	server.Main(nil, nil, func(srv *server.Server) error {
		if w.resultDBHost == "" {
			return errors.Reason("-resultdb-host is required").Err()
		}
		if w.swarmingHost == "" {
			return errors.Reason("-swarming-host is required").Err()
		}

		w.maxConcurrentDeriveRPCs = semaphore.NewWeighted(int64(*maxConcurrent))

		srv.RunInBackground("taskwatcher", func(ctx context.Context) {
			err := w.run(ctx)
			if err != nil && err != context.Canceled {
				srv.Fatal(err)
			}
		})
		return nil
	})
}

type watcher struct {
	resultDBHost string
	swarmingHost string
	tags         strpair.Map
	limit        *rate.Limiter

	swarming *swarming.Service
	deriver  pb.DeriverClient

	// internal state of listTasks function
	minTaskCreationTime time.Time

	maxConcurrentDeriveRPCs *semaphore.Weighted
}

func (w *watcher) run(ctx context.Context) error {
	w.limit = rate.NewLimiter(50, 1) // up to 50 QPS for Swarming task watching.

	w.minTaskCreationTime = clock.Now(ctx).UTC()

	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return err
	}
	httpClient := &http.Client{Transport: tr}

	if w.swarming, err = swarming.NewService(ctx, option.WithHTTPClient(httpClient)); err != nil {
		return err
	}
	w.swarming.UserAgent = "ResultDB taskwatcher"
	w.swarming.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", w.swarmingHost)

	w.deriver = pb.NewDeriverPRPCClient(&prpc.Client{
		C:    httpClient,
		Host: w.resultDBHost,
	})
	return w.loop(ctx)
}

// loop continuously polls swarming tasks and derives invocations.
func (w *watcher) loop(ctx context.Context) error {
	for {
		time.Sleep(time.Second) // sleep 1s before starting a new iteration
		switch err := w.loopIteration(ctx); {
		case err == context.Canceled:
			return nil
		case err != nil:
			logging.Errorf(ctx, "loop iteration failed: %s", err)
		}
	}
}

func (w *watcher) loopIteration(ctx context.Context) error {
	items, err := w.listTasks(ctx)
	if err != nil {
		return err
	}

	for _, t := range items {
		t := t
		logging.Infof(ctx, "discovered task %s", t.TaskId)
		// Start watching for this task. Do not wait for the goroutine to complete.
		go func() {
			if err := w.watchTask(ctx, t); err != nil {
				logging.Errorf(ctx, "watching task %s falied: %s", t.TaskId, err)
			}
		}()
	}
	return nil
}

func (w *watcher) watchTask(ctx context.Context, task *swarming.SwarmingRpcsTaskResult) error {
	// Poll until it finishes.
	for task.State == "PENDING" || task.State == "RUNNING" {
		time.Sleep(10 * time.Second) // do not poll for the same task too often
		if err := w.limit.Wait(ctx); err != nil {
			return err
		}
		logging.Debugf(ctx, "refreshing task %s", task.TaskId)

		reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
		freshTask, err := w.swarming.Task.Result(task.TaskId).Context(reqCtx).Do()
		cancel()
		if err != nil {
			logging.Errorf(ctx, "failed to fetch task %s; will try again later: %s", task.TaskId, err)
			time.Sleep(time.Second)
			continue
		}
		task = freshTask
	}

	// Wait for the turn.
	if err := w.maxConcurrentDeriveRPCs.Acquire(ctx, 1); err != nil {
		return err
	}
	defer w.maxConcurrentDeriveRPCs.Release(1)

	// Now call DeriveInvocation.
	res, err := w.deriver.DeriveChromiumInvocation(ctx, &pb.DeriveChromiumInvocationRequest{
		SwarmingTask: &pb.DeriveChromiumInvocationRequest_SwarmingTask{
			Hostname: w.swarmingHost,
			Id:       task.TaskId,
		},
	})
	if err != nil {
		logging.Errorf(ctx, "DeriveChromiumInvocation(%s) failed: %s", task.TaskId, err)
	} else {
		logging.Infof(ctx, "DeriveChromiumInvocation(%s) succeeded: %s", task.TaskId, res.Name)
	}
	return nil
}

// listTasks fetches a list of tasks created since w.minTaskCreationTime.
// If at least one task is fetched, updates w.minTaskCreationTime.
func (w *watcher) listTasks(ctx context.Context) ([]*swarming.SwarmingRpcsTaskResult, error) {
	if !w.minTaskCreationTime.IsZero() {
		logging.Infof(ctx, "fetching tasks created since %s", w.minTaskCreationTime)
	}

	const pageSize = 10
	cursor := ""
	var ret []*swarming.SwarmingRpcsTaskResult
	for {

		reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
		req := w.swarming.Tasks.List().
			Context(reqCtx).
			Tags(w.tags.Format()...).
			Limit(pageSize)
		if !w.minTaskCreationTime.IsZero() {
			req.Start(float64(w.minTaskCreationTime.Unix()))
		}
		if cursor != "" {
			req.Cursor(cursor)
		}

		res, err := req.Do()
		cancel()
		if err != nil {
			return nil, err
		}

		ret = append(ret, res.Items...)
		cursor = res.Cursor
		if cursor == "" || len(res.Items) < pageSize {
			break
		}
	}

	if len(ret) > 0 {
		ts, err := parseSwarmingTimestampString(ret[0].CreatedTs)
		if err != nil {
			return nil, errors.Annotate(err, "failed to parse created_ts %q", ret[0].CreatedTs).Err()
		}
		w.minTaskCreationTime = ts
	}
	return ret, nil
}

// parseSwarmingTimestampString converts a swarming-formatted string to a time.Time.
func parseSwarmingTimestampString(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, nil
	}

	// Timestamp strings from swarming should be RFC3339 without the trailing Z; check in case.
	if !strings.HasSuffix(ts, "Z") {
		ts += "Z"
	}

	return time.Parse(time.RFC3339, ts)
}
