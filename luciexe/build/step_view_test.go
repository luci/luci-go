// Copyright 2022 The LUCI Authors.
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

package build

import (
	"context"
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStepTags(t *testing.T) {
	ctx := memlogger.Use(context.Background())

	ftt.Run(`no tags added`, t, func(t *ftt.Test) {
		step := Step{
			ctx:    ctx,
			stepPb: &pb.Step{},
		}
		step.AddTagValue("", "")
		actualTags := step.stepPb.Tags

		expectedTags := []*pb.StringPair(nil)
		assert.Loosely(t, actualTags, should.Resemble(expectedTags))
	})

	ftt.Run(`Add step tags`, t, func(t *ftt.Test) {
		step := Step{
			ctx:    ctx,
			stepPb: &pb.Step{},
		}
		step.AddTagValue("build.testing.key", "value")
		actualTags := step.stepPb.Tags

		expectedTags := []*pb.StringPair{}
		expectedTags = append(expectedTags, &pb.StringPair{
			Key:   "build.testing.key",
			Value: "value",
		})
		assert.Loosely(t, actualTags, should.Resemble(expectedTags))
	})

	ftt.Run(`Add step tag with existing key`, t, func(t *ftt.Test) {
		// One key may have multiple values
		tags := []*pb.StringPair{}
		tags = append(tags, &pb.StringPair{
			Key:   "build.testing.key",
			Value: "a",
		})
		tags = append(tags, &pb.StringPair{
			Key:   "build.testing.key",
			Value: "g",
		})
		step := Step{
			ctx: ctx,
			stepPb: &pb.Step{
				Tags: tags,
			},
		}
		step.AddTagValue("build.testing.key", "d")
		actualTags := step.stepPb.Tags

		expectedTags := []*pb.StringPair{}
		expectedTags = append(expectedTags, &pb.StringPair{
			Key:   "build.testing.key",
			Value: "a",
		})
		expectedTags = append(expectedTags, &pb.StringPair{
			Key:   "build.testing.key",
			Value: "d",
		})
		expectedTags = append(expectedTags, &pb.StringPair{
			Key:   "build.testing.key",
			Value: "g",
		})
		assert.Loosely(t, actualTags, should.Resemble(expectedTags))
	})
}

func TestSummaryMarkdown(t *testing.T) {
	ctx := memlogger.Use(context.Background())

	ftt.Run(`no summary markdown; one is added`, t, func(t *ftt.Test) {
		step := Step{
			ctx:    ctx,
			stepPb: &pb.Step{},
		}
		step.SetSummaryMarkdown("test")
		assert.Loosely(t, step.stepPb.SummaryMarkdown, should.Equal("test"))
	})

	ftt.Run(`existing summary markdown; then modified`, t, func(t *ftt.Test) {
		step := Step{
			ctx: ctx,
			stepPb: &pb.Step{
				SummaryMarkdown: "test",
			},
		}
		step.SetSummaryMarkdown("some_really_cool_test_string")
		assert.Loosely(t, step.stepPb.SummaryMarkdown, should.Equal("some_really_cool_test_string"))
	})
}
