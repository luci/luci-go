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
	"fmt"
	"math/rand"
	"strings"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	modeldefs "go.chromium.org/luci/buildbucket/appengine/model/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// BuilderKind is the kind of the Builder entity.
const BuilderKind = "Bucket.Builder"

// BuilderStatKind is the kind of the BuilderStat entity.
const BuilderStatKind = "Builder"

// BuilderExpirationDuration is the maximum duration a builder can go without
// having a build scheduled before its BuilderStat may be deleted.
const BuilderExpirationDuration = 4 * 7 * 24 * time.Hour // 4 weeks

// BuilderStatZombieDuration is the maximum duration for which a zombie
// BuilderStat that can exist without having a build scheduled before it may be
// deleted.
//
// Zombie BuilderStat is a BuilderStat entity of which Builder entity doesn't
// exist.
const BuilderStatZombieDuration = 6 * time.Hour

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
	Config *pb.BuilderConfig `gae:"config,legacy"`

	// ConfigHash is used for fast deduplication of configs.
	ConfigHash string `gae:"config_hash"`

	// Metadata is the builder owner and health information.
	Metadata *pb.BuilderMetadata `gae:"builder_metadata,legacy"`
}

// FullBuilderName return the builder name in the format of "<project>.<bucket>.<builder>".
func (b *Builder) FullBuilderName() string {
	return fmt.Sprintf("%s/%s/%s", b.Parent.Parent().StringID(), b.Parent.StringID(), b.ID)
}

// BuilderKey returns a datastore key of a builder.
func BuilderKey(ctx context.Context, project, bucket, builder string) *datastore.Key {
	return datastore.KeyForObj(ctx, &Builder{
		ID:     builder,
		Parent: BucketKey(ctx, project, bucket),
	})
}

// BuilderStat represents a builder Datastore entity which is used internally for metrics.
//
// The builder will be registered automatically by scheduling a build,
// and unregistered automatically by not scheduling builds for BuilderExpirationDuration.
//
// Note: due to the historical reason, the entity kind is Builder.
type BuilderStat struct {
	_kind string `gae:"$kind,Builder"`

	// ID is a string with format "{project}:{bucket}:{builder}".
	ID string `gae:"$id"`

	// LastScheduled is the last time we received a valid build scheduling request
	// for this builder. Probabilistically update when scheduling a build.
	LastScheduled time.Time `gae:"last_scheduled,noindex"`
}

// BuilderKey returns a datastore key for the Builder that a given BuilderStat
// references.
//
// Panics if the ID of the BuilderStat is invalid.
func (s *BuilderStat) BuilderKey(ctx context.Context) *datastore.Key {
	parts := strings.Split(s.ID, ":")
	if len(parts) != 3 {
		panic(fmt.Errorf("invalid BuilderStatID: %s", s.ID))
	}
	return BuilderKey(ctx, parts[0], parts[1], parts[2])
}

// BuilderStatKey returns a datastore key for a given Builder.
func BuilderStatKey(ctx context.Context, project, bucket, builder string) *datastore.Key {
	return datastore.KeyForObj(ctx, &BuilderStat{
		ID: fmt.Sprintf("%s:%s:%s", project, bucket, builder),
	})
}

// UpdateBuilderStat updates or creates datastore BuilderStat entities.
func UpdateBuilderStat(ctx context.Context, builds []*Build, scheduledTime time.Time) error {
	seen := stringset.New(len(builds))
	builderStats := make([]*BuilderStat, 0, len(builds))
	for _, b := range builds {
		if b.Proto.Builder == nil {
			panic("Build.Proto.Builder isn't initialized")
		}
		id := fmt.Sprintf("%s:%s:%s", b.Proto.Builder.Project, b.Proto.Builder.Bucket, b.Proto.Builder.Builder)
		if seen.Add(id) {
			builderStats = append(builderStats, &BuilderStat{
				ID: id,
			})
		}
	}

	if err := GetIgnoreMissing(ctx, builderStats); err != nil {
		return errors.Annotate(err, "error fetching BuilderStat").Err()
	}

	var toPut []*BuilderStat
	for _, s := range builderStats {
		if s.LastScheduled.IsZero() {
			s.LastScheduled = scheduledTime
			toPut = append(toPut, s)
		} else {
			// Probabilistically update BuilderStat entities to avoid high contention.
			// The longer an entity isn't updated, the greater its probability.
			sinceLastUpdate := scheduledTime.Sub(s.LastScheduled)
			updateProbability := sinceLastUpdate.Seconds() / 3600.0
			if rand.Float64() < updateProbability {
				s.LastScheduled = scheduledTime
				toPut = append(toPut, s)
			}
		}
	}
	if len(toPut) == 0 {
		return nil
	}
	if err := datastore.Put(ctx, toPut); err != nil {
		return errors.Annotate(err, "error putting BuilderStat").Err()
	}
	return nil
}

// CustomBuilderMetrics is a Datastore entity that stores custom builder metrics
// and builders report to them.
type CustomBuilderMetrics struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is CustomBuilderMetricsKey.
	Key *datastore.Key `gae:"$key"`
	// LastUpdate is when this entity changed the last time.
	LastUpdate time.Time `gae:",noindex"`

	Metrics *modeldefs.CustomBuilderMetrics
}

// CustomBuilderMetricsKey is CustomBuilderMetrics entity key.
func CustomBuilderMetricsKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "CustomBuilderMetrics", "", 1, nil)
}

// BuilderQueue is a Datastore entity that stores the builder's current workload.
// It is used in conjunction with the max_concurrent_builds field defined
// in lucicfg builder definition.
// This entity only exists for builders with the max_concurrent_builds
// feature enabled.
type BuilderQueue struct {
	_     datastore.PropertyMap `gae:"-,extra"`
	_kind string                `gae:"$kind,BuilderQueue"`

	// ID is a string representation of the Bucket.Builder entity key,
	// e.g. "angle/ci/linux-test".
	// Only one BuilderQueue exists per Builder.
	ID string `gae:"$id"`

	// TriggeredBuilds represents the set of builds dispatched to task backend
	// but not yet terminated (i.e. builds in Cloud Task queue
	// + builds with created Backend tasks
	// + the builds actually running).
	TriggeredBuilds []int64 `gae:"triggered_builds,noindex"`

	// PendingBuilds represents the queue of builds not yet triggered.
	// (i.e. builds that were scheduled but not yet dispatched)
	PendingBuilds []int64 `gae:"pending_builds,noindex"`
}
