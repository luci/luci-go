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

package projectconfig

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
)

// Helper functions for ACL checking.

// IsAllowed checks to see if the user in the context is allowed to access
// the given project.
//
// Returns false for unknown projects. Returns an internal error if the check
// itself fails.
func IsAllowed(c context.Context, project string) (bool, error) {
	proj := Project{ID: project}
	switch err := datastore.Get(c, &proj); {
	case err == datastore.ErrNoSuchEntity:
		return false, nil
	case err != nil:
		logging.Errorf(c, "datastore error when fetching project %q: %s", project, err)
		return false, errors.New("internal server error", grpcutil.InternalTag)
	default:
		return CheckACL(c, proj.ACL)
	}
}

// CheckACL returns true if the caller is in the ACL.
//
// Returns an internal error if the check itself fails.
func CheckACL(c context.Context, acl ACL) (bool, error) {
	// Try to find a direct hit first, it is cheaper.
	caller := auth.CurrentIdentity(c)
	for _, ident := range acl.Identities {
		if caller == ident {
			return true, nil
		}
	}
	// More expensive groups check comes second. Note that admins implicitly have
	// access to all projects.
	// TODO(nodir): unhardcode group name to config file if there is a need
	yes, err := auth.IsMember(c, append(acl.Groups, "administrators")...)
	if err != nil {
		logging.Errorf(c, "error when checking administrators ACL: %s", err)
		return false, errors.New("internal server error", grpcutil.InternalTag)
	}
	return yes, nil
}
