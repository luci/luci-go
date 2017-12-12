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
	"encoding/binary"
	"net/http"
	"time"

	"golang.org/x/net/context"

	bbAccess "go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient/access"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"
)

var accessChecksCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(0),
	GlobalNamespace: "milo_buildbucket_access_checks",
	Marshal: func(item interface{}) ([]byte, error) {
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, item.(uint32))
		return bytes, nil
	},
	Unmarshal: func(blob []byte) (interface{}, error) {
		if len(blob) != 4 {
			return 0, errors.New("found malformed cache entry")
		}
		return binary.LittleEndian.Uint32(blob), nil
	},
}

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

// BucketPermissions gets permissions for the user for all given buckets.
func BucketPermissions(c context.Context, buckets []string) (bbAccess.Permissions, error) {
	var validTime time.Duration
	var gotPerms bool
	perms := make(bbAccess.Permissions, len(buckets))
	for _, bucket := range buckets {
		cached, err := accessChecksCache.GetOrCreate(c, bucket, func() (interface{}, time.Duration, error) {
			// If we've already RPC'd and pulled the info, just use it to create new entries.
			if gotPerms {
				return perms[bucket], validTime, nil
			}

			// Otherwise, prepare to make RPC to get permissions.
			settings := GetSettings(c)
			if settings.Buildbucket == nil {
				return nil, 0, errors.Reason("no buildbucket config found").Err()
			}
			t, err := auth.GetRPCTransport(c, auth.AsUser)
			if err != nil {
				return nil, 0, errors.Annotate(err, "getting RPC Transport").Err()
			}
			permsClient := bbAccess.NewClient(settings.Buildbucket.Host, &http.Client{Transport: t})

			// Make RPC and write results back so we can re-use them.
			perms, validTime, err = bbAccess.BucketPermissions(c, permsClient, buckets)
			if err != nil {
				return nil, 0, err
			}
			gotPerms = true
			return perms[bucket], validTime, nil
		})
		if err != nil {
			return nil, err
		}
		if !gotPerms {
			perms[bucket] = cached.(bbAccess.Action)
		}
	}
	return perms, nil
}
