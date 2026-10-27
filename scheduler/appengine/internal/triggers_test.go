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

	"google.golang.org/protobuf/types/known/structpb"

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
		assert.Loosely(c, actual, should.Match(expected))
	})
}

func TestIsTrivialTrigger(t *testing.T) {
	t.Parallel()

	bb := func(b *scheduler.BuildbucketTrigger) *Trigger {
		return &Trigger{Payload: &Trigger_Buildbucket{Buildbucket: b}}
	}
	props, err := structpb.NewStruct(map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}

	ftt.Run("IsTrivialTrigger", t, func(c *ftt.Test) {
		c.Run("no payload", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(&Trigger{}), should.BeTrue)
		})
		c.Run("noop and webui", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(&Trigger{Payload: &Trigger_Noop{Noop: &scheduler.NoopTrigger{}}}), should.BeTrue)
			assert.Loosely(c, IsTrivialTrigger(&Trigger{Payload: &Trigger_Webui{Webui: &scheduler.WebUITrigger{}}}), should.BeTrue)
		})
		c.Run("cron and gitiles", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(&Trigger{Payload: &Trigger_Cron{Cron: &scheduler.CronTrigger{}}}), should.BeFalse)
			assert.Loosely(c, IsTrivialTrigger(&Trigger{Payload: &Trigger_Gitiles{Gitiles: &scheduler.GitilesTrigger{}}}), should.BeFalse)
		})
		c.Run("empty buildbucket", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{})), should.BeTrue)
		})
		c.Run("buildbucket with allowed tags", func(c *ftt.Test) {
			trigger := bb(&scheduler.BuildbucketTrigger{Tags: []string{"user_agent:agent"}})
			assert.Loosely(c, IsTrivialTrigger(trigger), should.BeTrue)
			// Must not mutate the input.
			assert.Loosely(c, trigger.GetBuildbucket().Tags, should.Match([]string{"user_agent:agent"}))
		})
		c.Run("buildbucket with other tags", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{
				Tags: []string{"user_agent:agent", "a:b"},
			})), should.BeFalse)
		})
		c.Run("buildbucket with malformed tag", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{
				Tags: []string{"user_agent"},
			})), should.BeFalse)
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{
				Tags: []string{"user_agentx"},
			})), should.BeFalse)
		})
		c.Run("buildbucket with empty key tag", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{
				Tags: []string{":value"},
			})), should.BeFalse)
		})
		c.Run("buildbucket with differently cased key", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{
				Tags: []string{"User_Agent:agent"},
			})), should.BeFalse)
		})
		c.Run("buildbucket with properties", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{Properties: props})), should.BeFalse)
		})
		c.Run("buildbucket with properties and allowed tags", func(c *ftt.Test) {
			assert.Loosely(c, IsTrivialTrigger(bb(&scheduler.BuildbucketTrigger{
				Properties: props,
				Tags:       []string{"user_agent:agent"},
			})), should.BeFalse)
		})
	})
}
