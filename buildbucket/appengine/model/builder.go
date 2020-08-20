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

// BuilderKind is the kind of the Builder entity.
const BuilderKind = "Bucket.Builder"

// Builder is a Datastore entity that stores builder configuration.
// It is a child of Bucket entity.
//
// Builder entities are updated together with their parents, in a cron job.
type Builder struct {
	_kind string `gae:"$kind,Bucket.Builder"`

	// ID is the builder name, e.g. "linux-rel".
	ID string `gae:"$id"`

	// Parent is the key of the parent Bucket.
	Parent *datastore.Key `gae:"$parent"`

	// Config is the builder configuration feched from luci-config.
	Config pb.Builder `gae:"config"`

	// ConfigHash is used for fast deduplication of configs.
	ConfigHash string `gae:"config_hash"`
}

// BuilderKey returns a datastore key of a builder.
func BuilderKey(ctx context.Context, project, bucket, builder string) *datastore.Key {
	return datastore.KeyForObj(ctx, &Builder{
		ID:     builder,
		Parent: BucketKey(ctx, project, bucket),
	})
}
