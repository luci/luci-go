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
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"

	bbAccess "go.chromium.org/luci/buildbucket/access"
	accessProto "go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient/access"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
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

// IsAdmin returns true if the current identity is an administrator.
func IsAdmin(c context.Context) (bool, error) {
	// TODO(nodir): unhardcode group name to config file if there is a need
	return auth.IsMember(c, "administrators")
}

// AccessClient is a high-level interface for getting bucket permissions from buildbucket.
//
// The main purpose of this interface is to stub out bbAccess.BucketPermissions for testing,
// rather than relying on the low-level accessProto.AccessClient interface.
type AccessClient interface {
	bucketPermissions(c context.Context, buckets []string) (bbAccess.Permissions, time.Duration, error)
}

type accessClientImpl struct {
	accessProto.AccessClient
}

func (a *accessClientImpl) bucketPermissions(c context.Context, buckets []string) (bbAccess.Permissions, time.Duration, error) {
	return bbAccess.BucketPermissions(c, a.AccessClient, buckets)
}

// BuildbucketClient returns a new AccessClient for talking to this milo instance's buildbucket instance.
// The only pRPC service exposed by buildbucket currently is the access API.
func NewAccessClient(c context.Context) (AccessClient, error) {
	settings := GetSettings(c)
	if settings.Buildbucket == nil {
		return nil, errors.Reason("no buildbucket config found").Err()
	}
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	return &accessClientImpl{
		AccessClient: bbAccess.NewClient(settings.Buildbucket.Host, &http.Client{Transport: t}),
	}, nil
}

// BucketPermissions gets permissions for the user for all given buckets.
func BucketPermissions(c context.Context, client AccessClient, buckets []string) (bbAccess.Permissions, error) {
	perms := make(bbAccess.Permissions, len(buckets))

	// Create cache entries for each bucket.
	entries := make(map[string]memcache.Item, len(buckets))
	entrySlice := make([]memcache.Item, 0, len(buckets))
	for _, bucket := range buckets {
		item := memcache.NewItem(c, auth.CurrentIdentity(c).Email()+"|"+bucket)
		entries[bucket] = item
		entrySlice = append(entrySlice, item)
	}

	// Check the cache.
	if err := memcache.Get(c, entrySlice...); err != nil {
		logging.WithError(err).Warningf(c, "memcache get")
	}

	// Collect uncached buckets, if any. Also put cached buckets into perms.
	var uncachedBuckets []string
	for bucket, entry := range entries {
		action := bbAccess.Action(0)
		err := (&action).UnmarshalBinary(entry.Value())
		if err != nil {
			uncachedBuckets = append(uncachedBuckets, bucket)
			continue
		}
		perms[bucket] = action
	}

	// Finish early if all of the buckets were in the cache.
	if len(uncachedBuckets) == 0 {
		return perms, nil
	}

	// Make an RPC call to get uncached buckets from buildbucket.
	newPerms, validTime, err := client.bucketPermissions(c, uncachedBuckets)
	if err != nil {
		return nil, err
	}

	// Update items, collect them, and put their values into perms.
	updatedItems := make([]memcache.Item, len(newPerms))
	for bucket, action := range newPerms {
		bytes, err := (&action).MarshalBinary()
		if err != nil {
			return nil, errors.New("failed to marshal Action")
		}
		entries[bucket].SetValue(bytes)
		entries[bucket].SetExpiration(validTime)
		updatedItems = append(updatedItems, entries[bucket])
		perms[bucket] = action
	}

	// Update the cache.
	if err := memcache.Set(c, updatedItems...); err != nil {
		logging.WithError(err).Warningf(c, "memcache set")
	}

	return perms, nil
}
