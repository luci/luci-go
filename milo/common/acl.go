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
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/memcache"

	bbAccess "go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	accessProto "go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient/access"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/server/auth"
)

// Helper functions for ACL checking.

// IsAllowed checks to see if the user in the context is allowed to access
// the given project.
func IsAllowed(c context.Context, project string) (bool, error) {
	switch admin, err := IsAdmin(c); {
	case err != nil:
		return false, err
	case admin:
		return true, nil
	}

	// Get the project, because that's where the ACLs lie.
	err := access.Check(c, backend.AsUser, config.ProjectSet(project))
	innerError := errors.Unwrap(err)

	switch {
	case err == nil:
		return true, nil
	case err == access.ErrNoAccess:
		return false, nil
	case innerError == config.ErrNoConfig:
		return false, CodeNotFound.Tag().Apply(err)
	default:
		return false, err
	}
}

// IsAdmin returns true if the current identity is an administrator.
func IsAdmin(c context.Context) (bool, error) {
	// TODO(nodir): unhardcode group name to config file if there is a need
	return auth.IsMember(c, "administrators")
}

var accessClientFactoryKey = "access client factory key"

// AccessClientFactory wraps an accessProto.AccessClient and exports its Host.
type AccessClientFactory func(c context.Context, host string) (accessProto.AccessClient, error)

// prodAccessClientFactory creates a access client tied to the given buildbucket host.
func prodAccessClientFactory(c context.Context, host string) (accessProto.AccessClient, error) {
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	return bbAccess.NewClient(host, &http.Client{Transport: t}), nil
}

// WithAccessClient returns a context with the given AccessClient.
func withAccessClientFactory(c context.Context, a AccessClientFactory) context.Context {
	return context.WithValue(c, &accessClientFactoryKey, a)
}

// getAccessClient retrieves an AccessClient from the given context, tied to
// the current user and buildbucket host.
func getAccessClient(c context.Context, host string) (accessProto.AccessClient, error) {
	factory, ok := c.Value(&accessClientFactoryKey).(AccessClientFactory)
	if !ok {
		return nil, errors.New("access client factory not found in context")
	}
	return factory(c, host)
}

// WithAccess adds a production access client tied to the current
// buildbucket instance to the context.
func WithAccess(c context.Context) (context.Context, error) {
	return withAccessClientFactory(c, prodAccessClientFactory), nil
}

// BucketPermissions gets permissions for the current identity for all given buckets.
//
// TODO(mknyszek): If a cache entry expires, then there could be QPS issues if all
// instances query buildbucket for an update simultaneously. Evaluate whether there's
// an issue in practice, and if so, consider expiring cache entries randomly.
func BucketPermissions(c context.Context, buckets ...string) (bbAccess.Permissions, error) {
	perms := make(bbAccess.Permissions, len(buckets))

	settings := GetSettings(c)
	if settings.Buildbucket.GetHost() == "" {
		return nil, errors.Reason("no buildbucket host found").Err()
	}
	bbHost := settings.Buildbucket.GetHost()
	client, err := getAccessClient(c, bbHost)
	if err != nil {
		return nil, err
	}

	// Set the namespace in the context for memcache.
	c, err = info.Namespace(c, fmt.Sprintf("buildbucket-access-%s", bbHost))
	if err != nil {
		return nil, err
	}

	// Create cache entries for each bucket.
	entries := make([]memcache.Item, len(buckets))
	identityString := string(auth.CurrentIdentity(c))
	for i, bucket := range buckets {
		entries[i] = memcache.NewItem(c, identityString+"|"+bucket)
	}

	// Check the cache.
	if err := memcache.Get(c, entries...); err != nil && err != memcache.ErrCacheMiss {
		if merr, ok := err.(errors.MultiError); ok {
			for i, err := range merr {
				if err != nil && err != memcache.ErrCacheMiss {
					logging.WithError(err).Warningf(c, "memcache get %s", entries[i].Key())
				}
			}
		} else {
			logging.WithError(err).Warningf(c, "memcache get")
		}
	}

	// Collect uncached buckets, if any. Also put cached buckets into perms.
	var bucketsToCache []string
	var itemsToUpdate []memcache.Item
	for i, bucket := range buckets {
		action := bbAccess.Action(0)
		err := action.UnmarshalBinary(entries[i].Value())
		if err != nil {
			bucketsToCache = append(bucketsToCache, bucket)
			itemsToUpdate = append(itemsToUpdate, entries[i])
			continue
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
	for i, bucket := range bucketsToCache {
		action, ok := newPerms[bucket]
		if !ok {
			continue
		}
		bytes, err := action.MarshalBinary()
		if err != nil {
			return nil, errors.Annotate(err, "failed to marshal Action").Err()
		}
		itemsToUpdate[i].SetValue(bytes)
		itemsToUpdate[i].SetExpiration(validTime)
		perms[bucket] = action
	}

	// Update the cache.
	if err := memcache.Set(c, itemsToUpdate...); err != nil {
		logging.WithError(err).Warningf(c, "memcache set")
	}

	return perms, nil
}
