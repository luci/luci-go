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
	// Requires ClientFactory to be provided as well. Not compatible with
	// ConfigsDir.
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
	// It will be used to call LUCI Config service. Must be set if ServiceHost is
	// set, ignored otherwise.
	ClientFactory func(context.Context) (*http.Client, error)
}

// New instantiates a LUCI Config client based on the given options.
//
// The client fetches configs either from a LUCI Config service or from a local
// directory on disk (e.g. when running locally in development mode), depending
// on values of ServiceHost and ConfigsDir. If neither are set, returns a client
// that fails all calls with an error.
func New(opts Options) (config.Interface, error) {
	switch {
	case opts.ServiceHost == "" && opts.ConfigsDir == "":
		return erroring.New(errors.New("LUCI Config client is not configured")), nil
	case opts.ServiceHost != "" && opts.ConfigsDir != "":
		return nil, errors.New("either a LUCI Config service or a local config directory should be used, not both")
	case opts.ServiceHost != "" && opts.ClientFactory == nil:
		return nil, errors.New("need a client factory when using a LUCI Config service")
	}

	var base config.Interface
	switch {
	case opts.ServiceHost != "":
		base = remote.New(opts.ServiceHost, false, opts.ClientFactory)
	case opts.ConfigsDir != "":
		var err error
		if base, err = filesystem.New(opts.ConfigsDir); err != nil {
			return nil, err
		}
	default:
		panic("impossible")
	}

	varz := opts.Vars
	if varz == nil {
		varz = &vars.VarSet{} // empty: all var references will result in an error
	}

	return resolving.New(varz, base), nil
}
