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

	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// BucketKind is a bucket entity's kind in the datastore.
	BucketKind = "BucketV2"
)

// Bucket is a representation of a bucket in the datastore.
type Bucket struct {
	_kind string `gae:"$kind,BucketV2"`
	// ID is the bucket in v2 format.
	// e.g. try (never luci.chromium.try).
	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`

	// Bucket is the bucket in v2 format.
	// e.g. try (never luci.chromium.try).
	//
	// Note: This field is "computed" in *Bucket.Save phase and exists because v1
	// APIs are allowed to search without a luci project context.
	Bucket string `gae:"bucket_name"`
	// Proto is the pb.Bucket proto representation of the bucket.
	//
	// swarming.builders is zeroed and stored in separate Builder datastore
	// entities due to potentially large size.
	Proto *pb.Bucket `gae:"config,legacy"`
	// Revision is the config revision this entity was created from.
	// TODO(crbug/1042991): Switch to noindex.
	Revision string `gae:"revision"`
	// Schema is this entity's schema version.
	// TODO(crbug/1042991): Switch to noindex.
	Schema int32 `gae:"entity_schema_version"`
	// Shadows is the list of buckets this bucket shadows.
	// It will be empty if this bucket is not a shadow bucket.
	Shadows []string `gae:"shadows,noindex"`
}

// BucketKey returns a datastore key of a bucket.
func BucketKey(ctx context.Context, project, bucket string) *datastore.Key {
	return datastore.KeyForObj(ctx, &Bucket{
		ID:     bucket,
		Parent: ProjectKey(ctx, project),
	})
}

var _ datastore.PropertyLoadSaver = (*Bucket)(nil)

// Load implements datastore.PropertyLoadSaver
func (b *Bucket) Load(p datastore.PropertyMap) error {
	return datastore.GetPLS(b).Load(p)
}

// Save implements datastore.PropertyLoadSaver. It mutates this bucket entity
// by always copying its ID value into the bucket_name field.
func (b *Bucket) Save(withMeta bool) (datastore.PropertyMap, error) {
	b.Bucket = b.ID
	return datastore.GetPLS(b).Save(withMeta)
}
