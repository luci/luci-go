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

package buildmerge

import (
	"errors"
	"strings"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSetErrorOnBuild(t *testing.T) {
	t.Parallel()
	Convey(`setErrorOnBuild`, t, func() {
		Convey(`basic`, func() {
			build := &bbpb.Build{}
			setErrorOnBuild(build, errors.New("hi"))
			So(build, ShouldResembleProto, &bbpb.Build{
				SummaryMarkdown: "\n\nError in build protocol: hi",
				Status:          bbpb.Status_INFRA_FAILURE,
			})
		})

		Convey(`truncated message`, func() {
			build := &bbpb.Build{
				SummaryMarkdown: strings.Repeat("16 test pattern\n", 256),
			}
			setErrorOnBuild(build, errors.New("hi"))
			So(build.SummaryMarkdown, ShouldHaveLength, 4096)
			So(build.SummaryMarkdown, ShouldEndWith, "...\n\nError in build protocol: hi")
			build.SummaryMarkdown = ""
			So(build, ShouldResembleProto, &bbpb.Build{
				Status: bbpb.Status_INFRA_FAILURE,
			})
		})
	})
}

func TestProcessFinalBuild(t *testing.T) {
	t.Parallel()
	Convey(`processFinalBuild`, t, func() {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		So(err, ShouldBeNil)

		Convey(`empty`, func() {
			build := &bbpb.Build{}
			processFinalBuild(now, build)
			So(build, ShouldResembleProto, &bbpb.Build{
				SummaryMarkdown: ("\n\nError in build protocol: " +
					"Expected a terminal build status, got STATUS_UNSPECIFIED."),
				UpdateTime: now,
				EndTime:    now,
				Status:     bbpb.Status_INFRA_FAILURE,
			})
		})

		Convey(`success`, func() {
			build := &bbpb.Build{
				Status: bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS},
				},
			}
			processFinalBuild(now, build)
			So(build, ShouldResembleProto, &bbpb.Build{
				UpdateTime: now,
				EndTime:    now,
				Status:     bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS, EndTime: now},
				},
			})
		})

		Convey(`incomplete step`, func() {
			build := &bbpb.Build{
				Status: bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS},
					{SummaryMarkdown: "hi"},
				},
			}
			processFinalBuild(now, build)
			So(build, ShouldResembleProto, &bbpb.Build{
				UpdateTime: now,
				EndTime:    now,
				Status:     bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS, EndTime: now},
					{
						Status:          bbpb.Status_CANCELED,
						EndTime:         now,
						SummaryMarkdown: "hi\nstep was never finalized; did the build crash?",
					},
				},
			})
		})
	})
}

func TestUpdateStepFromBuild(t *testing.T) {
	Convey(`updateStepFromBuild`, t, func() {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		So(err, ShouldBeNil)

		Convey(`basic`, func() {
			step := &bbpb.Step{
				Logs: []*bbpb.Log{{Name: "something"}},
			}
			build := &bbpb.Build{
				SummaryMarkdown: "hi",
				Status:          bbpb.Status_FAILURE,
				EndTime:         now,
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{{Name: "other"}},
				},
			}
			updateStepFromBuild(step, build)
			So(step, ShouldResembleProto, &bbpb.Step{
				SummaryMarkdown: "hi",
				Status:          bbpb.Status_FAILURE,
				EndTime:         now,
				Logs: []*bbpb.Log{
					{Name: "something"},
					{Name: "other"},
				},
			})
		})
		Convey(`step status terminal`, func() {
			step := &bbpb.Step{
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: "hi step",
				EndTime:         now,
				Logs:            []*bbpb.Log{{Name: "step something"}},
			}
			build := &bbpb.Build{
				SummaryMarkdown: "hi sub build",
				Status:          bbpb.Status_FAILURE,
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{{Name: "build other"}},
				},
			}
			updateStepFromBuild(step, build)
			So(step, ShouldResembleProto, &bbpb.Step{
				SummaryMarkdown: "hi step",
				Status:          bbpb.Status_INFRA_FAILURE,
				EndTime:         now,
				Logs: []*bbpb.Log{
					{Name: "step something"},
				},
			})
		})
	})
}

