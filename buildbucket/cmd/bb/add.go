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
	"os"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/sync/parallel"

	structpb "github.com/golang/protobuf/ptypes/struct"
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
			r.RegisterGlobalFlags(defaultAuthOpts)
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
	bb add -commit https://chromium.googlesource.com/chromium/src/+/7dab11d0e282bfa1d6f65cc52195f9602921d5b9 infra/ci/linux-rel
	# Build latest revision
	bb add -commit https://chromium.googlesource.com/chromium/src/+/master infra/ci/linux-rel`,
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

			r.tagsFlag.Register(&r.Flags, `Tag the builds with key-value pairs, e.g. "foo:bar". Can be specified multiple times.

Example:
	bb add -t a:b -t buildset:my-builds/1234 infra/ci/linux-rel`,
			)

			r.Flags.Var(
				flag.StringSlice(&r.properties),
				"p",
				`Input properties for the build.

If it starts with @, properties are read from the JSON file at the path that
follows @. Only one -p value can start with @.
Example:
	bb add -p @my_properties.json chromium/try/linux-rel

Otherwise, must have A=B format, where A is a property name and B is a property
value. If B is valid JSON, then it is parsed as JSON; otherwise treated as a
string. Can be specified multiple times.
Example:
	bb add -p foo=1 -p 'bar={"a": 2}' chromium/try/linux-rel

If both forms are used together, values specified as A=B override the ones in
the file. `)
			return r
		},
	}
}

type addRun struct {
	baseCommandRun
	tagsFlag
	cls          []string
	commit       string
	ref          string
	experimental bool
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
		RequestId: uuid.New().String(),
		Fields:    completeBuildFieldMask,
		Tags:      r.tagsFlag.tags,
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

	if ret.Properties, err = r.parseProperties(); err != nil {
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

// parseProperties parses r.properties.
func (r *addRun) parseProperties() (*structpb.Struct, error) {
	if len(r.properties) == 0 {
		return nil, nil
	}

	fileName := ""
	for _, p := range r.properties {
		switch {
		case !strings.HasPrefix(p, "@"):
		case fileName != "":
			return nil, fmt.Errorf("multiple property files")
		default:
			fileName = p[1:]
		}
	}

	ret := &structpb.Struct{}
	if fileName != "" {
		f, err := os.Open(fileName)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := jsonpb.Unmarshal(f, ret); err != nil {
			return nil, fmt.Errorf("failed to parse %q: %s", fileName, err)
		}
	}

	// Parse A=B property values.
	for _, p := range r.properties {
		if strings.HasPrefix(p, "@") {
			continue
		}

		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 1 {
			return nil, fmt.Errorf("invalid property %q: no equals sign", p)
		}
		name := parts[0]
		value := parts[1]

		// Try parsing as JSON.
		// Note: jsonpb cannot unmarshal structpb.Value from JSON natively,
		// so we have to wrap JSON value in an object.
		wrappedJSON := fmt.Sprintf(`{"a": %s}`, value)
		buf := &structpb.Struct{}
		if err := jsonpb.UnmarshalString(wrappedJSON, buf); err == nil {
			ret.Fields[name] = buf.Fields["a"]
		} else {
			// Treat as string.
			ret.Fields[name] = &structpb.Value{
				Kind: &structpb.Value_StringValue{StringValue: value},
			}
		}
	}
	return ret, nil
}
