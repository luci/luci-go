// Copyright 2015 The LUCI Authors.
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

package bootstrap

import (
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/common/viewer"
)

// ErrNotBootstrapped is returned by Get when the current process is not
// bootstrapped.
var ErrNotBootstrapped = errors.New("not bootstrapped")

// Bootstrap contains information about the configured bootstrap environment.
//
// The bootstrap environment is loaded by probing the local application
// environment for variables emitted by a bootstrapping Butler.
type Bootstrap struct {
	// CoordinatorHost is the name of the upstream Coordinator host.
	//
	// This is just the host name ("example.appspot.com"), not a full URL.
	//
	// If this instance is not configured using a production Coordinator Output,
	// this will be empty.
	CoordinatorHost string

	// Project is the Butler instance project name.
	Project string
	// Prefix is the Butler instance prefix.
	Prefix types.StreamName
	// Namespace is prefix for stream names.
	Namespace types.StreamName

	// Client is the streamclient for this instance, or nil if the Butler has no
	// streamserver.
	Client *streamclient.Client
}

// GetFromEnv loads a Bootstrap instance from the given environment.
//
// It will return an error if the bootstrap data is invalid, and will return
// ErrNotBootstrapped if the current process is not bootstrapped.
func GetFromEnv(env environ.Env) (*Bootstrap, error) {
	// Detect Butler by looking for EnvStreamServerPath in the envrironent. This
	// is the only environment variable which matters for constructing a Butler
	// Client; all the rest are just needed to assemble viewer URLs.
	butlerSocket, ok := env.Lookup(EnvStreamServerPath)
	if !ok {
		return nil, ErrNotBootstrapped
	}

	bs := &Bootstrap{
		CoordinatorHost: env.Get(EnvCoordinatorHost),
		Prefix:          types.StreamName(env.Get(EnvStreamPrefix)),
		Namespace:       types.StreamName(env.Get(EnvNamespace)),
		Project:         env.Get(EnvStreamProject),
	}
	if err := bs.initializeClient(butlerSocket); err != nil {
		return nil, fmt.Errorf("bootstrap: failed to create stream client [%s]: %s", butlerSocket, err)
	}

	if len(bs.Prefix) > 0 {
		if err := bs.Prefix.Validate(); err != nil {
			return nil, fmt.Errorf("bootstrap: failed to validate prefix %q: %s", bs.Prefix, err)
		}
	}
	if len(bs.Project) > 0 {
		if err := config.ValidateProjectName(bs.Project); err != nil {
			return nil, fmt.Errorf("bootstrap: failed to validate project %q: %s", bs.Project, err)
		}
	}
	if len(bs.Namespace) > 0 {
		if err := bs.Namespace.Validate(); err != nil {
			return nil, fmt.Errorf("bootstrap: failed to validate namespace %q: %s", bs.Namespace, err)
		}
	}

	return bs, nil
}

func (bs *Bootstrap) initializeClient(v string) error {
	c, err := streamclient.New(v, bs.Namespace)
	if err != nil {
		return errors.Annotate(err, "bootstrap: failed to create stream client [%s]", v).Err()
	}
	bs.Client = c
	return nil
}

// Get is shorthand for `GetFromEnv(environ.System())`.
func Get() (*Bootstrap, error) {
	return GetFromEnv(environ.System())
}

// GetViewerURL returns a log stream viewer URL to the aggregate set of supplied
// stream paths.
//
// If both the Project and CoordinatorHost values are not populated, an error
// will be returned.
func (bs *Bootstrap) GetViewerURL(paths ...types.StreamPath) (string, error) {
	if bs.Project == "" {
		return "", errors.New("no project is configured")
	}
	if bs.CoordinatorHost == "" {
		return "", errors.New("no coordinator host is configured")
	}
	return viewer.GetURL(bs.CoordinatorHost, bs.Project, paths...), nil
}
