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

package casviewer

import (
	"context"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// NewClient connects to the instance of remote execution service, and returns a client.
func NewClient(ctx context.Context, instance string) (*client.Client, error) {
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get credentials").Err()
	}

	c, err := client.NewClient(ctx, instance,
		client.DialParams{
			Service:            "remotebuildexecution.googleapis.com:443",
			TransportCredsOnly: true,
		}, &client.PerRPCCreds{Creds: creds})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create client").Err()
	}

	return c, nil
}
