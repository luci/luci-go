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

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func cmdLS(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags] <PATH> [<PATH>...]`,
		ShortDesc: "lists builds under paths",
		LongDesc: `Lists builds under paths.

A PATH can be one of
- "<project>"
- "<project>/<bucket>"
- "<project>/<bucket>/<builder>"

Listed builds are sorted by creation time, descending.
`,
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{}
			r.RegisterGlobalFlags(defaultAuthOpts)
			r.buildFieldFlags.Register(&r.Flags)
			return r
		},
	}
}

type lsRun struct {
	baseCommandRun
	buildFieldFlags
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	req, err := r.parseSearchRequests(args)
	if err != nil {
		return r.done(ctx, err)
	}

	client, err := r.newClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	res, err := client.Batch(ctx, req)
	if err != nil {
		return r.done(ctx, err)
	}

	seen := map[int64]struct{}{}
	var builds []*buildbucketpb.Build
	for _, res := range res.Responses {
		if err := res.GetError(); err != nil {
			return r.done(ctx, fmt.Errorf("%s: %s", codes.Code(err.Code), err.Message))
		}
		for _, b := range res.GetSearchBuilds().Builds {
			if _, ok := seen[b.Id]; ok {
				continue
			}
			seen[b.Id] = struct{}{}
			builds = append(builds, b)
		}
	}
	// Sort by creation time, descending.
	// Build IDs are monotonically decreasing.
	sort.Slice(builds, func(i, j int) bool { return builds[i].Id < builds[j].Id })

	p := newStdoutPrinter(r.noColor)
	for _, b := range builds {
		if r.json {
			p.JSONPB(b)
		} else {
			p.Build(b)
			fmt.Println()
		}
	}
	return 0
}

// parseSearchRequests converts flags and arguments to a batched SearchBuilds
// requests.
func (r *lsRun) parseSearchRequests(args []string) (*buildbucketpb.BatchRequest, error) {
	baseReq, err := r.parseBaseRequest()
	if err != nil {
		return nil, err
	}

	ret := &buildbucketpb.BatchRequest{}
	for _, path := range args {
		searchBuilds := proto.Clone(baseReq).(*buildbucketpb.SearchBuildsRequest)
		var err error
		if searchBuilds.Predicate.Builder, err = r.parsePath(path); err != nil {
			return nil, fmt.Errorf("invalid path %q: %s", path, err)
		}
		ret.Requests = append(ret.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_SearchBuilds{SearchBuilds: searchBuilds},
		})
	}

	// If no arguments were passed, search in any project.
	if len(ret.Requests) == 0 {
		ret.Requests = append(ret.Requests, &buildbucketpb.BatchRequest_Request{
			Request: &buildbucketpb.BatchRequest_Request_SearchBuilds{SearchBuilds: baseReq},
		})
	}

	return ret, nil
}

// parseBaseRequest returns a base SearchBuildsRequest without builder filter.
func (r *lsRun) parseBaseRequest() (*buildbucketpb.SearchBuildsRequest, error) {
	ret := &buildbucketpb.SearchBuildsRequest{
		Predicate: &buildbucketpb.BuildPredicate{},
		Fields:    r.FieldMask(),
	}

	for i, p := range ret.Fields.Paths {
		ret.Fields.Paths[i] = "builds.*." + p
	}

	// TODO(nodir): parse flags.

	return ret, nil
}

func (r *lsRun) parsePath(path string) (*buildbucketpb.BuilderID, error) {
	bid := &buildbucketpb.BuilderID{}
	switch parts := strings.Split(path, "/"); len(parts) {
	case 3:
		bid.Builder = parts[2]
		fallthrough
	case 2:
		bid.Bucket = parts[1]
		fallthrough
	case 1:
		bid.Project = parts[0]
	default:
		return nil, fmt.Errorf("got %d components, want 1-3", len(parts))
	}
	return bid, nil
}
