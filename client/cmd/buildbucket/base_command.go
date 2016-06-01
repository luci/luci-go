// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/api/buildbucket/buildbucket/v1"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/logging"
)

type baseCommandRun struct {
	subcommands.CommandRunBase
	serviceAccountJSONPath string
	host                   string
}

func (r *baseCommandRun) SetDefaultFlags() {
	r.Flags.StringVar(&r.serviceAccountJSONPath, "service-account-json",
		"",
		"path to service account json file.")
	r.Flags.StringVar(&r.host, "host",
		"cr-buildbucket.appspot.com",
		"host for the buildbucket service instance.")
}

func (r *baseCommandRun) makeService(ctx context.Context, a subcommands.Application) (*buildbucket.Service, error) {
	if r.host == "" {
		return nil, errors.New("a host for the buildbucket service must be provided")
	}
	authenticator := auth.NewAuthenticator(ctx, auth.OptionalLogin, auth.Options{ServiceAccountJSONPath: r.serviceAccountJSONPath})

	client, err := authenticator.Client()
	if err != nil {
		logging.Errorf(ctx, "could not create client: %s", err)
		return nil, err
	}
	logging.Debugf(ctx, "client created")

	service, err := buildbucket.New(client)
	if err != nil {
		logging.Errorf(ctx, "could not create service: %s", err)
		return nil, err
	}

	protocol := "https"
	if isLocalHost(r.host) {
		protocol = "http"
	}

	service.BasePath = fmt.Sprintf("%s://%s/_ah/api/buildbucket/v1/", protocol, r.host)
	return service, nil
}

// TODO(robertocn): Dedup this code copied from client/cmd/rpc
func isLocalHost(host string) bool {
	switch {
	case host == "localhost", strings.HasPrefix(host, "localhost:"):
	case host == "127.0.0.1", strings.HasPrefix(host, "127.0.0.1:"):
	case host == "[::1]", strings.HasPrefix(host, "[::1]:"):
	case strings.HasPrefix(host, ":"):

	default:
		return false
	}
	return true
}
