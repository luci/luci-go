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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSetErrorOnBuild(t *testing.T) {
	t.Parallel()
	ftt.Run(`setErrorOnBuild`, t, func(t *ftt.Test) {
		t.Run(`basic`, func(t *ftt.Test) {
			build := &bbpb.Build{
				Output: &bbpb.Build_Output{},
			}
			setErrorOnBuild(build, errors.New("hi"))
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{
				SummaryMarkdown: "\n\nError in build protocol: hi",
				Status:          bbpb.Status_INFRA_FAILURE,
				Output: &bbpb.Build_Output{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "\n\nError in build protocol: hi",
				},
			}))
		})

		t.Run(`truncated message`, func(t *ftt.Test) {
			build := &bbpb.Build{
				SummaryMarkdown: strings.Repeat("16 test pattern\n", 256),
				Output:          &bbpb.Build_Output{},
			}
			setErrorOnBuild(build, errors.New("hi"))
			assert.Loosely(t, build.SummaryMarkdown, should.HaveLength(4096))
			assert.Loosely(t, build.SummaryMarkdown, should.HaveSuffix("...\n\nError in build protocol: hi"))
			assert.Loosely(t, build.Output.SummaryMarkdown, should.Equal(build.SummaryMarkdown))
			build.SummaryMarkdown = ""
			build.Output.SummaryMarkdown = ""
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{
				Status: bbpb.Status_INFRA_FAILURE,
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_INFRA_FAILURE,
				},
			}))
		})
	})
}

func TestProcessFinalBuild(t *testing.T) {
	t.Parallel()
	ftt.Run(`processFinalBuild`, t, func(t *ftt.Test) {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`empty`, func(t *ftt.Test) {
			build := &bbpb.Build{
				Output: &bbpb.Build_Output{},
			}
			processFinalBuild(now, build)
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{
				SummaryMarkdown: ("\n\nError in build protocol: " +
					"Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."),
				UpdateTime: now,
				EndTime:    now,
				Status:     bbpb.Status_INFRA_FAILURE,
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: ("\n\nError in build protocol: " +
						"Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."),
				},
			}))
		})

		t.Run(`success`, func(t *ftt.Test) {
			build := &bbpb.Build{
				Status: bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS},
				},
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_SUCCESS,
				},
			}
			processFinalBuild(now, build)
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{
				UpdateTime: now,
				EndTime:    now,
				Status:     bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS, EndTime: now},
				},
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_SUCCESS,
				},
			}))
		})

		t.Run(`incomplete step`, func(t *ftt.Test) {
			build := &bbpb.Build{
				Status: bbpb.Status_SUCCESS,
				Steps: []*bbpb.Step{
					{Status: bbpb.Status_SUCCESS},
					{SummaryMarkdown: "hi"},
				},
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_SUCCESS,
				},
			}
			processFinalBuild(now, build)
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{
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
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_SUCCESS,
				},
			}))
		})
	})
}

func TestUpdateStepFromBuild(t *testing.T) {
	ftt.Run(`updateStepFromBuild`, t, func(t *ftt.Test) {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`basic`, func(t *ftt.Test) {
			step := &bbpb.Step{
				Logs: []*bbpb.Log{{Name: "something"}},
			}
			build := &bbpb.Build{
				SummaryMarkdown: "hi",
				Status:          bbpb.Status_FAILURE,
				EndTime:         now,
				Output: &bbpb.Build_Output{
					Logs:   []*bbpb.Log{{Name: "other"}},
					Status: bbpb.Status_FAILURE,
				},
			}
			updateStepFromBuild(step, build)
			assert.Loosely(t, step, should.Resemble(&bbpb.Step{
				SummaryMarkdown: "hi",
				Status:          bbpb.Status_FAILURE,
				EndTime:         now,
				Logs: []*bbpb.Log{
					{Name: "something"},
					{Name: "other"},
				},
			}))
		})
		t.Run(`step status terminal`, func(t *ftt.Test) {
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
					Logs:   []*bbpb.Log{{Name: "build other"}},
					Status: bbpb.Status_FAILURE,
				},
			}
			updateStepFromBuild(step, build)
			assert.Loosely(t, step, should.Resemble(&bbpb.Step{
				SummaryMarkdown: "hi step",
				Status:          bbpb.Status_INFRA_FAILURE,
				EndTime:         now,
				Logs: []*bbpb.Log{
					{Name: "step something"},
				},
			}))
		})
	})
}

