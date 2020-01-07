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
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/grpc/prpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"

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
	req := &pb.DeriveInvocationRequest{
		SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
			Hostname: r.swarmingHost,
			Id:       taskID,
		},
	}

	if !r.wait {
		return r.recorder.DeriveInvocation(ctx, req)
	}

	var inv *pb.Invocation
	err := retry.Retry(ctx, newPollingIter, func() error {
		var err error
		inv, err = r.recorder.DeriveInvocation(ctx, req, prpc.ExpectedCode(codes.OK, codes.FailedPrecondition))
		if isTaskIncomplete(err) {
			return errors.Annotate(err, "task is not complete yet").Tag(notReady).Err()
		}
		return err
	}, func(err error, d time.Duration) {
		logging.Infof(ctx, "task %s is incomplete; will wait for %s", taskID, d)
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func isTaskIncomplete(err error) bool {
	return hasPreconditionViolationOfType(
		status.Convert(err).Details(),
		pb.DeriveInvocationPreconditionFailureType_INCOMPLETE_SWARMING_TASK.String(),
	)
}

func hasPreconditionViolationOfType(details []interface{}, typ string) bool {
	for _, d := range details {
		if f, ok := d.(*errdetails.PreconditionFailure); ok {
			for _, v := range f.Violations {
				if v.Type == typ {
					return true
				}
			}
		}
	}
	return false
}
