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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var (
	permCreateInvocation = realms.RegisterPermission("resultdb.invocations.create")

	// trusted creator permissions
	permSetProducerResource  = realms.RegisterPermission("resultdb.invocations.setProducerResource")
	permExportToBigQuery     = realms.RegisterPermission("resultdb.invocations.exportToBigQuery")
	permCreateWithReservedID = realms.RegisterPermission("resultdb.invocations.createWithReservedID")
)

// getPermissions determines if the current caller has the specified permissions
// in the given invocation's realm and returns them as a map from permission to boolean..
func getPermissions(ctx context.Context, inv *pb.Invocation, perms ...realms.Permission) (map[realms.Permission]bool, error) {
	realm := inv.GetRealm()
	if realm == "" {
		// TODO(crbug.com/1013316): Remove this fallback when realm is required in
		// all invocation creations.
		realm = "chromium:public"
	}
	var err error
	result := make(map[realms.Permission]bool, len(perms))
	for _, p := range perms {
		result[p], err = auth.HasPermission(ctx, p, []string{realm})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
