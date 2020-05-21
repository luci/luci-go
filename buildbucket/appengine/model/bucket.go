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

	pb "go.chromium.org/luci/buildbucket/proto"
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
	// Proto is the pb.Bucket proto representation of the bucket.
	//
	// acl_sets is zeroed by inlining acls. swarming.builders is
	// zeroed and stored in separate Builder datastore entities due to
	// potentially large size.
	//
	// noindex is not respected here, it's set in pb.Bucket.ToProperty.
	Proto pb.Bucket `gae:"config,noindex"`
	// Revision is the config revision this entity was created from.
	// TODO(crbug/1042991): Switch to noindex.
	Revision string `gae:"revision"`
	// Schema is this entity's schema version.
	// TODO(crbug/1042991): Switch to noindex.
	Schema int32 `gae:"entity_schema_version"`
}

// NoRole indicates the user has no defined role in a bucket.
const NoRole pb.Acl_Role = -1

// GetRole returns the role of current identity in this bucket. Roles are
// numerically comparable and role n implies roles [0, n-1] as well. May return
// NoRole if the current identity has no defined role in this bucket.
// TODO(crbug/1042991): Move elsewhere as needed.
func (b *Bucket) GetRole(ctx context.Context) (pb.Acl_Role, error) {
	id := auth.CurrentIdentity(ctx)
	// Projects can do anything in their own buckets regardless of ACLs.
	if id.Kind() == identity.Project && id.Value() == b.Parent.StringID() {
		return pb.Acl_WRITER, nil
	}
	// Admins can do anything in all buckets regardless of ACLs.
	switch is, err := auth.IsMember(ctx, "administrators"); {
	case err != nil:
		return NoRole, errors.Annotate(err, "failed to check group membership in %q", "administrators").Err()
	case is:
		return pb.Acl_WRITER, nil
	}
	role := NoRole
	for _, rule := range b.Proto.Acls {
		// Check this rule if it could potentially confer a higher role.
		if rule.Role > role {
			if rule.Identity == string(id) {
				role = rule.Role
			} else if id.Kind() == identity.User && rule.Identity == id.Email() {
				role = rule.Role
			} else if is, err := auth.IsMember(ctx, rule.Group); err != nil {
				// Empty group membership checks always return false without
				// any error so it doesn't matter if the group is unspecified.
				return NoRole, errors.Annotate(err, "failed to check group membership in %q", rule.Group).Err()
			} else if is {
				role = rule.Role
			}
		}
	}
	return role, nil
}
