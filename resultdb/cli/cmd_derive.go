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

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const deriveUsage = `derive [flags] SWARMING_HOST TASK_ID [TASK_ID]...`

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
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &deriveRun{}
			r.queryRun.registerFlags(p)
			// TODO(crbug.com/1021849): add -base-test-variant flag.
			// TODO(crbug.com/1021849): add -base-test-path-prefix flag.
			return r
		},
	}
}

type deriveRun struct {
	queryRun
	swarmingHost string
	taskIDs      []string
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

	invNames, err := r.deriveInvocations(ctx)
	if err != nil {
		return r.done(err)
	}

	return r.done(r.queryAndPrint(ctx, &pb.InvocationPredicate{Names: invNames}))
}

// deriveInvocations derives invocations from the swarming tasks and returns
// invocation names.
func (r *deriveRun) deriveInvocations(ctx context.Context) ([]string, error) {
	eg, ctx := errgroup.WithContext(ctx)
	ret := make([]string, len(r.taskIDs))
	for i, tid := range r.taskIDs {
		i := i
		tid := tid
		eg.Go(func() error {
			res, err := r.recorder.DeriveInvocation(ctx, &pb.DeriveInvocationRequest{
				SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
					Hostname: r.swarmingHost,
					Id:       tid,
				},
			})
			if err != nil {
				return err
			}

			ret[i] = res.Name
			return nil
		})
	}
	return ret, eg.Wait()
}
