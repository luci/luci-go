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

package recorder

import (
	"context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

var (
	permCreateInvocation = realms.RegisterPermission("resultdb.invocations.create")

	// Internal permissions
	permCreateWithReservedID = realms.RegisterPermission("resultdb.invocations.createWithReservedID")
	permExportToBigQuery     = realms.RegisterPermission("resultdb.invocations.exportToBigQuery")
	permSetProducerResource  = realms.RegisterPermission("resultdb.invocations.setProducerResource")

	recorderPermissions = []realms.Permission{
		permCreateInvocation,
		permCreateWithReservedID,
		permExportToBigQuery,
		permSetProducerResource,
	}
)

// getPermissions determines if the current caller has the permissions relevant to this service in the given realm,
// and returns them as a permissionSet.
func getPermissions(ctx context.Context, realm string) (permissionSet, error) {
	if realm == "" {
		// TODO(crbug.com/1013316): Remove this fallback when realm is required in
		// all invocation creations.
		realm = "chromium:public"
	}
	result := make(permissionSet)
	for _, p := range recorderPermissions {
		allowed, err := auth.HasPermission(ctx, p, []string{realm})
		if err != nil {
			return nil, err
		}
		if allowed {
			result[p] = true
		}
	}
	return result, nil
}

// permissionSet represents a set of granted permissions.
// This does not carry any knowledge of the realm these permissions are granted at, nor any permissions not granted.
type permissionSet map[realms.Permission]interface{}

// Has is a shortcut for checking the presence of a a key in a permissionSet.
func (ps permissionSet) Has(perm realms.Permission) bool {
	_, exists := ps[perm]
	return exists
}
