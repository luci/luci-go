// Copyright 2019 The LUCI Authors.
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

package exe

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/luciexe/exe/build"
)

func mkRun(main MainFn) runMiddle {
	return func(ctx context.Context, cfg *config, initial *bbpb.Build, userArgs []string, sendFn func(*bbpb.Build) error) (*bbpb.Build, interface{}, error) {
		return build.Sink{
			InitialBuild: initial,
			LogdogClient: cfg.ldClient,
			SendLimit:    cfg.lim,
			SendFunc:     sendFn,
		}.Use(ctx, func(ctx context.Context, state *build.State) error {
			cCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			signals.HandleInterrupt(cancel)

			return main(cCtx, state, userArgs)
		})
	}
}
