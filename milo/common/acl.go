// Copyright 2016 The LUCI Authors.
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

package common

import (
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/errors"
	accessProto "go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient/access"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/server/auth"
)

// Helper functions for ACL checking.

// IsAllowedProject checks to see if the user in the context is allowed to access
// the given project.
func IsAllowedProject(c context.Context, project string) (bool, error) {
	switch admin, err := IsAdmin(c); {
	case err != nil:
		return false, err
	case admin:
		return true, nil
	}
	// Get the project, because that's where the ACLs lie.
	err := access.Check(
		c, backend.AsUser,
		cfgtypes.ProjectConfigSet(cfgtypes.ProjectName(project)))
	switch err {
	case nil:
		return true, nil
	case access.ErrNoAccess:
		return false, nil
	default:
		return false, err
	}
}

// Permissions represents a set of permitted actions for a set of buckets.
//
// The slice of actions will always be small (<15 elements) so searching
// linearly is reasonably fast.
type Permissions map[string][]buildbucket.Action

// Can checks whether an Action is allowed for a given bucket.
func (p Permissions) Can(bucket string, action buildbucket.Action) bool {
	actions, ok := p[bucket]
	if !ok {
		return false
	}
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}

// BucketPermissions retrieves the current identity's permitted actions for
// a set of buckets.
func BucketPermissions(c context.Context, buckets []string) (Permissions, error) {
	// Set up buildbucket pRPC client.
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.Reason("failed to get transport for buildbucket server").Err()
	}
	client := accessProto.NewAccessPRPCClient(&prpc.Client{
		C:    &http.Client{Transport: t},
		Host: GetSettings(c).Buildbucket.Host,
	})

	// Make permitted actions call.
	req := accessProto.PermittedActionsRequest{
		ResourceKind: "bucket",
		ResourceIds:  buckets,
	}
	resp, err := client.PermittedActions(c, &req)
	if err != nil {
		return nil, err
	}

	// Parse proto into a convenient format.
	perms := make(map[string][]buildbucket.Action, len(resp.Permitted))
	for bucket, actions := range resp.Permitted {
		newActions := make([]buildbucket.Action, len(actions.Actions))
		for _, action := range actions.Actions {
			newAction, err := buildbucket.ParseAction(action)
			if err != nil {
				return nil, err
			}
			newActions = append(newActions, newAction)
		}
		perms[bucket] = newActions
	}
	return perms, nil
}

// IsAdmin returns true if the current identity is an administrator.
func IsAdmin(c context.Context) (bool, error) {
	// TODO(nodir): unhardcode group name to config file if there is a need
	return auth.IsMember(c, "administrators")
}
