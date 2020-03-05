// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
