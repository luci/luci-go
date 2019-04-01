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

package cli

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdAdd(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `add [flags] builder [builder...]`,
		ShortDesc: "schedule builds",
		LongDesc: `Schedule builds.

Example:
	# Add linux-rel and mac-rel builds to chromium/ci bucket
	bb add chromium/ci/{linux-rel,mac-rel}
`,
		CommandRun: func() subcommands.CommandRun {
			r := &addRun{}
			r.RegisterGlobalFlags(p)
			r.buildFieldFlags.Register(&r.Flags)

			r.clsFlag.Register(&r.Flags, `CL URL as input for the builds. Can be specified multiple times.

Example:
	# Schedule a linux-rel tryjob for CL 1539021
	bb add -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1 chromium/try/linux-rel`,
			)

			r.commitFlag.Register(&r.Flags, `Commit URL as input to the builds.
Example:
	# Build a specific revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/7dab11d0e282bfa1d6f65cc52195f9602921d5b9 chromium/ci/linux-rel
	# Build latest chromium/src revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/master chromium/ci/linux-rel`)
			r.Flags.StringVar(&r.ref, "ref", "refs/heads/master", "Git ref for the -commit that specifies a commit hash.")

			r.tagsFlag.Register(&r.Flags, `Build tags. Can be specified multiple times.

Example:
	bb add -t a:1 -t b:2 chromium/try/linux-rel`)

			r.Flags.BoolVar(&r.experimental, "exp", false, `Mark the builds as experimental`)
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	buildFieldFlags
	clsFlag
	commitFlag
	tagsFlag
	ref          string
	experimental bool
}

func (r *addRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	baseReq, err := r.prepareBaseRequest(ctx)
	if err != nil {
		return r.done(ctx, err)
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

func (r *addRun) prepareBaseRequest(ctx context.Context) (*buildbucketpb.ScheduleBuildRequest, error) {
	ret := &buildbucketpb.ScheduleBuildRequest{
		RequestId: strconv.FormatInt(rand.Int63(), 10),
		Tags:      r.Tags(),
		Fields:    r.FieldMask(),
	}

	if r.experimental {
		ret.Experimental = buildbucketpb.Trinary_YES
	}

	var err error
	if ret.GerritChanges, err = r.retrieveCLs(ctx, r.httpClient); err != nil {
		return nil, err
	}

	if ret.GitilesCommit, err = r.retrieveCommit(); err != nil {
		return nil, err
	}
	if ret.GitilesCommit != nil && ret.GitilesCommit.Ref == "" {
		ret.GitilesCommit.Ref = r.ref
	}

	return ret, nil
}
