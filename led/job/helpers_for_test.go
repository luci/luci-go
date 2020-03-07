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
