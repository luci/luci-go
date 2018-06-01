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
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"

	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
)

type baseCommandRun struct {
	subcommands.CommandRunBase
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
	host           string
}

func (r *baseCommandRun) SetDefaultFlags(defaultAuthOpts auth.Options) {
	r.Flags.StringVar(
		&r.host,
		"host",
		"cr-buildbucket.appspot.com",
		"host for the buildbucket service instance.")
	r.authFlags.Register(&r.Flags, defaultAuthOpts)
}

func (r *baseCommandRun) createClient(ctx context.Context) (*client, error) {
	if r.host == "" {
		return nil, fmt.Errorf("a host for the buildbucket service must be provided")
	}
	if strings.ContainsRune(r.host, '/') {
		return nil, fmt.Errorf("invalid host %q", r.host)
	}
	var err error
	if r.parsedAuthOpts, err = r.authFlags.Options(); err != nil {
		return nil, err
	}

	authenticator := auth.NewAuthenticator(ctx, auth.OptionalLogin, r.parsedAuthOpts)
	httpClient, err := authenticator.Client()
	if err != nil {
		return nil, err
	}

	protocol := "https"
	if lhttp.IsLocalHost(r.host) {
		protocol = "http"
	}

	return &client{
		HTTP: httpClient,
		baseURL: &url.URL{
			Scheme: protocol,
			Host:   r.host,
			Path:   "/_ah/api/buildbucket/v1/",
		},
	}, nil
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
	client, err := r.createClient(ctx)
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
