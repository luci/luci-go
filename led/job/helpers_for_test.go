// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"time"

	"github.com/golang/protobuf/ptypes"
	api "go.chromium.org/luci/swarming/proto/api"
)

func testBBJob() *Definition {
	return &Definition{JobType: &Definition_Buildbucket{
		Buildbucket: &Buildbucket{},
	}}
}

func testSWJob(sliceExps ...time.Duration) *Definition {
	ret := &Definition{JobType: &Definition_Swarming{
		Swarming: &Swarming{},
	}}
	if len(sliceExps) > 0 {
		sw := ret.GetSwarming()
		sw.Task = &api.TaskRequest{}

		for _, exp := range sliceExps {
			sw.Task.TaskSlices = append(sw.Task.TaskSlices, &api.TaskSlice{
				Expiration: ptypes.DurationProto(exp),
			})
		}
	}
	return ret
}
