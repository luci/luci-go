// Copyright 2021 The LUCI Authors.
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

package metrics

import (
	"context"
	"hash/fnv"
	"reflect"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"
	tsmonpb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
)

// Builder is a metric target that represents a LUCI Builder.
type Builder struct {
	// Project is the LUCI project of the Builder.
	Project string
	// Bucket is the bucket name of the Builder.
	Bucket string
	// Builder is the name of the Builder.
	Builder string

	// ServiceName is the Cloud project ID of the Buildbucket service.
	ServiceName string
	// JobName is the Cloud service module ID of the Buildbucket service.
	JobName string
	// InstanceID is the ID of the worker instance that reported the Builder
	// to metrics.
	InstanceID string
}

// Clone returns a deep copy.
func (b *Builder) Clone() types.Target {
	clone := *b
	return &clone
}

// Type returns the metric type identification.
func (b *Builder) Type() types.TargetType {
	return types.TargetType{Name: "buildbucket.Builder", Type: reflect.TypeOf(&Builder{})}
}

// Hash computes a hash of the Builder object.
func (b *Builder) Hash() uint64 {
	h := fnv.New64a()
	h.Write([]byte(b.Project))
	h.Write([]byte(b.Bucket))
	h.Write([]byte(b.Builder))
	h.Write([]byte(b.ServiceName))
	h.Write([]byte(b.JobName))
	h.Write([]byte(b.InstanceID))
	return h.Sum64()
}

// PopulateProto populates root labels into the proto for the target fields.
func (b *Builder) PopulateProto(d *tsmonpb.MetricsCollection) {
	d.RootLabels = []*tsmonpb.MetricsCollection_RootLabels{
		target.RootLabel("project", b.Project),
		target.RootLabel("bucket", b.Bucket),
		target.RootLabel("builder", b.Builder),

		target.RootLabel("service_name", b.ServiceName),
		target.RootLabel("job_name", b.JobName),
		target.RootLabel("instance_id", b.InstanceID),
	}
}

// ReportBuilderMetrics computes and reports Builder metrics.
func ReportBuilderMetrics(ctx context.Context, serviceName, jobName, insID string) error {
	// Reset the metric to stop reporting no-longer-existing builders.
	tsmon.GetState(ctx).Store().Reset(ctx, V2.BuilderPresence)

	q := datastore.NewQuery(model.BuilderKind).KeysOnly(true)
	return datastore.RunBatch(ctx, 256, q, func(bk *datastore.Key) error {
		tctx := target.Set(ctx, &Builder{
			Project: bk.Parent().Parent().StringID(),
			Bucket:  bk.Parent().StringID(),
			Builder: bk.StringID(),

			ServiceName: serviceName,
			JobName:     jobName,
			InstanceID:  insID,
		})
		V2.BuilderPresence.Set(tctx, true)
		return nil
	})
}
