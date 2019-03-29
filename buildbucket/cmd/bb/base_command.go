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
	"net/http"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

type baseCommandRun struct {
	subcommands.CommandRunBase
	authFlags authcli.Flags
	host      string
	json      bool
	noColor   bool
}

func (r *baseCommandRun) RegisterGlobalFlags(defaultAuthOpts auth.Options) {
	r.Flags.StringVar(
		&r.host,
		"host",
		"cr-buildbucket.appspot.com",
		"Host for the buildbucket service instance.")
	r.Flags.BoolVar(
		&r.json,
		"json",
		false,
		"Print information in JSON format.")
	r.Flags.BoolVar(
		&r.noColor,
		"nocolor",
		false,
		"Disable coloration.")
	r.authFlags.Register(&r.Flags, defaultAuthOpts)
}

func (r *baseCommandRun) validateHost() error {
	if r.host == "" {
		return fmt.Errorf("a host for the buildbucket service must be provided")
	}
	if strings.ContainsRune(r.host, '/') {
		return fmt.Errorf("invalid host %q", r.host)
	}
	return nil
}

func (r *baseCommandRun) createHTTPClient(ctx context.Context) (*http.Client, error) {
	opts, err := r.authFlags.Options()
	if err != nil {
		return nil, err
	}
	return auth.NewAuthenticator(ctx, auth.SilentLogin, opts).Client()
}

func (r *baseCommandRun) newClient(ctx context.Context) (buildbucketpb.BuildsClient, error) {
	if err := r.validateHost(); err != nil {
		return nil, err
	}

	httpClient, err := r.createHTTPClient(ctx)
	if err != nil {
		return nil, err
	}

	opts := prpc.DefaultOptions()
	opts.Insecure = lhttp.IsLocalHost(r.host)

	info, err := version.GetCurrentVersion()
	if err != nil {
		return nil, err
	}
	opts.UserAgent = fmt.Sprintf("buildbucket CLI, instanceID=%q", info.InstanceID)

	return buildbucketpb.NewBuildsPRPCClient(&prpc.Client{
		C:       httpClient,
		Host:    r.host,
		Options: opts,
	}), nil
}

// batchAndDone executes req and prints the response.
func (r *baseCommandRun) batchAndDone(ctx context.Context, req *buildbucketpb.BatchRequest) int {
	client, err := r.newClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	res, err := client.Batch(ctx, req)
	if err != nil {
		return r.done(ctx, err)
	}

	hasErr := false
	p := newStdoutPrinter(r.noColor)
	for i, res := range res.Responses {
		var build *buildbucketpb.Build
		switch res := res.Response.(type) {

		case *buildbucketpb.BatchResponse_Response_Error:
			hasErr = true

			var requestTitle string
			switch req := req.Requests[i].Request.(type) {
			case *buildbucketpb.BatchRequest_Request_GetBuild:
				r := req.GetBuild
				if r.Id != 0 {
					requestTitle = fmt.Sprintf("build %d", r.Id)
				} else {
					requestTitle = fmt.Sprintf(`build "%s/%d"`, protoutil.FormatBuilderID(r.Builder), r.BuildNumber)
				}

			case *buildbucketpb.BatchRequest_Request_CancelBuild:
				requestTitle = fmt.Sprintf("build %d", req.CancelBuild.Id)

			default:
				requestTitle = fmt.Sprintf("request #%d", i)
			}

			fmt.Fprintf(os.Stderr, "%s: %s: %s\n", requestTitle, codes.Code(res.Error.Code), res.Error.Message)
			continue

		case *buildbucketpb.BatchResponse_Response_GetBuild:
			build = res.GetBuild
		case *buildbucketpb.BatchResponse_Response_CancelBuild:
			build = res.CancelBuild
		case *buildbucketpb.BatchResponse_Response_ScheduleBuild:
			build = res.ScheduleBuild
		default:
			panic("forgot to update batchAndDone()?")
		}

		if r.json {
			p.JSONPB(build)
		} else {
			if i > 0 {
				p.f("\n")
			}
			p.Build(build)
		}
	}
	if hasErr {
		return 1
	}
	return 0
}

func (r *baseCommandRun) done(ctx context.Context, err error) int {
	if err != nil {
		logging.Errorf(ctx, "%s", err)
		return 1
	}
	return 0
}

// retrieveCL retrieves GerritChange from a string.
// Makes a Gerrit RPC if necessary.
func (r *baseCommandRun) retrieveCL(ctx context.Context, cl string) (*buildbucketpb.GerritChange, error) {
	ret, err := parseCL(cl)
	if err != nil {
		return nil, err
	}

	if ret.Project != "" && ret.Patchset != 0 {
		return ret, nil
	}

	// Fetch CL info from Gerrit.
	httpClient, err := r.createHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	client, err := gerrit.NewRESTClient(httpClient, ret.Host, true)
	if err != nil {
		return nil, err
	}
	change, err := client.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  ret.Change,
		Options: []gerritpb.QueryOption{gerritpb.QueryOption_CURRENT_REVISION},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CL %d from %q: %s", ret.Change, ret.Host, err)
	}

	ret.Project = change.Project
	ret.Patchset = int64(change.Revisions[change.CurrentRevision].Number)
	return ret, nil
}
