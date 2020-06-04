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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var (
	permCreateInvocation    = realms.RegisterPermission("resultdb.invocations.create")
	permSetProducerResource = realms.RegisterPermission("resultdb.invocations.setProducerResource")

	// trustedInvocationCreators is a CIA group that can create invocations
	// with custom IDs and specify BQ Exports.
	trustedInvocationCreators = "luci-resultdb-trusted-invocation-creators"
)

// isTrustedCreator returns a boolean indicating whether the authenticated user
// is trusted with invocation creation permissions that affect the global scope,
// namely: specifying non-guid invocation ids, and setting the BQ export field.
func isTrustedCreator(ctx context.Context) (bool, error) {
	return auth.IsMember(ctx, trustedInvocationCreators)
}

// hasPermission determines if the current caller has the specified permission
// in the given invocation's realm.
func hasPermission(ctx context.Context, perm realms.Permission, inv *pb.Invocation) (bool, error) {
	return auth.HasPermission(ctx, permCreateInvocation, []string{inv.Realm})
}

// validateCreateInvocationsPermission returns an error if the caller does not
// have permission to create invocations in their specified realms.
func validateCreateInvocationsPermission(ctx context.Context, reqs []*pb.CreateInvocationRequest) error {
	for _, req := range reqs {
		// TODO(crbug.com/1013316): Remove this fallback when realm is required in
		// all invocation creations.
		if req.GetInvocation().GetRealm() == "" {
			return nil
		}
		allowed, err := hasPermission(ctx, permCreateInvocation, req.GetInvocation())
		if err != nil {
			return err
		}
		if !allowed {
			return errors.Reason(`invocation: realm: creator does not have permission to create invocations in realm: %s`, req.GetInvocation().Realm).Err()
		}
	}
	return nil
}

// validateSetProducerResourcePermission returns an error if the caller does not
// have permission to set the producerResource field of the invocation.
func validateSetProducerResourcePermission(ctx context.Context, inv *pb.Invocation) error {
	// TODO(crbug.com/1013316): Remove this fallback when realm is required in
	// all invocation creations.
	if inv.Realm == "" {
		trusted, err := isTrustedCreator(ctx)
		if err != nil {
			return err
		}
		if trusted {
			return nil
		}
		return errors.Reason(`invocation: producer_resource: only trusted systems are allowed to set producer_resource field for now`).Err()
	}
	allowed, err := hasPermission(ctx, permSetProducerResource, inv)
	if err != nil {
		return err
	}
	if !allowed && inv.ProducerResource != "" {
		return errors.Reason(`invocation: producer_resource: only trusted systems are allowed to set producer_resource field for now`).Err()
	}
	return nil
}
