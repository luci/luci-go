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

// Command prederiver polls swarming tasks, calls
// luci.resultdb.rpc.v1.Recocorder.DeriveInvocation and logs results.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/api/option"
	"google.golang.org/grpc/metadata"

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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func main() {
	mathrand.SeedRandomly()

	p := &prederiver{tags: strpair.Map{}}
	flag.StringVar(&p.resultDBHost, "resultdb-host", chromeinfra.ResultDBHost, text.Doc(`
		ResultDB hostname.
	`))
	flag.Var(luciflag.StringPairs(p.tags), "tag", text.Doc(`
		Colon-separated key-value tags to use when searching for tasks, e.g. pool:Chrome.
	`))
	flag.StringVar(&p.swarmingHost, "swarming-host", "", text.Doc(`
		Swarming hostname, e.g. chromium-swarm.appspot.com.
	`))

	server.Main(nil, func(srv *server.Server) error {
		if p.resultDBHost == "" {
			return errors.Reason("-resultdb-host is required").Err()
		}
		if p.swarmingHost == "" {
			return errors.Reason("-swarming-host is required").Err()
		}

		srv.RunInBackground("prederiver", func(ctx context.Context) {
			err := p.run(ctx)
			if err != nil && err != context.Canceled {
				srv.Fatal(err)
			}
		})
		return nil
	})
}

type prederiver struct {
	resultDBHost           string
	swarmingHost           string
	tags                   strpair.Map
	swarmingRequestLimiter *rate.Limiter

	swarming *swarming.Service
	recorder pb.RecorderClient

	// internal state of listTasks function
	minTaskCreationTime time.Time
}

func (p *prederiver) run(ctx context.Context) error {
	p.swarmingRequestLimiter = rate.NewLimiter(10, 1) // up to 10 QPS for Swarming.

	p.minTaskCreationTime = clock.Now(ctx).UTC()

	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return err
	}
	httpClient := &http.Client{Transport: tr}

	if p.swarming, err = swarming.NewService(ctx, option.WithHTTPClient(httpClient)); err != nil {
		return err
	}
	p.swarming.UserAgent = "ResultDB prederiver"
	p.swarming.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", p.swarmingHost)

	prpcOpts := prpc.DefaultOptions()
	prpcOpts.Insecure = true // TODO(crbug.com/1020691): remove when we have a domain name.
	p.recorder = pb.NewRecorderPRPCClient(&prpc.Client{
		C: httpClient,

		// TODO(crbug.com/1020691): use p.resultDBHost when it works
		Host:    "35.227.221.130",
		Options: prpcOpts,
	})
	return p.loop(ctx)
}

// loop continuously polls swarming tasks and derives invocations.
func (p *prederiver) loop(ctx context.Context) error {
	for {
		switch err := p.loopIteration(ctx); {
		case err == context.Canceled:
			return nil
		case err != nil:
			logging.Errorf(ctx, "loop iteration failed: %s", err)
		}
	}
}

func (p *prederiver) loopIteration(ctx context.Context) error {
	items, err := p.listTasks(ctx)
	if err != nil {
		return err
	}

	for _, t := range items {
		t := t
		logging.Infof(ctx, "discovered task %s", t.TaskId)
		// Start watching for this task. Do not wait for the goroutine to complete.
		go func() {
			if err := p.watchTask(ctx, t); err != nil {
				logging.Infof(ctx, "watching task %s falied: %s", t.TaskId, err)
			}
		}()
	}
	return nil
}

func (p *prederiver) watchTask(ctx context.Context, task *swarming.SwarmingRpcsTaskResult) error {
	// Poll until it finishes.
	for task.State == "PENDING" || task.State == "RUNNING" {
		time.Sleep(10 * time.Second) // do not poll for the same task too often
		if err := p.swarmingRequestLimiter.Wait(ctx); err != nil {
			return err
		}
		logging.Debugf(ctx, "refreshing task %s", task.TaskId)

		reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
		freshTask, err := p.swarming.Task.Result(task.TaskId).Context(reqCtx).Do()
		cancel()
		if err != nil {
			logging.Errorf(ctx, "failed to fetch task %s; will try again later", task.TaskId)
			time.Sleep(time.Second)
		}
		task = freshTask
	}

	// TODO(crbug.com/1020691): remove once we have a domain name
	ctx = metadata.AppendToOutgoingContext(ctx, "host", p.resultDBHost)
	res, err := p.recorder.DeriveInvocation(ctx, &pb.DeriveInvocationRequest{
		SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
			Hostname: p.swarmingHost,
			Id:       task.TaskId,
		},
	})
	if err != nil {
		logging.Errorf(ctx, "DeriveInvocation(%s) failed: %s", task.TaskId, err)
	} else {
		logging.Infof(ctx, "DeriveInvocation(%s) succeeded: %s", task.TaskId, res.Name)
	}
	return nil
}

// listTasks fetches a list of tasks till p.taskCreationTimeEnd.
// If at least one task is fetched, updates p.taskCreationTimeEnd.
func (p *prederiver) listTasks(ctx context.Context) ([]*swarming.SwarmingRpcsTaskResult, error) {
	if !p.minTaskCreationTime.IsZero() {
		logging.Infof(ctx, "fetching tasks created before %s", p.minTaskCreationTime)
	}
	const pageSize = 10
	cursor := ""
	var ret []*swarming.SwarmingRpcsTaskResult
	for {
		if err := p.swarmingRequestLimiter.Wait(ctx); err != nil {
			return nil, err
		}

		reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
		req := p.swarming.Tasks.List().
			Context(reqCtx).
			Tags(p.tags.Format()...).
			Limit(pageSize)
		if !p.minTaskCreationTime.IsZero() {
			req.Start(float64(p.minTaskCreationTime.Unix()))
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

		logging.Infof(ctx, "next page...")
	}
	logging.Infof(ctx, "done paging")

	if len(ret) > 0 {
		ts, err := parseSwarmingTimestampString(ret[0].CreatedTs)
		if err != nil {
			return nil, errors.Annotate(err, "failed to parse created_ts %q", ret[0].CreatedTs).Err()
		}
		p.minTaskCreationTime = ts
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
