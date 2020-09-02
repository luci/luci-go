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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
)

var (
	permCreateInvocation  = realms.RegisterPermission("resultdb.invocations.create")
	permIncludeInvocation = realms.RegisterPermission("resultdb.invocations.include")

	// Internal permissions
	permCreateWithReservedID = realms.RegisterPermission("resultdb.invocations.createWithReservedID")
	permExportToBigQuery     = realms.RegisterPermission("resultdb.invocations.exportToBigQuery")
	permSetProducerResource  = realms.RegisterPermission("resultdb.invocations.setProducerResource")
)

// verifyPermissionBatch is checks multiple invocations' realms for the specified
// permission.
func verifyPermissionBatch(ctx context.Context, permission realms.Permission, ids invocations.IDSet) (err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.resultdb.verifyPermissionBatch")
	defer func() { ts.End(err) }()

	realms, err := invocations.ReadRealms(span.Single(ctx), ids)
	if err != nil {
		return err
	}

	checked := stringset.New(1)
	for id, realm := range realms {
		if !checked.Add(realm) {
			continue
		}
		// Note: HasPermission does not make RPCs.
		switch allowed, err := auth.HasPermission(ctx, permission, realm); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm of invocation %s`, permission, id)
		}
	}
	return nil
}
