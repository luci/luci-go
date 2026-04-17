// Copyright 2026 The LUCI Authors.
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
	"io"
	"os"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func cmdReadWorkplan(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `read-workplan [flags]`,
		ShortDesc: "read data from a workplan",
		LongDesc: text.Doc(`
			Make a TurboCIOrchestrator.ReadWorkPlan RPC.

			The request message is read from stdin, in binary format.
			The response is printed to stdout, also in binary format.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &readWorkplanRun{}
			r.RegisterGlobalFlags(p)
			return r
		},
	}
}

type readWorkplanRun struct {
	baseCommandRun
}

func (r *readWorkplanRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClient(ctx); err != nil {
		return r.done(err)
	}
	if err := r.run(ctx); err != nil {
		return r.done(err)
	}
	return 0
}

func (r *readWorkplanRun) run(ctx context.Context) error {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	var req orchestratorpb.ReadWorkPlanRequest
	if err := proto.Unmarshal(in, &req); err != nil {
		return fmt.Errorf("failed to parse ReadWorkPlanRequest from stdin: %w", err)
	}
	resp, err := r.client.ReadWorkPlan(ctx, &req)
	if err != nil {
		return err
	}
	out, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal ReadWorkPlanResponse: %w", err)
	}
	if _, err = os.Stdout.Write(out); err != nil {
		return fmt.Errorf("failed to write to stdout: %w", err)
	}
	return nil
}
