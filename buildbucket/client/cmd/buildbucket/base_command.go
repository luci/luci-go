// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/lhttp"
	"github.com/luci/luci-go/common/logging"
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
			Path:   "/api/buildbucket/v1/",
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

	res, err := client.call(ctx, method, relURL, body)
	if err != nil {
		return r.done(ctx, err)
	}

	fmt.Printf("%s\n", res)
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
