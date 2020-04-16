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
	. "go.chromium.org/luci/common/testing/assertions"
	api "go.chromium.org/luci/swarming/proto/api"
)

func TestGetCurrentIsolated(t *testing.T) {
	t.Parallel()

	Convey(`GetCurrentIsolated`, t, func() {
		Convey(`none (bb)`, func() {
			current, err := testBBJob().Info().CurrentIsolated()
			So(err, ShouldBeNil)
			So(current, ShouldBeNil)
		})

		Convey(`UserPayload`, func() {
			jd := testBBJob()
			jd.UserPayload = &api.CASTree{Digest: "hello"}
			current, err := jd.Info().CurrentIsolated()
			So(err, ShouldBeNil)
			So(current, ShouldResemble, &api.CASTree{Digest: "hello"})
		})

		Convey(`Swarming`, func() {
			Convey(`one slice`, func() {
				jd := testSWJob()
				jd.GetSwarming().Task = &api.TaskRequest{
					TaskSlices: []*api.TaskSlice{
						{
							Properties: &api.TaskProperties{CasInputs: &api.CASTree{
								Digest: "hello",
							}},
						},
					},
				}
				current, err := jd.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(current, ShouldResemble, &api.CASTree{Digest: "hello"})
			})

			Convey(`slice+UserPayload (match)`, func() {
				jd := testSWJob()
				jd.UserPayload = &api.CASTree{Digest: "hello"}
				jd.GetSwarming().Task = &api.TaskRequest{
					TaskSlices: []*api.TaskSlice{
						{
							Properties: &api.TaskProperties{CasInputs: &api.CASTree{
								Digest: "hello",
							}},
						},
					},
				}
				current, err := jd.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(current, ShouldResemble, &api.CASTree{Digest: "hello"})
			})

			Convey(`slice+UserPayload (mismatch)`, func() {
				jd := testSWJob()
				jd.UserPayload = &api.CASTree{Digest: "hello there"}
				jd.GetSwarming().Task = &api.TaskRequest{
					TaskSlices: []*api.TaskSlice{
						{
							Properties: &api.TaskProperties{CasInputs: &api.CASTree{
								Digest: "hello",
							}},
						},
					},
				}
				_, err := jd.Info().CurrentIsolated()
				So(err, ShouldErrLike, "Definition contains multiple isolateds")
			})

		})

	})

}
