// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

func TestTaskName(t *testing.T) {
	t.Parallel()

	Convey(`TaskName`, t, func() {
		Convey(`swarming`, func() {
			jd := testSWJob()
			jd.GetSwarming().Task = &api.TaskRequest{Name: "hello"}
			So(jd.Info().TaskName(), ShouldEqual, "hello")
		})

		Convey(`buildbucket`, func() {
			jd := testBBJob()
			jd.GetBuildbucket().Name = "hello"
			So(jd.Info().TaskName(), ShouldEqual, "hello")
		})
	})
}
