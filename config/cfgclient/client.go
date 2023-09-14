// Copyright 2020 The LUCI Authors.
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

package cfgclient

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/erroring"
	"go.chromium.org/luci/config/impl/filesystem"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/config/impl/resolving"
	"go.chromium.org/luci/config/vars"
)

// Options describe how to configure a LUCI Config client.
type Options struct {
	// Vars define how to substitute ${var} placeholders in config sets and paths.
	//
	// If nil, vars are not allowed. Pass &vars.Vars explicitly to use the global
	// var set.
	Vars *vars.VarSet

	// ServiceHost is a hostname of a LUCI Config service to use.
	//
	// If given, indicates configs should be fetched from the LUCI Config service.
	// If it's Config Service V1 (where the host ends with "appspot.com"), it
	// requires ClientFactory to be provided as well.
	//
	// Not compatible with ConfigsDir.
	ServiceHost string

	// ConfigsDir is a file system directory to fetch configs from instead of
	// a LUCI Config service.
	//
	// See https://godoc.org/go.chromium.org/luci/config/impl/filesystem for the
	// expected layout of this directory.
	//
	// Useful when running locally in development mode. Not compatible with
	// ServiceHost.
	ConfigsDir string

	// ClientFactory initializes an authenticating HTTP client on demand.
	//
	// It will be used to call LUCI Config service. Must be set if ServiceHost
	// points to Config Service V1, ignored otherwise.
	ClientFactory func(context.Context) (*http.Client, error)

	// GetPerRPCCredsFn generates PerRPCCredentials for the gRPC connection.
	//
	// Must be set for calling Luci-Config v2, ignored otherwise.
	GetPerRPCCredsFn func(context.Context) (credentials.PerRPCCredentials, error)

	// UserAgent is the optional additional User-Agent fragment which will be
	// appended to gRPC calls.
	UserAgent string
}

// New instantiates a LUCI Config client based on the given options.
//
// The client fetches configs either from a LUCI Config service or from a local
// directory on disk (e.g. when running locally in development mode), depending
// on values of ServiceHost and ConfigsDir. If neither are set, returns a client
// that fails all calls with an error.
func New(ctx context.Context, opts Options) (config.Interface, error) {
	switch {
	case opts.ServiceHost == "" && opts.ConfigsDir == "":
		return erroring.New(errors.New("LUCI Config client is not configured")), nil
	case opts.ServiceHost != "" && opts.ConfigsDir != "":
		return nil, errors.New("either a LUCI Config service or a local config directory should be used, not both")
	case IsV1Host(opts.ServiceHost) && opts.ClientFactory == nil:
		return nil, errors.New("need a client factory when using a LUCI Config service v1")
	case opts.ServiceHost != "" && opts.GetPerRPCCredsFn == nil:
		return nil, errors.New("GetPerRPCCredsFn must be set when using a LUCI Config service v2")
	}

	var base config.Interface
	var err error
	switch {
	case opts.ServiceHost != "" && IsV1Host(opts.ServiceHost):
		base = remote.NewV1(opts.ServiceHost, false, opts.ClientFactory)
	case opts.ServiceHost != "":
		var creds credentials.PerRPCCredentials
		if creds, err = opts.GetPerRPCCredsFn(ctx); err == nil {
			base, err = remote.NewV2(ctx, remote.V2Options{
				Host:      opts.ServiceHost,
				Creds:     creds,
				UserAgent: opts.UserAgent,
			})
		}
	case opts.ConfigsDir != "":
		base, err = filesystem.New(opts.ConfigsDir)
	default:
		panic("impossible")
	}
	if err != nil {
		return nil, err
	}

	varz := opts.Vars
	if varz == nil {
		varz = &vars.VarSet{} // empty: all var references will result in an error
	}

	return resolving.New(varz, base), nil
}

// IsV1Host checks if the provided host points to the old v1 service.
func IsV1Host(host string) bool {
	return strings.HasSuffix(host, ".appspot.com")
}