func TestUpdateBaseFromUserbuild(t *testing.T) {
	Convey(`updateBaseFromUserBuild`, t, func() {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		So(err, ShouldBeNil)

		Convey(`basic`, func() {
			base := &bbpb.Build{
				Steps: []*bbpb.Step{{Name: "sup"}},
			}
			build := &bbpb.Build{
				SummaryMarkdown: "hi",
				Status:          bbpb.Status_CANCELED,
				StatusDetails: &bbpb.StatusDetails{
					Timeout: &bbpb.StatusDetails_Timeout{},
				},
				UpdateTime: now,
				EndTime:    now,
				Tags: []*bbpb.StringPair{
					{Key: "hi", Value: "there"},
				},
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{{Name: "other"}},
				},
			}
			buildClone := proto.Clone(build).(*bbpb.Build)
			buildClone.Steps = append(buildClone.Steps, &bbpb.Step{Name: "sup"})
			updateBaseFromUserBuild(base, build)
			So(base, ShouldResembleProto, buildClone)
		})

		Convey(`nil build`, func() {
			base := &bbpb.Build{
				Steps:           []*bbpb.Step{{Name: "sup"}},
				SummaryMarkdown: "hi",
			}
			baseClone := proto.Clone(base).(*bbpb.Build)
			updateBaseFromUserBuild(base, nil)
			So(base, ShouldResembleProto, baseClone)
		})

		Convey(`output is merged`, func() {
			base := &bbpb.Build{
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "hello"},
					},
				},
			}
			updateBaseFromUserBuild(base, &bbpb.Build{
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "world"},
					},
				},
			})
			So(base, ShouldResembleProto, &bbpb.Build{
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "hello"},
						{Name: "world"},
					},
				},
			})
		})
	})
}

func TestUpdateBuildFromGlobalSubBuild(t *testing.T) {
	Convey(`TestUpdateBuildFromGlobalSubBuild`, t, func() {
		base := &bbpb.Build{}
		sub := &bbpb.Build{}

		Convey(`empty parent`, func() {
			Convey(`empty child`, func() {
				updateBuildFromGlobalSubBuild(base, sub)
				So(base, ShouldResembleProto, &bbpb.Build{})
			})

			Convey(`properties`, func() {
				s, err := structpb.NewStruct(map[string]interface{}{
					"hello": "world",
					"this":  100,
				})
				So(err, ShouldBeNil)
				sub.Output = &bbpb.Build_Output{Properties: s}
				updateBuildFromGlobalSubBuild(base, sub)
				m := base.Output.Properties.AsMap()
				So(m, ShouldResemble, map[string]interface{}{
					"hello": "world",
					"this":  100.0, // because JSON semantics
				})
			})
		})

		Convey(`populated parent`, func() {
			s, err := structpb.NewStruct(map[string]interface{}{
				"hello": "world",
				"this":  100,
			})
			So(err, ShouldBeNil)
			base.Output = &bbpb.Build_Output{
				Properties: s,
			}

			Convey(`empty child`, func() {
				updateBuildFromGlobalSubBuild(base, sub)
				So(base, ShouldResembleProto, &bbpb.Build{
					Output: &bbpb.Build_Output{
						Properties: s,
					},
				})
			})

			Convey(`properties`, func() {
				sSub, err := structpb.NewStruct(map[string]interface{}{
					"newkey": "yes",
					"hello":  "replacement",
				})
				So(err, ShouldBeNil)
				sub.Output = &bbpb.Build_Output{Properties: sSub}
				updateBuildFromGlobalSubBuild(base, sub)

				sNew, err := structpb.NewStruct(map[string]interface{}{
					"hello":  "replacement",
					"this":   100,
					"newkey": "yes",
				})
				So(err, ShouldBeNil)
				So(base, ShouldResembleProto, &bbpb.Build{
					Output: &bbpb.Build_Output{
						Properties: sNew,
					},
				})
			})

		})
	})
}
