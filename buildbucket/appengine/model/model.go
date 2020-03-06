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
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// Ensure BuildProto implements datastore.PropertyConverter.
var _ datastore.PropertyConverter = &BuildProto{}

// BuildProto is a wrapper around buildbucketpb.Build.
// Implements datastore.PropertyConverter.
// Allows reading from and writing to the datastore.
type BuildProto struct {
	buildbucketpb.Build
}

// FromProperty deserializes buildbucketpb.Build protos from the datastore.
// Implements datastore.PropertyConverter.
func (bp *BuildProto) FromProperty(p datastore.Property) error {
	return proto.Unmarshal(p.Value().([]byte), bp)
}

// ToProperty serializes buildbucketpb.Build protos for the datastore.
// Implements datastore.PropertyConverter.
func (bp *BuildProto) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	bytes, err := proto.Marshal(bp)
	if err != nil {
		return p, errors.Annotate(err, "failed to marshal proto").Err()
	}
	// noindex is not respected in the tags in the Build model.
	return p, p.SetValue(bytes, datastore.NoIndex)
}

// PubsubCallback encapsulates parameters for a Pub/Sub callback.
type PubSubCallback struct {
	AuthToken string `gae:"auth_token,noindex"`
	Topic     string `gae:"topic,noindex"`
	UserData  string `gae:"user_data,noindex"`
}

// Build is representation of a build in the datastore.
type Build struct {
	_kind string `gae:"$kind,Build"`
	ID    int64  `gae:"$id"`

	// LegacyProperties are properties set for v1 legacy builds.
	LegacyProperties

	// Proto is the buildbucketpb.Build proto representation of the build.
	//
	// infra and input.properties are set during build creation but are
	// later stored separately.
	// output.properties and steps are zeroed and stored in separate
	// datastore entities due to potentially large size.
	// tags are given their own field so they can be indexed.
	//
	// noindex is not respected here, it's set in BuildProto.ToProperty.
	Proto BuildProto `gae:"proto,noindex"`

	Project string `gae:"project"`
	// <project>/<bucket>. Bucket is in v2 format.
	// e.g. chromium/try (never chromium/luci.chromium.try).
	BucketID string `gae:"bucket_id"`
	// <project>/<bucket>/<builder>. Bucket is in v2 format.
	// e.g. chromium/try/linux-rel.
	BuilderID string `gae:"builder_id"`

	Canary bool `gae:"canary"`
	// TODO(crbug/1042991): Create datastore.PropertyConverter in server/auth.
	CreatedBy []byte `gae:"created_by,noindex"`
	// TODO(nodir): Replace reliance on create_time indices with id.
	CreateTime time.Time `gae:"create_time"`
	// Experimental, if true, means to exclude from monitoring and search results
	// (unless specifically requested in search results).
	Experimental bool                 `gae:"experimental"`
	Incomplete   bool                 `gae:"incomplete"`
	IsLuci       bool                 `gae:"is_luci"`
	Status       buildbucketpb.Status `gae:"status_v2"`
	// Tags is a slice of "<key>:<value>" strings taken from Proto.Tags.
	// Stored separately in order to index.
	Tags []string `gae:"tags"`

	// PubSubCallback, if set, creates notifications for build status changes.
	PubSubCallback PubSubCallback `gae:"pubsub_callback,noindex"`
}
