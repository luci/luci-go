// Copyright 2017 The LUCI Authors.
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

package config

import (
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/common/proto/srcman"
)

// Builder represents the state of the last build seen from a particular
// builder in order to implement certain notification triggers (i.e. on change).
type Builder struct {
	// Parent is a datastore key to this Builders's parent (which will always
	// be a Project), effectively making it a child of a specific project.
	Parent *datastore.Key `gae:"$parent"`

	// ID is the builder's canonical ID (e.g. buildbucket/<project>/<bucket>/<name>).
	ID string `gae:"$id"`

	// Repository is the repository this builder is tracking and the repository that
	// StatusRevision is valid for.
	Repository string

	// Status is current status of the builder.
	// It is updated every time a new build has a new status and either
	//   1) the new build has a newer revision than StatusRevision, or
	//   2) the new build's revision == StatusRevision, but it has a newer
	//      creation time.
	Status buildbucketpb.Status

	// StatusBuildTime can be used to decide whether Status should be updated.
	// It is computed as the creation time of the build that caused a change
	// of Status.
	StatusBuildTime time.Time

	// StatusRevision can be used to decide whether Status should be updated.
	// It is the revision of the codebase that's associated with the build
	// that caused a change of Status.
	StatusRevision string

	// StatusSourceManifest can be used to decide whether Status should be
	// updated, and it can also be used to compute a blamelist. It is the source
	// manifest associated with the build that caused a change of Status.
	// Note: we assume here that each build has either one source manifest or
	// none.
	StatusSourceManifest *srcman.Manifest `gae:"-"`
}

// StatusUnknown is used in the LookupBuilder return value
// if builder status is unknown.
const StatusUnknown buildbucketpb.Status = -1

// NewBuilder creates a new builder from an ID, a revision, and a build.
func NewBuilder(parent *datastore.Key, builder *notifyConfig.Builder) *Builder {
	return &Builder{
		Parent:     parent,
		ID:         fmt.Sprintf("buildbucket/%s/%s/%s", parent.StringID(), builder.Bucket, builder.Name),
		Repository: builder.Repository,
		Status:     StatusUnknown,
	}
}
// Load loads a Builder's information from props.
//
// This implements PropertyLoadSaver. Load decodes the property StatusSourceManifest
// stored in the datastore which is encoded binaryproto, and decodes it into the
// struct's StatusSourceManifest field.
func (b *Builder) Load(props datastore.PropertyMap) error {
	if pdata, ok := props["StatusSourceManifest"]; ok {
		configs := pdata.Slice()
		if len(configs) != 1 {
			return fmt.Errorf("property `StatusSourceManifest` is a property slice")
		}
		configBytes, ok := configs[0].Value().([]byte)
		if !ok {
			return fmt.Errorf("expected byte array for property `StatusSourceManifest`")
		}
		var manifest srcman.Manifest
		if err := proto.Unmarshal(configBytes, &manifest); err != nil {
			return err
		}
		b.StatusSourceManifest = &manifest
		delete(props, "StatusSourceManifest")
	}
	return datastore.GetPLS(b).Load(props)
}

// Save saves a Builder's information to a property map.
//
// This implements PropertyLoadSaver. Save encodes the StatusSourceManifest
// field as binary proto and stores it in the StatusSourceManifest property.
func (b *Builder) Save(withMeta bool) (datastore.PropertyMap, error) {
	props, err := datastore.GetPLS(b).Save(withMeta)
	if err != nil {
		return nil, err
	}
	if b.StatusSourceManifest == nil {
		return props, nil
	}
	bytes, err := proto.Marshal(b.StatusSourceManifest)
	if err != nil {
		return nil, err
	}
	props["StatusSourceManifest"] = datastore.MkProperty(bytes)
	return props, nil
}
