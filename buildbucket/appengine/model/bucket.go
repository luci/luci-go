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

// CanView returns whether the current identity can view the given bucket.
// TODO(crbug/1042991): Move elsewhere as needed.
func (b *Bucket) CanView(ctx context.Context) (bool, error) {
	id := auth.CurrentIdentity(ctx)
	// Projects can view their buckets regardless of ACLs.
	if id.Kind() == identity.Project && id.Value() == b.Parent.StringID() {
		return true, nil
	}
	grp := make([]string, len(b.Proto.Acls))
	// The lowest level of access that can be declared is READER so it's not
	// necessary to check roles. Any matching rule implies the user can view.
	for i, rule := range b.Proto.Acls {
		// Identity may be a plain email address (shorthand for user:email).
		if rule.Identity == string(id) || id.Kind() == identity.User && rule.Identity == id.Email() {
			return true, nil
		}
		// Empty group membership checks always return false
		// so it doesn't matter if the group is unspecified.
		grp[i] = rule.Group
	}
	is, err := auth.IsMember(ctx, grp...)
	if err != nil {
		return false, errors.Annotate(err, "failed to check group membership in one of %q", grp).Err()
	}
	return is, nil
}
