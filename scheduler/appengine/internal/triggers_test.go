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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
)

func TestToPublicTriggers(t *testing.T) {
	t.Parallel()

	ftt.Run("ToPublicTriggers", t, func(c *ftt.Test) {
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
		assert.Loosely(c, actual, should.Resemble(expected))
	})
}
