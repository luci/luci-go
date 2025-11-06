// Copyright 2021 The LUCI Authors.
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

package service

import (
	"context"
	"flag"
	"net/http"
	"time"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/server/bundleServicesClient"
)

// coordinatorFlags contains configuration of how to contact the coordinator.
type coordinatorFlags struct {
	// Host is the coordinator service's host:port.
	Host string
	// Insecure, if true, indicated to use HTTP instead of HTTPs.
	Insecure bool
}

// register registers flags in the flag set.
func (f *coordinatorFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.Host, "coordinator", f.Host,
		"The Coordinator service's `[host][:port]`.")
	fs.BoolVar(&f.Insecure, "coordinator-insecure", f.Insecure,
		"Connect to Coordinator over HTTP (instead of HTTPS).")
}

// validate returns an error if some parsed flags have invalid values.
func (f *coordinatorFlags) validate() error {
	if f.Host == "" {
		return errors.New("-coordinator is required")
	}
	return nil
}

// coordinator instantiates the coordinator client.
func coordinator(ctx context.Context, f *coordinatorFlags) (*bundleServicesClient.Client, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, errors.Fmt("failed to get the token source: %w", err)
	}
	prpcClient := &prpc.Client{
		C:       &http.Client{Transport: tr},
		Host:    f.Host,
		Options: prpc.DefaultOptions(),
	}
	if f.Insecure {
		prpcClient.Options.Insecure = true
	}
	return &bundleServicesClient.Client{
		ServicesClient:       logdog.NewServicesPRPCClient(prpcClient),
		DelayThreshold:       time.Second,
		BundleCountThreshold: 20,
	}, nil
}
