// Copyright 2018 The LUCI Authors.
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

package main

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	buildbucket "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/cli"
)

func cmdRetry(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `retry [flags] <build id>`,
		ShortDesc: "retry a build",
		LongDesc:  "Retry a build.",
		CommandRun: func() subcommands.CommandRun {
			r := &retryRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			return r
		},
	}
}

type retryRun struct {
	baseCommandRun
	buildIDArg
}

func (r *retryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	id, err := uuid.NewRandom()
	if err != nil {
		return r.done(ctx, err)
	}

	req := &buildbucket.ApiRetryRequestMessage{ClientOperationId: id.String()}
	if err := r.parseArgs(args); err != nil {
		return r.done(ctx, err)
	}

	return r.callAndDone(ctx, "PUT", fmt.Sprintf("builds/%d/retry", r.buildID), req)
}
