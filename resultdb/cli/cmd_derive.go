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

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const deriveUsage = `chromium-derive [flags] SWARMING_HOST TASK_ID [TASK_ID]...`

func cmdDerive(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: deriveUsage,
		ShortDesc: "derive results from Chromium swarming tasks and query them",
		LongDesc: text.Doc(`
			Derives Invocation(s) from Chromium Swarming task(s) and prints results,
			like ls subcommand.
			If an invocation already exists for a given task, then reuses it.

			SWARMING_HOST must be a hostname without a scheme, e.g.
			chromium-swarm.appspot.com.

			This subcommand is temporary. It exists only to aid transition to
			ResultDB.
			TODO(1030191): remove this subcommand.
		`),
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			r := &deriveRun{}
			r.queryRun.registerFlags(p)
			r.Flags.BoolVar(&r.wait, "wait", false, text.Doc(`
				Wait for the tasks to complete.
				Without waiting, if the task is incomplete, exits with an error.
			`))
			return r
		},
	}
}

type deriveRun struct {
	queryRun
	swarmingHost string
	taskIDs      []string
	wait         bool
}

func (r *deriveRun) parseArgs(args []string) error {
	if len(args) < 2 {
		return errors.Reason("usage: %s", deriveUsage).Err()
	}

	r.swarmingHost = args[0]
	r.taskIDs = args[1:]

	if strings.Contains(r.swarmingHost, "/") {
		return errors.Reason("invalid swarming host %q", r.swarmingHost).Err()
	}

	return r.queryRun.validate()
}

func (r *deriveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	invIDs, err := r.deriveInvocations(ctx)
	if err != nil {
		return r.done(err)
	}

	return r.done(r.queryAndPrint(ctx, invIDs))
}

// deriveInvocations derives invocations from the swarming tasks and returns
// invocation ids.
func (r *deriveRun) deriveInvocations(ctx context.Context) ([]string, error) {
	eg, ctx := errgroup.WithContext(ctx)
	ret := make([]string, len(r.taskIDs))
	for i, tid := range r.taskIDs {
		i := i
		tid := tid
		eg.Go(func() error {
			res, err := r.deriveInvocation(ctx, tid)
			if err != nil {
				return err
			}

			ret[i], err = pbutil.ParseInvocationName(res.Name)
			return err
		})
	}
	return ret, eg.Wait()
}

// deriveInvocation derives an invocation from a task.
func (r *deriveRun) deriveInvocation(ctx context.Context, taskID string) (*pb.Invocation, error) {
	task := &pb.DeriveInvocationRequest_SwarmingTask{
		Hostname: r.swarmingHost,
		Id:       taskID,
	}
	if r.wait {
		if err := r.waitForTaskToComplete(ctx, task); err != nil {
			return nil, err
		}
	}

	return r.recorder.DeriveInvocation(ctx, &pb.DeriveInvocationRequest{
		SwarmingTask: task,
	})
}

func (r *deriveRun) waitForTaskToComplete(ctx context.Context, task *pb.DeriveInvocationRequest_SwarmingTask) error {
	svc, err := swarming.New(r.http)
	if err != nil {
		return err
	}
	svc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", task.Hostname)

	return retry.Retry(ctx, newPollingIter, func() error {
		res, err := svc.Task.Result(task.Id).Context(ctx).Do()
		if err, ok := err.(*googleapi.Error); ok && err.Code >= 500 {
			return transient.Tag.Apply(err)
		}
		if err != nil {
			return err
		}

		if res.State == "PENDING" || res.State == "RUNNING" {
			return errors.Reason("incomplete task").Tag(notReady).Err()
		}

		return nil
	}, func(err error, d time.Duration) {
		if notReady.In(err) {
			logging.Infof(ctx, "task %s is incomplete; will wait for %s", task.Id, d)
		} else {
			logging.Warningf(ctx, "transient error while fetching task %s: %s; will wait for %s", task.Id, err, d)
		}
	})
}
