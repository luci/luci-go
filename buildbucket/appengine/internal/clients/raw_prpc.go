// Copyright 2023 The LUCI Authors.
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

package clients

import (
	"context"
	"net/http"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
)

// CreateRawPrpcClient creates a raw PRPC client for use with the specified host.
// Buildbucket will invoke the client's service with a Project-scoped identity.
func CreateRawPrpcClient(ctx context.Context, host, project string) (client *prpc.Client, err error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}
	client = &prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	return
}
