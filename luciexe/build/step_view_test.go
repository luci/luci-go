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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStepTags(t *testing.T) {
	ctx := memlogger.Use(context.Background())

	Convey(`no tags added`, t, func() {
		step := Step{
			ctx:    ctx,
			stepPb: &pb.Step{},
		}
		step.AddTagValue("", "")
		actualTags := step.stepPb.Tags

		expectedTags := []*pb.StringPair(nil)
		So(actualTags, ShouldResembleProto, expectedTags)
	})

	Convey(`Add step tags`, t, func() {
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
		So(actualTags, ShouldResembleProto, expectedTags)
	})

	Convey(`Add step tag with existing key`, t, func() {
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
		So(actualTags, ShouldResembleProto, expectedTags)
	})
}

func TestSummaryMarkdown(t *testing.T) {
	ctx := memlogger.Use(context.Background())

	Convey(`no summary markdown; one is added`, t, func() {
		step := Step{
			ctx:    ctx,
			stepPb: &pb.Step{},
		}
		step.SetSummaryMarkdown("test")
		So(step.stepPb.SummaryMarkdown, ShouldEqual, "test")
	})

	Convey(`existing summary markdown; then modified`, t, func() {
		step := Step{
			ctx: ctx,
			stepPb: &pb.Step{
				SummaryMarkdown: "test",
			},
		}
		step.SetSummaryMarkdown("some_really_cool_test_string")
		So(step.stepPb.SummaryMarkdown, ShouldEqual, "some_really_cool_test_string")
	})
}
