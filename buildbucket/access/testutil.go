// Copyright 2017 The LUCI Authors.
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

package access

import (
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/proto/access"
)

// TestClient implements an access.AccessClient with stubs for use in testing.
type TestClient struct {
	*access.PermittedActionsResponse
	*access.DescriptionResponse
	Error error
}

// PermittedActions implements the AccessClient interface.
func (c *TestClient) PermittedActions(_ context.Context, req *access.PermittedActionsRequest, _ ...grpc.CallOption) (*access.PermittedActionsResponse, error) {
	return c.PermittedActionsResponse, c.Error
}

// Description implements the AccessClient interface.
func (c *TestClient) Description(_ context.Context, _ *empty.Empty, _ ...grpc.CallOption) (*access.DescriptionResponse, error) {
	return c.DescriptionResponse, c.Error
}

// PermissionsToPermittedActions converts a Permissions back into a PermittedActionsResponse.
//
// This is useful in slimming down the amount of boilerplate needed for tests, because
// Permissions is a simpler data structure.
func PermissionsToPermittedActions(p Permissions) *access.PermittedActionsResponse {
	perms := make(map[string]*access.PermittedActionsResponse_ResourcePermissions, len(p))
	for bucket, action := range p {
		var actions []string
		for a, name := range actionToName {
			if action&a == a {
				actions = append(actions, name)
			}
		}
		perms[bucket] = &access.PermittedActionsResponse_ResourcePermissions{Actions: actions}
	}
	return &access.PermittedActionsResponse{
		Permitted: perms,
		ValidityDuration: &duration.Duration{},
	}
}
