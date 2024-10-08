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

	"go.chromium.org/luci/common/tsmon/target"

	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
)

var serviceInfoCtxKey = "holds the service information"

type serviceInfo struct {
	service  string
	job      string
	instance string
}

// WithBuilder returns a context for a Builder target with the given info.
//
// WithServiceInfo must be called once before this function. Otherwise, this panics.
func WithBuilder(ctx context.Context, project, bucket, builder string) context.Context {
	val := ctx.Value(&serviceInfoCtxKey)
	if val == nil {
		panic("missing service info; forgot to call metrics.WithServiceInfo()?")
	}
	info := val.(*serviceInfo)

	return target.Set(ctx, &bbmetrics.BuilderTarget{
		Project: project,
		Bucket:  bucket,
		Builder: builder,

		ServiceName: info.service,
		JobName:     info.job,
		InstanceID:  info.instance,
	})
}

// WithServiceInfo sets the service info to be set in the Builder target.
//
// Call this in the initialization of a server process.
func WithServiceInfo(ctx context.Context, service, job, instance string) context.Context {
	return context.WithValue(ctx, &serviceInfoCtxKey, &serviceInfo{
		service:  service,
		job:      job,
		instance: instance,
	})
}
