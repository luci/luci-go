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

package main

import (
	"fmt"
	"math/rand"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdAdd(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `add [flags] builder [builder...]`,
		ShortDesc: "schedule builds",
		LongDesc: `Schedule builds.

Examples:
	# Add linux-rel and mac-rel builds to chromium/ci bucket
	bb add infra/ci/{linux-rel,mac-rel}

	# Build a specific revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/7dab11d0e282bfa1d6f65cc52195f9602921d5b9 infra/ci/linux-rel

	# Add linux-rel tryjob for a CL.
	bb add -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1 infra/try/linux-rel
`,
		CommandRun: func() subcommands.CommandRun {
			r := &addRun{}
			r.RegisterGlobalFlags(defaultAuthOpts)
			r.buildFieldFlags.Register(&r.Flags)

			r.clsFlag.Register(&r.Flags, `CL URL as input for the builds. Can be specified multiple times.

Example:
	# Schedule a linux-rel tryjob for CL 1539021
	bb add -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1 infra/try/linux-rel`,
			)

			r.commitFlag.Register(&r.Flags, `Commit URL as input to the builds.
Example:
	# Build a specific revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/7dab11d0e282bfa1d6f65cc52195f9602921d5b9 infra/ci/linux-rel
	# Build latest revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/master infra/ci/linux-rel`)

			r.Flags.StringVar(&r.ref, "ref", "refs/heads/master", "Git ref for the -commit that specifies a commit hash.")
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	buildFieldFlags
	clsFlag
	commitFlag
	ref string
}

func (r *addRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	baseReq := &buildbucketpb.ScheduleBuildRequest{
		RequestId: strconv.FormatInt(rand.Int63(), 10),
	}

	var err error
	if baseReq.GerritChanges, err = r.retrieveCLs(ctx, r.httpClient); err != nil {
		return r.done(ctx, err)
	}

	if baseReq.GitilesCommit, err = r.retrieveCommit(); err != nil {
		return r.done(ctx, err)
	}
	if baseReq.GitilesCommit != nil && baseReq.GitilesCommit.Ref == "" {
		baseReq.GitilesCommit.Ref = r.ref
	}

	req := &buildbucketpb.BatchRequest{}
	for i, a := range args {
		schedReq := proto.Clone(baseReq).(*buildbucketpb.ScheduleBuildRequest)
		schedReq.RequestId += fmt.Sprintf("-%d", i)

		var err error
		schedReq.Builder, err = protoutil.ParseBuilderID(a)
		if err != nil {
			return r.done(ctx, fmt.Errorf("invalid builder %q: %s", a, err))
		}
		req.Requests = append(req.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_ScheduleBuild{ScheduleBuild: schedReq},
		})
	}

	return r.batchAndDone(ctx, req)
}
