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

package rbe

import (
	"context"
	"fmt"
	"net/http"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

// NewInternalsClient constructs an RPC client to call Swarming internals service.
func NewInternalsClient(ctx context.Context, projectID string) (internalspb.InternalsClient, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return internalspb.NewInternalsClient(
		&prpc.Client{
			C:       &http.Client{Transport: t},
			Host:    fmt.Sprintf("%s.appspot.com", projectID),
			Options: prpc.DefaultOptions(),
		},
	), nil
}
