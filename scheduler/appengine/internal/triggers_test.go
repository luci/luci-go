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

package internal

import (
	"testing"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestToPublicTriggers(t *testing.T) {
	t.Parallel()

	Convey("ToPublicTriggers", t, func(c C) {
		gitiles := &scheduler.GitilesTrigger{}

		input := &Trigger{
			Id:    "id",
			Title: "deadbeef",
			Url:   "https://example.com",
			Payload: &Trigger_Gitiles{
				Gitiles: gitiles,
			},
		}
		expected := &scheduler.Trigger{
			Id:    "id",
			Title: "deadbeef",
			Url:   "https://example.com",
			Payload: &scheduler.Trigger_Gitiles{
				Gitiles: gitiles,
			},
		}
		actual := ToPublicTrigger(input)
		So(actual, ShouldResembleProto, expected)
	})
}
