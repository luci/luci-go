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

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"

	structpb "github.com/golang/protobuf/ptypes/struct"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdAdd(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `add [flags] [BUILDER [[BUILDER...]]`,
		ShortDesc: "add builds",
		LongDesc: doc(`
			Add a build for each BUILDER argument.

			A BUILDER must have format "<project>/<bucket>/<builder>", for example "chromium/try/linux-rel".

			Example: add linux-rel and mac-rel builds to chromium/ci bucket using Shell expansion.
				bb add chromium/ci/{linux-rel,mac-rel}
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &addRun{}
			r.RegisterDefaultFlags(p)
			r.RegisterJSONFlag()

			r.clsFlag.Register(&r.Flags, doc(`
				CL URL as input for the builds. Can be specified multiple times.

				Example: add a linux-rel tryjob for CL 1539021
					bb add -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1 chromium/try/linux-rel
			`))
			r.commitFlag.Register(&r.Flags, doc(`
				Commit URL as input to the builds.

				Example: build a specific revision
					bb add -commit https://chromium.googlesource.com/chromium/src/+/7dab11d0e282bfa1d6f65cc52195f9602921d5b9 chromium/ci/linux-rel

				Example: build latest chromium/src revision
					bb add -commit https://chromium.googlesource.com/chromium/src/+/master chromium/ci/linux-rel
			`))
			r.Flags.StringVar(&r.ref, "ref", "refs/heads/master", "Git ref for the -commit that specifies a commit hash.")
			r.tagsFlag.Register(&r.Flags, doc(`
				Build tags. Can be specified multiple times.

				Example: add a build with tags "a:1" and "b:2".
					bb add -t a:1 -t b:2 chromium/try/linux-rel
			`))
			r.Flags.BoolVar(&r.experimental, "exp", false, doc(`
				Mark the builds as experimental
			`))
			r.Flags.Var(PropertiesFlag(&r.properties), "p", doc(`
				Input properties for the build.

				If a flag value starts with @, properties are read from the JSON file at the
				path that follows @. Example:
					bb add -p @my_properties.json chromium/try/linux-rel
				This form can be used only in the first flag value.

				Otherwise, a flag value must have name=value form.
				If the property value is valid JSON, then it is parsed as JSON;
				otherwise treated as a string. Example:
					bb add -p foo=1 -p 'bar={"a": 2}' chromium/try/linux-rel
				Different property names can be specified multiple times.
			`))
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	clsFlag
	commitFlag
	tagsFlag

	ref          string
	experimental bool
	properties   structpb.Struct
}

func (r *addRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) == 0 {
		return 0
	}

	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx); err != nil {
		return r.done(ctx, err)
	}

	baseReq, err := r.prepareBaseRequest(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	req := &pb.BatchRequest{}
	for i, a := range args {
		schedReq := proto.Clone(baseReq).(*pb.ScheduleBuildRequest)
		schedReq.RequestId += fmt.Sprintf("-%d", i)

		var err error
		schedReq.Builder, err = protoutil.ParseBuilderID(a)
		if err != nil {
			return r.done(ctx, fmt.Errorf("invalid builder %q: %s", a, err))
		}
		req.Requests = append(req.Requests, &pb.BatchRequest_Request{
			Request: &pb.BatchRequest_Request_ScheduleBuild{ScheduleBuild: schedReq},
		})
	}

	return r.batchAndDone(ctx, req)
}

func (r *addRun) prepareBaseRequest(ctx context.Context) (*pb.ScheduleBuildRequest, error) {
	ret := &pb.ScheduleBuildRequest{
		RequestId:  uuid.New().String(),
		Tags:       r.Tags(),
		Fields:     completeBuildFieldMask,
		Properties: &r.properties,
	}

	if r.experimental {
		ret.Experimental = pb.Trinary_YES
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
