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
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/sync/parallel"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdAdd(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `add [flags] builder [builder...]`,
		ShortDesc: "schedule builds",
		LongDesc: `Schedule builds.

Example:
	# Add linux-rel and mac-rel builds to chromium/ci bucket
	bb add infra/ci/{linux-rel,mac-rel}
`,
		CommandRun: func() subcommands.CommandRun {
			r := &addRun{}
			r.SetDefaultFlags(defaultAuthOpts)
			r.Flags.Var(
				flag.StringSlice(&r.cls),
				"cl",
				`CL URL as input for the builds. Can be specified multiple times.

Example:
	# Schedule a linux-rel tryjob for CL 1539021
	bb add -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1 infra/try/linux-rel`,
			)

			r.Flags.StringVar(
				&r.commit,
				"commit",
				"",
				`Commit URL as input to the builds.
Example:
	# Build a specific revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/7dab11d0e282bfa1d6f65cc52195f9602921d5b9 infra/ci/linux-rel`,
			)

			r.Flags.StringVar(
				&r.ref,
				"ref",
				"refs/heads/master",
				`Git ref for the -commit`,
			)

			r.Flags.BoolVar(
				&r.experimental,
				"exp", false,
				`Mark the builds as experimental.`,
			)

			r.Flags.Var(
				flag.StringSlice(&r.tags),
				"t",
				`Tag for the builds, e.g. "foo:bar". Can be specified multiple times.

Example:
	bb add -t a:b -t buildset:my-builds/1234 infra/ci/linux-rel`,
			)

			r.Flags.Var(
				flag.StringSlice(&r.properties),
				"p",
				`Input properties for the build.
If starts with @, all properties are read a file that follows @.
Example:
	bb add -p @my_properties.json chromium/try/linux-rel

Otherwise, must have A=B form, where A is a property name and B is a property value.
If A is valid JSON, then it is parsed as JSON; otherwise treated as a string.
Can be specified multiple times.
Example:
	bb add -p foo=1 -p 'bar={"a": 2}' chromium/try/linux-rel
`)
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	cls          []string
	commit       string
	ref          string
	experimental bool
	tags         []string
	properties   []string
}

func (r *addRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

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
		Fields:    completeBuildFieldMask,
	}

	var err error
	if ret.GerritChanges, err = r.parseCLs(ctx); err != nil {
		return nil, err
	}

	if ret.GitilesCommit, err = r.parseCommit(); err != nil {
		return nil, err
	}

	if r.experimental {
		ret.Experimental = buildbucketpb.Trinary_YES
	}

	if ret.Tags, err = r.parseTags(); err != nil {
		return nil, err
	}

	return ret, nil
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

	commit, confirmRef, err := parseCommit(r.commit)
	switch {
	case err != nil:
		return nil, err

	case confirmRef && !confirm(fmt.Sprintf("Git ref %q?", commit.Ref), true):
		return nil, fmt.Errorf("Please specify a valid ref name, that starts with refs/")

	case commit.Ref == "":
		commit.Ref = r.ref
	}
	return commit, nil
}

func (r *addRun) parseTags() ([]*buildbucketpb.StringPair, error) {
	ret := make([]*buildbucketpb.StringPair, len(r.tags))
	for i, t := range r.tags {
		if !strings.Contains(t, ":") {
			return nil, fmt.Errorf("invalid tag %q: no colon", t)
		}
		k, v := strpair.Parse(t)
		ret[i] = &buildbucketpb.StringPair{Key: k, Value: v}
	}
	return ret, nil
}
