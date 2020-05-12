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

package model

import (
	"context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// Project is the parent entity of buckets in the datastore.
// Entities of this kind don't exist in the datastore.
type Project struct {
	ID string `gae:"$id"`
}

// Bucket is a representation of a bucket in the datastore.
type Bucket struct {
	_kind string `gae:"$kind,BucketV2"`
	// ID is the bucket in v2 format.
	// e.g. try (never luci.chromium.try).
	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`

	// Bucket is the bucket in v2 format.
	// e.g. try (never luci.chromium.try).
	Bucket string `gae:"bucket_name"`
	// Proto is the buildbucketpb.Bucket proto representation of the bucket.
	//
	// acl_sets is zeroed by inlining acls. swarming.builders is
	// zeroed and stored in separate Builder datastore entities due to
	// potentially large size.
	//
	// noindex is not respected here, it's set in buildbucketpb.Bucket.ToProperty.
	Proto buildbucketpb.Bucket `gae:"config,noindex"`
	// Revision is the config revision this entity was created from.
	// TODO(crbug/1042991): Switch to noindex.
	Revision string `gae:"revision"`
	// Schema is this entity's schema version.
	// TODO(crbug/1042991): Switch to noindex.
	Schema int32 `gae:"entity_schema_version"`
}

// GetRole returns the role of current identity in this bucket. Roles are
// numerically comparable and role n implies roles [0, n-1] as well. A nil
// role implies the current identity has no permissions in this bucket.
// TODO(crbug/1042991): Move elsewhere as needed.
func (b *Bucket) GetRole(ctx context.Context) (*buildbucketpb.Acl_Role, error) {
	id := auth.CurrentIdentity(ctx)
	// Projects can do anything regardless of ACLs.
	if id.Kind() == identity.Project && id.Value() == b.Parent.StringID() {
		role := buildbucketpb.Acl_WRITER
		return &role, nil
	}
	role := buildbucketpb.Acl_Role(-1)
	for _, rule := range b.Proto.Acls {
		// Check this rule if it could potentially confer a higher role.
		if rule.Role > role {
			if rule.Identity == string(id) {
				role = rule.Role
			} else if id.Kind() == identity.User && rule.Identity == id.Email() {
				role = rule.Role
			} else {
				// Empty group membership checks always return false
				// so it doesn't matter if the group is unspecified.
				is, err := auth.IsMember(ctx, rule.Group)
				if err != nil {
					return nil, errors.Annotate(err, "failed to check group membership in %q", rule.Group).Err()
				}
				if is {
					role = rule.Role
				}
			}
		}
	}
	if role == -1 {
		return nil, nil
	}
	return &role, nil
}
