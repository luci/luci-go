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
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/sync/parallel"

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
			r.Flags.Var(flag.StringSlice(&r.cls), "cl", "CL URL as input for the builds. Can be specified multiple times.")
			r.Flags.StringVar(&r.commit, "commit", "", "Commit URL as input to the builds.")
			r.Flags.StringVar(&r.ref, "ref", "refs/heads/master", "Git ref for the -commit")
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	buildFieldFlags
	cls    []string
	commit string
	ref    string
}

func (r *addRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	baseReq := &buildbucketpb.ScheduleBuildRequest{
		RequestId: strconv.FormatInt(rand.Int63(), 10),
	}

	var err error
	if baseReq.GerritChanges, err = r.parseCLs(ctx); err != nil {
		return r.done(ctx, err)
	}

	if baseReq.GitilesCommit, err = r.parseCommit(); err != nil {
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

// parseCLs parses r.cls.
func (r *addRun) parseCLs(ctx context.Context) ([]*buildbucketpb.GerritChange, error) {
	ret := make([]*buildbucketpb.GerritChange, len(r.cls))
	return ret, parallel.FanOutIn(func(work chan<- func() error) {
		for i, cl := range r.cls {
			i := i
			work <- func() error {
				change, err := r.retrieveCL(ctx, cl)
				if err != nil {
					return fmt.Errorf("parsing CL %q: %s", cl, err)
				}
				ret[i] = change
				return nil
			}
		}
	})
}

// parseCommit parses r.commit.
func (r *addRun) parseCommit() (*buildbucketpb.GitilesCommit, error) {
	if r.commit == "" {
		return nil, nil
	}

	commit, mustConfirmRef, err := parseCommit(r.commit)
	switch {
	case err != nil:
		return nil, fmt.Errorf("invalid -commit: %s", err)

	case mustConfirmRef:
		fmt.Printf("Please confirm the git ref [%s] ", commit.Ref)
		var reply string
		fmt.Scanln(&reply)
		reply = strings.TrimSpace(reply)
		if reply != "" {
			commit.Ref = reply
		}

	case commit.Ref == "":
		commit.Ref = r.ref
	}
	return commit, nil
}
