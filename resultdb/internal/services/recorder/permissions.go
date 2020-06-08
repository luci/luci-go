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

	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var (
	permCreateInvocation = realms.RegisterPermission("resultdb.invocations.create")

	// trusted creator permissions
	permSetProducerResource    = realms.RegisterPermission("resultdb.invocations.setProducerResource")
	permExportToBigQuery       = realms.RegisterPermission("resultdb.invocations.exportToBigQuery")
	permCreateWithReservedName = realms.RegisterPermission("resultdb.invocations.createWithReservedName")
)

// hasPermission determines if the current caller has the specified permission
// in the given invocation's realm.
func hasPermission(ctx context.Context, perm realms.Permission, inv *pb.Invocation) (bool, error) {
	realm := inv.GetRealm()
	if realm == "" {
		// TODO(crbug.com/1013316): Remove this fallback when realm is required in
		// all invocation creations.
		realm = "chromium:public"
	}
	return auth.HasPermission(ctx, perm, []string{realm})
}

// validateCreateInvocationsPermission returns an error if the caller does not
// have a permission to create invocations in their specified realms.
func validateCreateInvocationPermission(ctx context.Context, reqs ...*pb.CreateInvocationRequest) error {
	for _, req := range reqs {
		inv := req.GetInvocation()
		allowed, err := hasPermission(ctx, permCreateInvocation, inv)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to create invocations in realm %q`, inv.Realm)
		}
	}
	return nil
}

// validateSetProducerResourcePermission returns an error if the caller does not
// have permission to set the producerResource field in any of the invocations.
func validateSetProducerResourcePermission(ctx context.Context, reqs ...*pb.CreateInvocationRequest) error {
	for _, req := range reqs {
		inv := req.GetInvocation()
		if inv.GetProducerResource() == "" {
			continue
		}
		allowed, err := hasPermission(ctx, permSetProducerResource, inv)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to set the field in realm %q`, req.GetInvocation().Realm)
		}
	}
	return nil
}
