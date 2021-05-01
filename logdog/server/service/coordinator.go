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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
)

// CoordinatorFlags contains configuration of how to contact the coordinator.
type CoordinatorFlags struct {
	// Host is the coordinator service's host:port.
	Host string
	// Insecure, if true, indicated to use HTTP instead of HTTPs.
	Insecure bool
}

// Register registers flags in the flag set.
func (f *CoordinatorFlags) Register(fs *flag.FlagSet) {
	fs.StringVar(&f.Host, "coordinator", f.Host,
		"The Coordinator service's `[host][:port]`.")
	fs.BoolVar(&f.Insecure, "coordinator-insecure", f.Insecure,
		"Connect to Coordinator over HTTP (instead of HTTPS).")
}

// Validate returns an error if some parsed flags have invalid values.
func (f *CoordinatorFlags) Validate() error {
	if f.Host == "" {
		return errors.New("-coordinator is required")
	}
	return nil
}

// Coordinator instantiates the connection to the coordinator.
func Coordinator(ctx context.Context, f *CoordinatorFlags) (logdog.ServicesClient, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the token source").Err()
	}
	prpcClient := &prpc.Client{
		C:       &http.Client{Transport: tr},
		Host:    f.Host,
		Options: prpc.DefaultOptions(),
	}
	if f.Insecure {
		prpcClient.Options.Insecure = true
	}
	return logdog.NewServicesPRPCClient(prpcClient), nil
}
