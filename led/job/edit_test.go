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

package job

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	api "go.chromium.org/luci/swarming/proto/api"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

func TestClearCurrentIsolated(t *testing.T) {
	t.Parallel()

	runCases(t, `ClearCurrentIsolated`, []testCase{
		{
			name: "basic",
			fn: func(jd *Definition) {
				jd.UserPayload = &api.CASTree{
					Digest:    "deadbeef",
					Namespace: "namespace",
					Server:    "server",
				}
				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					if slc.Properties == nil {
						slc.Properties = &api.TaskProperties{}
					}
					slc.Properties.CasInputs = &api.CASTree{
						Digest:    "deadbeef",
						Namespace: "namespace",
						Server:    "server",
					}
				}
				SoEdit(jd, func(je Editor) {
					je.ClearCurrentIsolated()
				})

				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					So(slc.Properties.CasInputs, ShouldBeNil)
				}

				iso, err := jd.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(iso, ShouldResemble, &swarmingpb.CASTree{
					Namespace: "namespace",
					Server:    "server",
				})

			},
		},
	})
}
