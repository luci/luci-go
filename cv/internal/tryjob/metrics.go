// Copyright 2022 The LUCI Authors.
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

package tryjob

import (
	"context"

	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/target"

	"go.chromium.org/luci/cv/internal/common"
)

// RunWithBuilderMetricsTarget executes `cb` under the context of a Builder
// metrics target.
//
// It is normally used to report metrics using Builder Target.
func RunWithBuilderMetricsTarget(ctx context.Context, env *common.Env, def *Definition, cb func(context.Context)) {
	if def.GetBuildbucket() == nil {
		panic(errors.Fmt("only Buildbucket backend is supported. Got %T", def.GetBackend()))
	}
	builder := def.GetBuildbucket().GetBuilder()
	cctx := target.Set(ctx, &bbmetrics.BuilderTarget{
		Project: builder.GetProject(),
		Bucket:  builder.GetBucket(),
		Builder: builder.GetBuilder(),

		ServiceName: env.GAEInfo.CloudProject,
		JobName:     env.GAEInfo.ServiceName,
		InstanceID:  env.GAEInfo.InstanceID,
	})
	cb(cctx)
}