func TestUpdateBaseFromUserbuild(t *testing.T) {
	ftt.Run(`updateBaseFromUserBuild`, t, func(t *ftt.Test) {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`basic`, func(t *ftt.Test) {
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
					Logs:   []*bbpb.Log{{Name: "other"}},
					Status: bbpb.Status_CANCELED,
					StatusDetails: &bbpb.StatusDetails{
						Timeout: &bbpb.StatusDetails_Timeout{},
					},
				},
			}
			buildClone := proto.Clone(build).(*bbpb.Build)
			buildClone.Steps = append(buildClone.Steps, &bbpb.Step{Name: "sup"})
			updateBaseFromUserBuild(base, build)
			assert.Loosely(t, base, should.Resemble(buildClone))
		})

		t.Run(`nil build`, func(t *ftt.Test) {
			base := &bbpb.Build{
				Steps:           []*bbpb.Step{{Name: "sup"}},
				SummaryMarkdown: "hi",
			}
			baseClone := proto.Clone(base).(*bbpb.Build)
			updateBaseFromUserBuild(base, nil)
			assert.Loosely(t, base, should.Resemble(baseClone))
		})

		t.Run(`output is merged`, func(t *ftt.Test) {
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
			assert.Loosely(t, base, should.Resemble(&bbpb.Build{
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "hello"},
						{Name: "world"},
					},
				},
			}))
		})
	})
}

func TestUpdateBuildFromGlobalSubBuild(t *testing.T) {
	ftt.Run(`TestUpdateBuildFromGlobalSubBuild`, t, func(t *ftt.Test) {
		base := &bbpb.Build{}
		sub := &bbpb.Build{}

		t.Run(`empty parent`, func(t *ftt.Test) {
			t.Run(`empty child`, func(t *ftt.Test) {
				updateOutputProperties(base, sub, []string{""})
				assert.Loosely(t, base, should.Resemble(&bbpb.Build{}))
			})

			t.Run(`properties`, func(t *ftt.Test) {
				s, err := structpb.NewStruct(map[string]any{
					"hello": "world",
					"this":  100,
				})
				assert.Loosely(t, err, should.BeNil)
				sub.Output = &bbpb.Build_Output{Properties: s}
				updateOutputProperties(base, sub, []string{""})
				m := base.Output.Properties.AsMap()
				assert.Loosely(t, m, should.Resemble(map[string]any{
					"hello": "world",
					"this":  100.0, // because JSON semantics
				}))
			})
		})

		t.Run(`populated parent`, func(t *ftt.Test) {
			s, err := structpb.NewStruct(map[string]any{
				"hello": "world",
				"this":  100,
			})
			assert.Loosely(t, err, should.BeNil)
			base.Output = &bbpb.Build_Output{
				Properties: s,
			}

			t.Run(`empty child`, func(t *ftt.Test) {
				updateOutputProperties(base, sub, []string{""})
				assert.Loosely(t, base, should.Resemble(&bbpb.Build{
					Output: &bbpb.Build_Output{
						Properties: s,
					},
				}))
			})

			t.Run(`properties`, func(t *ftt.Test) {
				sSub, err := structpb.NewStruct(map[string]any{
					"newkey": "yes",
					"hello":  "replacement",
				})
				assert.Loosely(t, err, should.BeNil)
				sub.Output = &bbpb.Build_Output{Properties: sSub}
				updateOutputProperties(base, sub, []string{""})

				sNew, err := structpb.NewStruct(map[string]any{
					"hello":  "replacement",
					"this":   100,
					"newkey": "yes",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, base, should.Resemble(&bbpb.Build{
					Output: &bbpb.Build_Output{
						Properties: sNew,
					},
				}))
			})

		})

		t.Run(`deep path`, func(t *ftt.Test) {
			s, err := structpb.NewStruct(map[string]any{
				"hello": "world",
				"this":  100,
			})
			assert.Loosely(t, err, should.BeNil)
			base.Output = &bbpb.Build_Output{
				Properties: s,
			}
			sub.Output = &bbpb.Build_Output{
				Properties: proto.Clone(s).(*structpb.Struct),
			}

			sNew, err := structpb.NewStruct(map[string]any{
				"hello": "world",
				"this":  100,
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"hello": "world",
							"this":  100,
						},
					},
				},
			})

			updateOutputProperties(base, sub, []string{"a", "b", "c"})
			assert.Loosely(t, base, should.Resemble(&bbpb.Build{
				Output: &bbpb.Build_Output{
					Properties: sNew,
				},
			}))
		})
	})
}
