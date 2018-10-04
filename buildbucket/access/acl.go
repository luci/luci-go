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
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/grpc/prpc"
)

// Permissions represents a set of permitted actions for a set of buckets.
//
// Since Action is encoded as bits, the Action here is actually the bitwise OR
// of all available Actions for a particular bucket.
type Permissions map[string]Action

// Can checks whether an Action is allowed for a given bucket.
func (p Permissions) Can(bucket string, action Action) bool {
	return (p[bucket] & action) == action
}

// FromProto populates a Permissions from an access.PermittedActionsResponse.
func (p Permissions) FromProto(resp *access.PermittedActionsResponse) error {
	// Clean up p before we populate it.
	for bucket := range p {
		delete(p, bucket)
	}
	for bucket, actions := range resp.Permitted {
		newActions := Action(0)
		for _, action := range actions.Actions {
			newAction, err := ParseAction(action)
			if err != nil {
				return err
			}
			newActions |= newAction
		}
		p[bucket] = newActions
	}
	return nil
}

// ToProto converts a Permissions into a PermittedActionsResponse.
func (p Permissions) ToProto(validTime time.Duration) *access.PermittedActionsResponse {
	perms := make(map[string]*access.PermittedActionsResponse_ResourcePermissions, len(p))
	for bucket, action := range p {
		var actions []string
		for a, name := range actionToName {
			if action&a == a {
				actions = append(actions, name)
			}
		}
		sort.Strings(actions)
		perms[bucket] = &access.PermittedActionsResponse_ResourcePermissions{Actions: actions}
	}
	return &access.PermittedActionsResponse{
		Permitted:        perms,
		ValidityDuration: ptypes.DurationProto(validTime),
	}
}

// NewClient returns a new AccessClient which can be used to talk to
// buildbucket's access API.
func NewClient(host string, client *http.Client) access.AccessClient {
	return access.NewAccessPRPCClient(&prpc.Client{
		Host: host,
		C:    client,
	})
}

// BucketPermissions retrieves permitted actions for a set of buckets, for the
// identity specified in the client. It also returns the duration for which the
// client is allowed to cache the permissions.
func BucketPermissions(c context.Context, client access.AccessClient, buckets []string) (Permissions, time.Duration, error) {
	// Make permitted actions call.
	req := access.PermittedActionsRequest{
		ResourceKind: "bucket",
		ResourceIds:  buckets,
	}
	resp, err := client.PermittedActions(c, &req)
	if err != nil {
		return nil, 0, err
	}

	// Parse proto into a convenient format.
	perms := make(Permissions, len(resp.Permitted))
	if err := perms.FromProto(resp); err != nil {
		return nil, 0, err
	}
	dur, err := ptypes.Duration(resp.ValidityDuration)
	if err != nil {
		return nil, 0, err
	}
	return perms, dur, nil
}
