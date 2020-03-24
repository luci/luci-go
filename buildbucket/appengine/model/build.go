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
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// PubSubCallback encapsulates parameters for a Pub/Sub callback.
type PubSubCallback struct {
	AuthToken string `gae:"auth_token,noindex"`
	Topic     string `gae:"topic,noindex"`
	UserData  string `gae:"user_data,noindex"`
}

// Build is a representation of a build in the datastore.
type Build struct {
	_kind string `gae:"$kind,Build"`
	ID    int64  `gae:"$id"`

	// LegacyProperties are properties set for v1 legacy builds.
	LegacyProperties

	// Proto is the pb.Build proto representation of the build.
	//
	// infra, input.properties, output.properties, and steps
	// are zeroed and stored in separate datastore entities
	// due to their potentially large size.
	// tags are given their own field so they can be indexed.
	//
	// noindex is not respected here, it's set in pb.Build.ToProperty.
	Proto pb.Build `gae:"proto,noindex"`

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
	Experimental        bool      `gae:"experimental"`
	Incomplete          bool      `gae:"incomplete"`
	IsLuci              bool      `gae:"is_luci"`
	ResultDBUpdateToken string    `gae:"resultdb_update_token,noindex"`
	Status              pb.Status `gae:"status_v2"`
	StatusChangedTime   time.Time `gae:"status_changed_time"`
	// Tags is a slice of "<key>:<value>" strings taken from Proto.Tags.
	// Stored separately in order to index.
	Tags []string `gae:"tags"`

	// PubSubCallback, if set, creates notifications for build status changes.
	PubSubCallback PubSubCallback `gae:"pubsub_callback,noindex"`
}

// ToProto returns the *pb.Build representation of this build.
// TODO(crbug/1042991): Support field masks.
func (b *Build) ToProto(ctx context.Context) (*pb.Build, error) {
	p := proto.Clone(&b.Proto).(*pb.Build)
	for _, t := range b.Tags {
		k, v := strpair.Parse(t)
		p.Tags = append(p.Tags, &pb.StringPair{
			Key:   k,
			Value: v,
		})
	}
	key := datastore.KeyForObj(ctx, b)
	inf := &BuildInfra{
		ID:    1,
		Build: key,
	}
	inp := &BuildInputProperties{
		ID:    1,
		Build: key,
	}
	out := &BuildOutputProperties{
		ID:    1,
		Build: key,
	}
	if err := datastore.Get(ctx, inf, inp, out); err != nil {
		// Output properties won't be found unless the build has finished.
		if merr, ok := err.(errors.MultiError); !ok || merr[0] != nil || merr[1] != nil || (merr[2] != nil && merr[2] != datastore.ErrNoSuchEntity) {
			return nil, errors.Annotate(err, "error fetching build details for %q", key).Err()
		}
	}
	p.Infra = &inf.Proto
	if p.Input == nil {
		p.Input = &pb.Build_Input{}
	}
	p.Input.Properties = &inp.Proto.Struct
	if p.Output == nil {
		p.Output = &pb.Build_Output{}
	}
	p.Output.Properties = &out.Proto.Struct
	// TODO(crbug/1042991): Add steps.
	return p, nil
}
