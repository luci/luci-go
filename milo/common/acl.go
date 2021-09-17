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
	"context"
	"fmt"
	"net/http"

	"go.chromium.org/luci/gae/service/datastore"

	bbAccess "go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	accessProto "go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
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
		// Do not leak internal error details to unauthorized users.
		return false, errors.Reason("internal server error").
			Tag(grpcutil.InternalTag).
			InternalReason("datastore error when fetching project %q: %s", project, err).Err()
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
		// Do not leak internal error details to unauthorized users.
		return false, errors.Reason("internal server error").
			Tag(grpcutil.InternalTag).
			InternalReason("error when checking ACL: %s", err).Err()
	}
	return yes, nil
}

var accessClientKey = "access client key"

// AccessClient wraps an accessProto.AccessClient and exports its Host.
type AccessClient struct {
	accessProto.AccessClient
	Host string
}

// NewAccessClient creates a new AccessClient for talking to this milo instance's buildbucket instance.
func NewAccessClient(c context.Context) (*AccessClient, error) {
	settings := GetSettings(c)
	if settings.Buildbucket.GetHost() == "" {
		return nil, errors.Reason("no buildbucket host found").Err()
	}
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	return &AccessClient{
		AccessClient: bbAccess.NewClient(settings.Buildbucket.Host, &http.Client{Transport: t}),
		Host:         settings.Buildbucket.Host,
	}, nil
}

// WithAccessClient attaches an AccessClient to the given context.
func WithAccessClient(c context.Context, a *AccessClient) context.Context {
	return context.WithValue(c, &accessClientKey, a)
}

// GetAccessClient retrieves an AccessClient from the given context.
func GetAccessClient(c context.Context) *AccessClient {
	if client, ok := c.Value(&accessClientKey).(*AccessClient); !ok {
		panic("access client not found in context")
	} else {
		return client
	}
}

// BucketPermissions gets permissions for the current identity for all given buckets.
//
// TODO(mknyszek): If a cache entry expires, then there could be QPS issues if all
// instances query buildbucket for an update simultaneously. Evaluate whether there's
// an issue in practice, and if so, consider expiring cache entries randomly.
func BucketPermissions(c context.Context, buckets ...string) (bbAccess.Permissions, error) {
	perms := make(bbAccess.Permissions, len(buckets))
	client := GetAccessClient(c)

	var bucketsToCache []string

	cache := caching.GlobalCache(c, fmt.Sprintf("buildbucket-access-%s", client.Host))
	if cache == nil {
		panic("global cache not available in context")
	}

	// Create cache entries for each bucket.
	identityString := string(auth.CurrentIdentity(c))
	for _, bucket := range buckets {
		blob, err := cache.Get(c, identityString+"|"+bucket)
		if err != nil {
			bucketsToCache = append(bucketsToCache, bucket)
			continue
		}

		action := bbAccess.Action(0)
		err = action.UnmarshalBinary(blob)
		if err != nil {
			return nil, err
		}
		perms[bucket] = action
	}

	// Finish early if all of the buckets were in the cache.
	if len(bucketsToCache) == 0 {
		return perms, nil
	}

	// Make an RPC to get uncached buckets from buildbucket.
	newPerms, validTime, err := bbAccess.BucketPermissions(c, client, bucketsToCache)
	if err != nil {
		return nil, err
	}

	// Update items, collect them, and put their values into perms.
	for _, bucket := range bucketsToCache {
		action, ok := newPerms[bucket]
		if !ok {
			continue
		}
		bytes, err := action.MarshalBinary()
		if err != nil {
			return nil, errors.Annotate(err, "failed to marshal Action").Err()
		}
		err = cache.Set(c, identityString+"|"+bucket, bytes, validTime)
		if err != nil {
			logging.WithError(err).Warningf(c, "failed to cache permission for bucket: %s", bucket)
		}
		perms[bucket] = action
	}

	return perms, nil
}
