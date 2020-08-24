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

package exe

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/luciexe/exe/build"
)

func mkRawRun(mainRaw MainRawFn) runMiddle {
	return func(ctx context.Context, cfg *config, initial *bbpb.Build, userArgs []string, sendFn func(*bbpb.Build) error) (lastBuild *bbpb.Build, recovered interface{}, err error) {
		lastBuild = initial

		var wrappedSender func()
		if sendFn == nil {
			wrappedSender = func() {}
		} else {
			wrappedSender = func() { sendFn(initial) }
		}

		defer func() {
			if !protoutil.IsEnded(lastBuild.Status) {
				// can't use switch and assign recovered
				if recovered = recover(); recovered != nil {
					err = build.ErrCallbackPaniced
				}
				lastBuild.Status, lastBuild.StatusDetails = build.GetErrorStatus(err)
				lastBuild.EndTime = google.NewTimestamp(clock.Now(ctx))
				wrappedSender()
			}
		}()

		cCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		signals.HandleInterrupt(cancel)

		err = mainRaw(cCtx, initial, userArgs, wrappedSender)
		return
	}
}
