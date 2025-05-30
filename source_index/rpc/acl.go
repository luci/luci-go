// Copyright 2024 The LUCI Authors.
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

package rpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/source_index/internal/gitilesutil"
)

// wasPublicRepoCacheDuration is the cache duration for whether the repository
// was public.
//
// We don't return any confidential data in the RPC. Note that commit hashes
// from confidential repositories are not considered confidential (see
// b/352641557). Even if a public repository was made confidential while the
// result is cached, the public already knew that the repository existed. As a
// result, this can be cached for a long time. However, we don't want to cache
// it for too long because
//  1. if a confidential repo is made public, we want to update the cached value
//     so that all users will start using the same cache, and
//  2. the amount of traffic hitting the same cache key will be a lot more than
//     the canKnowRepoExistsCache since the cache key is not namespaced by the
//     user identity. We don't need to keep the cache for too long.
var wasPublicRepoCacheDuration = time.Hour * 24
var wasPublicRepoCache = layered.RegisterCache(layered.Parameters[bool]{
	GlobalNamespace: "was-public-repository-v1",
	Marshal: func(item bool) ([]byte, error) {
		if item {
			return []byte{0}, nil
		}
		return nil, nil
	},
	Unmarshal: func(blob []byte) (bool, error) {
		return len(blob) > 0, nil
	},
})

// wasPublicRepo checks whether an anonymous user had access to the repository
// by calling Gitiles with no credential.
func wasPublicRepo(ctx context.Context, host, repository string) (bool, error) {
	client, err := gitilesutil.NewClient(ctx, host, auth.NoAuth)
	if err != nil {
		return false, errors.Fmt("initialize Gitiles client: %w", err)
	}
	cacheKey := fmt.Sprintf("gitiles/%q/repository/%q", host, repository)
	canAccessRepo, err := wasPublicRepoCache.GetOrCreate(ctx, cacheKey, func() (v bool, exp time.Duration, err error) {
		_, err = client.GetProject(ctx, &gitilespb.GetProjectRequest{Name: repository})
		switch status.Code(err) {
		case codes.OK:
			return true, wasPublicRepoCacheDuration, nil
		case codes.PermissionDenied, codes.NotFound, codes.Unauthenticated:
			// It's Ok to cache failed response here. Even if this is no longer
			// correct, we will check again with the user's credential.
			return false, wasPublicRepoCacheDuration, nil
		default:
			return false, 0, err
		}
	})
	if err != nil {
		return false, err
	}
	return canAccessRepo, nil
}

// canKnowRepoExistsCacheDuration is the cache duration for whether
// the user should be able to know the existence of a Gitiles repository.
//
// We don't return any confidential data in the RPC. Note that commit hashes
// from confidential repositories are not considered confidential (see
// b/352641557). Even if the user's access to a repository was revoked while the
// result is cached, they already knew that the repository existed. As a result,
// this can be cached for a long time.
var canKnowRepoExistsCacheDuration = time.Hour * 24 * 7 * 4
var canKnowRepoExistsCache = layered.RegisterCache(layered.Parameters[bool]{
	GlobalNamespace: "can-know-repository-exists-v1",
	Marshal: func(item bool) ([]byte, error) {
		if item {
			return []byte{0}, nil
		}
		return nil, nil
	},
	Unmarshal: func(blob []byte) (bool, error) {
		return len(blob) > 0, nil
	},
})

// ensureCanKnowRepoExists checks whether the user has access to the repository
// by calling Gitiles with the user's credential.
//
// This is to prevent unauthorized users using this RPC to verify the
// existence of private repositories.
//
// Note that we do not check position_ref here because position_ref does not
// necessarily need to match an actual branch. It can be an arbitrary string
// in the commit message.
//
// Do not check whether the user has access to the commit either because
//  1. commit hashes are not considered to be confidential (see b/352641557);
//     and
//  2. we must check ACL before reading the database otherwise unauthorized
//     users can still verify the existence of a repository by observing the
//     query latency.
func ensureCanKnowRepoExists(ctx context.Context, host, repository string) error {
	// If the repo was a public repo, everyone knows it existed. No point checking
	// with the user's credential. This is more efficient because all users can
	// share the same cache.
	wasPublic, err := wasPublicRepo(ctx, host, repository)
	if err != nil {
		// Log the error only. We have another chance to check the access with the
		// user's credential. This matters when Gitiles returns an unexpected
		// status.
		logging.WithError(err).Errorf(ctx, "check whether the repo was public")
	} else if wasPublic {
		return nil
	}

	client, err := gitilesutil.NewClient(ctx, host, auth.AsCredentialsForwarder)
	if err != nil {
		return errors.Fmt("initialize Gitiles client: %w", err)
	}
	cacheKey := fmt.Sprintf("user/%q/gitiles/%q/repository/%q", string(auth.CurrentUser(ctx).Identity), host, repository)
	canAccessRepo, err := canKnowRepoExistsCache.GetOrCreate(ctx, cacheKey, func() (v bool, exp time.Duration, err error) {
		_, err = client.GetProject(ctx, &gitilespb.GetProjectRequest{Name: repository})
		if err != nil {
			// Do not cache failed responses. Always return an error. Otherwise users
			// will be locked out of the service even when they are later granted
			// access. Additionally, the provided user credential might be missing
			// required OAuth scopes, which might be fixed in later calls.
			return false, 0, err
		}
		return true, canKnowRepoExistsCacheDuration, nil
	})
	if err != nil {
		return appstatus.Attachf(err, grpcutil.Code(err), "cannot access repository https://%s/%s", host, repository)
	}
	if !canAccessRepo {
		// This branch should never been hit.
		return appstatus.Errorf(codes.Internal, "invariant violated: the user must be have access to the repository")
	}
	return nil
}
