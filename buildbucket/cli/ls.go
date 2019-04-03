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
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/cli"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdLS(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `ls [flags] [PATH [PATH...]]`,
		ShortDesc: "lists builds under paths",
		LongDesc: doc(`
			Lists builds under paths.

			A PATH argument can be one of
			 - "<project>"
			 - "<project>/<bucket>"
			 - "<project>/<bucket>/<builder>"

			Listed builds are sorted by creation time, descending.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &lsRun{}
			r.RegisterDefaultFlags(p)
			r.RegisterJSONFlag()
			r.buildFieldFlags.Register(&r.Flags)
			r.clsFlag.Register(&r.Flags, doc(`
				CL URLs that builds must be associated with.

				Example: list builds of CL 1539021.
					bb ls -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1
			`))
			r.tagsFlag.Register(&r.Flags, doc(`
				Tags that builds must have. Can be specified multiple times.
				All tags must be present.

				Example: list builds with tags "a:1" and "b:2".
					bb ls -t a:1 -t b:2
			`))
			r.Flags.BoolVar(&r.includeExperimental, "exp", false, doc(`
				Print experimental builds too
			`))
			r.Flags.Var(StatusFlag(&r.status), "status",
				fmt.Sprintf("Build status. Valid values: %s.", strings.Join(StatusFlagValues, ", ")))
			return r
		},
	}
}

type lsRun struct {
	baseCommandRun
	buildFieldFlags
	clsFlag
	tagsFlag

	status              pb.Status
	includeExperimental bool
}

func (r *lsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	req, err := r.parseSearchRequests(ctx, args)
	if err != nil {
		return r.done(ctx, err)
	}

	res, err := r.client.Batch(ctx, req)
	if err != nil {
		return r.done(ctx, err)
	}

	seen := map[int64]struct{}{}
	var builds []*pb.Build
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
func (r *lsRun) parseSearchRequests(ctx context.Context, args []string) (*pb.BatchRequest, error) {
	baseReq, err := r.parseBaseRequest(ctx)
	if err != nil {
		return nil, err
	}

	ret := &pb.BatchRequest{}
	for _, path := range args {
		searchBuilds := proto.Clone(baseReq).(*pb.SearchBuildsRequest)
		var err error
		if searchBuilds.Predicate.Builder, err = r.parsePath(path); err != nil {
			return nil, fmt.Errorf("invalid path %q: %s", path, err)
		}
		ret.Requests = append(ret.Requests, &pb.BatchRequest_Request{
			Request: &pb.BatchRequest_Request_SearchBuilds{SearchBuilds: searchBuilds},
		})
	}

	// If no arguments were passed, search in any project.
	if len(ret.Requests) == 0 {
		ret.Requests = append(ret.Requests, &pb.BatchRequest_Request{
			Request: &pb.BatchRequest_Request_SearchBuilds{SearchBuilds: baseReq},
		})
	}

	return ret, nil
}

// parseBaseRequest returns a base SearchBuildsRequest without builder filter.
func (r *lsRun) parseBaseRequest(ctx context.Context) (*pb.SearchBuildsRequest, error) {
	ret := &pb.SearchBuildsRequest{
		Predicate: &pb.BuildPredicate{
			Tags:                r.Tags(),
			Status:              r.status,
			IncludeExperimental: r.includeExperimental,
		},
		Fields: r.FieldMask(),
	}

	for i, p := range ret.Fields.Paths {
		ret.Fields.Paths[i] = "builds.*." + p
	}

	var err error
	if ret.Predicate.GerritChanges, err = r.clsFlag.retrieveCLs(ctx, r.httpClient); err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *lsRun) parsePath(path string) (*pb.BuilderID, error) {
	bid := &pb.BuilderID{}
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
