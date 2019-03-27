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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

type baseCommandRun struct {
	subcommands.CommandRunBase
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
	host           string
	json           bool
}

func (r *baseCommandRun) SetDefaultFlags(defaultAuthOpts auth.Options) {
	r.Flags.StringVar(
		&r.host,
		"host",
		"cr-buildbucket.appspot.com",
		"host for the buildbucket service instance.")
	r.Flags.BoolVar(
		&r.json,
		"json",
		false,
		"Print information in JSON format.")
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
	var err error
	if r.parsedAuthOpts, err = r.authFlags.Options(); err != nil {
		return nil, err
	}

	authenticator := auth.NewAuthenticator(ctx, auth.OptionalLogin, r.parsedAuthOpts)
	return authenticator.Client()
}

func (r *baseCommandRun) newLegacyClient(ctx context.Context) (*legacyClient, error) {
	if err := r.validateHost(); err != nil {
		return nil, err
	}

	httpClient, err := r.createHTTPClient(ctx)
	if err != nil {
		return nil, err
	}

	protocol := "https"
	if lhttp.IsLocalHost(r.host) {
		protocol = "http"
	}

	return &legacyClient{
		HTTP: httpClient,
		baseURL: &url.URL{
			Scheme: protocol,
			Host:   r.host,
			Path:   "/_ah/api/buildbucket/v1/",
		},
	}, nil
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

func (r *baseCommandRun) done(ctx context.Context, err error) int {
	if err != nil {
		logging.Errorf(ctx, "%s", err)
		return 1
	}
	return 0
}

// callAndDone makes a buildbucket API call, prints error or response, and returns exit code.
func (r *baseCommandRun) callAndDone(ctx context.Context, method, relURL string, body interface{}) int {
	client, err := r.newLegacyClient(ctx)
	if err != nil {
		return r.done(ctx, err)
	}

	resBytes, err := client.call(ctx, method, relURL, body)
	if err != nil {
		return r.done(ctx, err)
	}

	var res struct {
		Error *buildbucket.ApiErrorMessage
	}
	switch err := json.Unmarshal(resBytes, &res); {
	case err != nil:
		return r.done(ctx, err)
	case res.Error != nil:
		logging.Errorf(ctx, "Request error (reason: %s): %s", res.Error.Reason, res.Error.Message)
		return 1
	}

	fmt.Printf("%s\n", resBytes)
	return 0
}

// buildIDArg can be embedded into a subcommand that accepts a build ID.
type buildIDArg struct {
	buildID int64
}

func (a *buildIDArg) parseArgs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing parameter: <Build ID>")
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments: %s", args[1:])
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("expected a build id (int64), got %s: %s", args[0], err)
	}

	a.buildID = id
	return nil
}

type repeatedBuildIDArg struct {
	buildIDs []int64
}

func (a *repeatedBuildIDArg) parseArgs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing parameter: <Build ID>")
	}

	a.buildIDs = make([]int64, len(args))
	for i, arg := range args {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return fmt.Errorf("expected a build id (int64), got %s: %s", arg, err)
		}
		a.buildIDs[i] = id
	}

	return nil
}
