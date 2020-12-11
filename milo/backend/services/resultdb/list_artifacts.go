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

package resultdbproxy

import (
	"context"

	"go.chromium.org/luci/milo/resultdb"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
)

// ListArtifacts implements milopb.MiloInternal service
func (s *Service) ListArtifacts(ctx context.Context, req *resultpb.ListArtifactsRequest) (*resultpb.ListArtifactsResponse, error) {
	host, err := resultdb.GetHost(ctx)
	if err != nil {
		return nil, err
	}
	client, err := resultdb.ResultdbClient(ctx, host, auth.AsUser)
	if err != nil {
		return nil, err
	}
	inv, err := client.ListArtifacts(ctx, req)
	return inv, err
}
