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
	"context"
	"fmt"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/reflectutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"

	"go.chromium.org/luci/luciexe"
)

func mkDesc(name string) *logpb.LogStreamDescriptor {
	return &logpb.LogStreamDescriptor{
		Name:        name,
		StreamType:  logpb.StreamType_DATAGRAM,
		ContentType: luciexe.BuildProtoContentType,
	}
}

func TestAgent(t *testing.T) {
	t.Parallel()

	ftt.Run(`buildState`, t, func(t *ftt.Test) {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		assert.Loosely(t, err, should.BeNil)
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
		ctx, cancel := context.WithCancel(ctx)

		baseProps, err := structpb.NewStruct(map[string]any{
			"test": "value",
		})
		assert.Loosely(t, err, should.BeNil)

		base := &bbpb.Build{
			Input: &bbpb.Build_Input{
				Properties: baseProps,
			},
			Output: &bbpb.Build_Output{
				Logs: []*bbpb.Log{
					{Name: "stdout", Url: "stdout"},
				},
			},
		}
		// we omit view url here to keep tests simpler
		merger, err := New(ctx, "u/", base, func(ns, stream types.StreamName) (url, viewURL string) {
			return fmt.Sprintf("url://%s%s", ns, stream), ""
		})
		assert.Loosely(t, err, should.BeNil)
		defer merger.Close()
		defer cancel()

		getFinal := func() (lastBuild *bbpb.Build) {
			for build := range merger.MergedBuildC {
				lastBuild = build
			}
			return
		}

		t.Run(`can close without any data`, func(t *ftt.Test) {
			merger.Close()
			build := <-merger.MergedBuildC

			base.Output.Logs[0].Url = "url://u/stdout"

			assert.Loosely(t, build, should.Match(base))
		})

		t.Run(`bad stream type`, func(t *ftt.Test) {
			cb := merger.onNewStream(&logpb.LogStreamDescriptor{
				Name:        "u/build.proto",
				StreamType:  logpb.StreamType_TEXT, // should be DATAGRAM
				ContentType: luciexe.BuildProtoContentType,
			})
			assert.Loosely(t, cb, should.BeNil)
			// NOTE: here and below we do ShouldBeTrue on `ok` instead of using
			// ShouldNotBeNil on `tracker`. This is because ShouldNotBeNil is
			// currently (as of Sep'19) implemented in terms of ShouldBeNil, which
			// ends up traversing the entire `tracker` struct with `reflect`. This
			// causes the race detector to claim that we're reading the contents of
			// the atomic.Value in tracker without a lock (which is true).
			tracker, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			assert.Loosely(t, tracker.getLatestBuild(), should.Match(&bbpb.Build{
				EndTime:         now,
				UpdateTime:      now,
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: "\n\nError in build protocol: build proto stream \"u/build.proto\" has type \"TEXT\", expected \"DATAGRAM\"",
				Output: &bbpb.Build_Output{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "\n\nError in build protocol: build proto stream \"u/build.proto\" has type \"TEXT\", expected \"DATAGRAM\"",
				},
			}))
		})

		t.Run(`bad content type`, func(t *ftt.Test) {
			cb := merger.onNewStream(&logpb.LogStreamDescriptor{
				Name:        "u/build.proto",
				StreamType:  logpb.StreamType_DATAGRAM,
				ContentType: "i r bad",
			})
			assert.Loosely(t, cb, should.BeNil)
			tracker, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			assert.Loosely(t, tracker.getLatestBuild(), should.Match(&bbpb.Build{
				EndTime:         now,
				UpdateTime:      now,
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: fmt.Sprintf("\n\nError in build protocol: stream \"u/build.proto\" has content type \"i r bad\", expected one of %v", []string{luciexe.BuildProtoContentType, luciexe.BuildProtoZlibContentType}),
				Output: &bbpb.Build_Output{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: fmt.Sprintf("\n\nError in build protocol: stream \"u/build.proto\" has content type \"i r bad\", expected one of %v", []string{luciexe.BuildProtoContentType, luciexe.BuildProtoZlibContentType}),
				},
			}))
		})

		t.Run(`build.proto suffix but bad stream type and content type `, func(t *ftt.Test) {
			cb := merger.onNewStream(&logpb.LogStreamDescriptor{
				Name:        "u/build.proto",
				StreamType:  logpb.StreamType_TEXT,
				ContentType: "i r bad",
			})
			assert.Loosely(t, cb, should.BeNil)
			tracker, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			assert.Loosely(t, tracker.getLatestBuild(), should.Match(&bbpb.Build{
				EndTime:         now,
				UpdateTime:      now,
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: fmt.Sprintf("\n\nError in build protocol: build.proto stream \"u/build.proto\" has stream type \"TEXT\" and content type \"i r bad\", expected \"DATAGRAM\" and one of %v", []string{luciexe.BuildProtoContentType, luciexe.BuildProtoZlibContentType}),
				Output: &bbpb.Build_Output{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: fmt.Sprintf("\n\nError in build protocol: build.proto stream \"u/build.proto\" has stream type \"TEXT\" and content type \"i r bad\", expected \"DATAGRAM\" and one of %v", []string{luciexe.BuildProtoContentType, luciexe.BuildProtoZlibContentType}),
				},
			}))
		})

		t.Run(`ignores out-of-namespace streams`, func(t *ftt.Test) {
			assert.Loosely(t, merger.onNewStream(&logpb.LogStreamDescriptor{Name: "uprefix"}), should.BeNil)
			assert.Loosely(t, merger.onNewStream(&logpb.LogStreamDescriptor{Name: "nope/something"}), should.BeNil)
			assert.Loosely(t, merger.states, should.BeEmpty)
		})

		t.Run(`ignores new registrations on closure`, func(t *ftt.Test) {
			merger.Close()
			merger.onNewStream(mkDesc("u/build.proto"))
			assert.Loosely(t, merger.states, should.BeEmpty)
		})

		t.Run(`will merge+relay root proto only`, func(t *ftt.Test) {
			cb := merger.onNewStream(mkDesc("u/build.proto"))
			assert.Loosely(t, cb, should.NotBeNil)
			tracker, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			tracker.handleNewData(mkDgram(t, &bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "Hello"},
				},
			}))

			mergedBuild := <-merger.MergedBuildC
			expect := reflectutil.ShallowCopy(base).(*bbpb.Build)
			expect.Steps = append(expect.Steps, &bbpb.Step{Name: "Hello"})
			expect.UpdateTime = now
			expect.Output.Logs[0].Url = "url://u/stdout"
			assert.Loosely(t, mergedBuild, should.Match(expect))

			merger.Close()
			<-merger.MergedBuildC // final build
		})

		t.Run(`can emit changes for merge steps`, func(t *ftt.Test) {
			merger.onNewStream(mkDesc("u/build.proto"))
			merger.onNewStream(mkDesc("u/sub/build.proto"))

			rootTrack, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)
			subTrack, ok := merger.states["url://u/sub/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			// No merge step yet
			rootTrack.handleNewData(mkDgram(t, &bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "Hello"},
				},
			}))
			expect := reflectutil.ShallowCopy(base).(*bbpb.Build)
			expect.Steps = append(expect.Steps, &bbpb.Step{Name: "Hello"})
			expect.UpdateTime = now
			expect.Output.Logs[0].Url = "url://u/stdout"
			assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))

			// order of updates doesn't matter, so we'll update the sub build first
			subTrack.handleNewData(mkDgram(t, &bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "SubStep"},
				},
			}))
			// the root stream doesn't have the merge step yet, so it doesn't show up.
			assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))

			// Ok, now add the merge step
			rootTrack.handleNewData(mkDgram(t, &bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "Hello"},
					{Name: "Merge",
						MergeBuild: &bbpb.Step_MergeBuild{
							FromLogdogStream: "sub/build.proto",
						}},
				},
			}))
			expect.Steps = append(expect.Steps, &bbpb.Step{
				Name: "Merge",
				MergeBuild: &bbpb.Step_MergeBuild{
					FromLogdogStream: "url://u/sub/build.proto",
				},
			})
			expect.Steps = append(expect.Steps, &bbpb.Step{Name: "Merge|SubStep"})
			expect.UpdateTime = now
			assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))

			t.Run(`and shut down`, func(t *ftt.Test) {
				merger.Close()
				expect.EndTime = now
				expect.Status = bbpb.Status_INFRA_FAILURE
				expect.Output.Status = bbpb.Status_INFRA_FAILURE
				expect.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."
				expect.Output.SummaryMarkdown = expect.SummaryMarkdown
				for _, step := range expect.Steps {
					step.EndTime = now
					if step.Name != "Merge" {
						step.Status = bbpb.Status_CANCELED
						step.SummaryMarkdown = "step was never finalized; did the build crash?"
					} else {
						step.Status = bbpb.Status_INFRA_FAILURE
						step.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."
					}
				}
				assert.Loosely(t, getFinal(), should.Match(expect))
			})

			t.Run(`can handle recursive merge steps`, func(t *ftt.Test) {
				merger.onNewStream(mkDesc("u/sub/super_deep/build.proto"))
				superTrack, ok := merger.states["url://u/sub/super_deep/build.proto"]
				assert.Loosely(t, ok, should.BeTrue)

				subTrack.handleNewData(mkDgram(t, &bbpb.Build{
					Steps: []*bbpb.Step{
						{Name: "SubStep"},
						{Name: "SuperDeep",
							MergeBuild: &bbpb.Step_MergeBuild{
								FromLogdogStream: "super_deep/build.proto",
							}},
					},
				}))
				<-merger.MergedBuildC // digest subTrack update
				superTrack.handleNewData(mkDgram(t, &bbpb.Build{
					Steps: []*bbpb.Step{
						{Name: "Hi!"},
					},
				}))
				expect.Steps = append(expect.Steps,
					&bbpb.Step{
						Name: "Merge|SuperDeep",
						MergeBuild: &bbpb.Step_MergeBuild{
							FromLogdogStream: "url://u/sub/super_deep/build.proto",
						},
					},
					&bbpb.Step{
						Name: "Merge|SuperDeep|Hi!",
					},
				)
				assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))

				t.Run(`and shut down`, func(t *ftt.Test) {
					merger.Close()

					expect.EndTime = now
					expect.Status = bbpb.Status_INFRA_FAILURE
					expect.Output.Status = bbpb.Status_INFRA_FAILURE
					expect.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."
					expect.Output.SummaryMarkdown = expect.SummaryMarkdown
					for _, step := range expect.Steps {
						step.EndTime = now
						switch step.Name {
						case "Merge", "Merge|SuperDeep":
							step.Status = bbpb.Status_INFRA_FAILURE
							step.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."
						default:
							step.Status = bbpb.Status_CANCELED
							step.SummaryMarkdown = "step was never finalized; did the build crash?"
						}
					}
					assert.Loosely(t, getFinal(), should.Match(expect))
				})
			})

			t.Run(`and merge sub-build successfully as it becomes invalid`, func(t *ftt.Test) {
				// added an invalid step to sub build
				subTrack.handleNewData(mkDgram(t, &bbpb.Build{
					Steps: []*bbpb.Step{
						{Name: "SubStep"},
						{
							Name: "Invalid_SubStep",
							Logs: []*bbpb.Log{
								{Url: "emoji 💩 is not a valid url"},
							},
						},
					},
				}))

				t.Run(`and shut down`, func(t *ftt.Test) {
					merger.Close()

					expect.EndTime = now
					expect.Status = bbpb.Status_INFRA_FAILURE
					expect.Output.Status = bbpb.Status_INFRA_FAILURE
					expect.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."
					expect.Output.SummaryMarkdown = expect.SummaryMarkdown
					expect.Steps = nil
					expect.Steps = append(expect.Steps,
						&bbpb.Step{
							Name:            "Hello",
							EndTime:         now,
							Status:          bbpb.Status_CANCELED,
							SummaryMarkdown: "step was never finalized; did the build crash?",
						},
						&bbpb.Step{
							Name:    "Merge",
							Status:  bbpb.Status_INFRA_FAILURE,
							EndTime: now,
							MergeBuild: &bbpb.Step_MergeBuild{
								FromLogdogStream: "url://u/sub/build.proto",
							},
							SummaryMarkdown: "\n\nError in build protocol: step[\"Invalid_SubStep\"].logs[\"\"]: bad log url \"emoji 💩 is not a valid url\": illegal character ( ) at index 5",
						},
						&bbpb.Step{
							Name:            "Merge|SubStep",
							EndTime:         now,
							Status:          bbpb.Status_CANCELED,
							SummaryMarkdown: "step was never finalized; did the build crash?",
						},
						&bbpb.Step{
							Name:    "Merge|Invalid_SubStep",
							Status:  bbpb.Status_INFRA_FAILURE,
							EndTime: now,
							Logs: []*bbpb.Log{
								{Url: "emoji 💩 is not a valid url"},
							},
							SummaryMarkdown: "bad log url \"emoji 💩 is not a valid url\": illegal character ( ) at index 5",
						},
					)
					assert.Loosely(t, getFinal(), should.Match(expect))
				})
			})
		})

		t.Run(`can merge sub-build`, func(t *ftt.Test) {
			merger.onNewStream(mkDesc("u/build.proto"))
			rootTrack, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			rootTrack.handleNewData(mkDgram(t, &bbpb.Build{
				Steps: []*bbpb.Step{
					{
						Name:   "Merge",
						Status: bbpb.Status_STARTED,
						MergeBuild: &bbpb.Step_MergeBuild{
							FromLogdogStream: "sub/build.proto",
						},
					},
				},
			}))

			expect := proto.Clone(base).(*bbpb.Build)
			expect.Steps = nil
			expect.UpdateTime = now
			expect.Output.Logs[0].Url = "url://u/stdout"

			t.Run(`when sub-build stream has not been registered yet`, func(t *ftt.Test) {
				expect.Steps = []*bbpb.Step{
					{
						Name:   "Merge",
						Status: bbpb.Status_STARTED,
						MergeBuild: &bbpb.Step_MergeBuild{
							FromLogdogStream: "url://u/sub/build.proto",
						},
						SummaryMarkdown: "build.proto stream: \"url://u/sub/build.proto\" is not registered",
					},
				}
				assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))

				t.Run(`Append existing SummaryMarkdown`, func(t *ftt.Test) {
					rootTrack.handleNewData(mkDgram(t, &bbpb.Build{
						Steps: []*bbpb.Step{
							{
								Name:            "Merge",
								Status:          bbpb.Status_STARTED,
								SummaryMarkdown: "existing summary",
								MergeBuild: &bbpb.Step_MergeBuild{
									FromLogdogStream: "sub/build.proto",
								},
							},
						},
					}))

					expect.Steps = []*bbpb.Step{
						{
							Name:   "Merge",
							Status: bbpb.Status_STARTED,
							MergeBuild: &bbpb.Step_MergeBuild{
								FromLogdogStream: "url://u/sub/build.proto",
							},
							SummaryMarkdown: "existing summary\n\nbuild.proto stream: \"url://u/sub/build.proto\" is not registered",
						},
					}
					assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))
				})

				t.Run(`then registered but stream is empty`, func(t *ftt.Test) {
					merger.onNewStream(mkDesc("u/sub/build.proto"))
					subTrack, ok := merger.states["url://u/sub/build.proto"]
					assert.Loosely(t, ok, should.BeTrue)
					expect.Steps = []*bbpb.Step{
						{
							Name:   "Merge",
							Status: bbpb.Status_STARTED,
							MergeBuild: &bbpb.Step_MergeBuild{
								FromLogdogStream: "url://u/sub/build.proto",
							},
							SummaryMarkdown: "build.proto stream: \"url://u/sub/build.proto\" is empty",
						},
					}
					// send something random to kick off a merge.
					merger.onNewStream(mkDesc("u/unknown/build.proto"))(mkDgram(t, &bbpb.Build{}))
					assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))

					t.Run(`finally merge properly when sub-build stream is present`, func(t *ftt.Test) {
						subTrack.handleNewData(mkDgram(t, &bbpb.Build{
							Status: bbpb.Status_SUCCESS,
							Output: &bbpb.Build_Output{
								Status: bbpb.Status_SUCCESS,
							},
							Steps: []*bbpb.Step{
								{Name: "SubStep"},
							},
						}))
						expect.Steps = []*bbpb.Step{
							{
								Name:   "Merge",
								Status: bbpb.Status_SUCCESS,
								MergeBuild: &bbpb.Step_MergeBuild{
									FromLogdogStream: "url://u/sub/build.proto",
								},
							},
							{Name: "Merge|SubStep"},
						}
						assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))
					})
				})
			})
		})

		t.Run(`can merge sub-build into global namespace`, func(t *ftt.Test) {
			merger.onNewStream(mkDesc("u/build.proto"))
			rootTrack, ok := merger.states["url://u/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			baseProps, err := structpb.NewStruct(map[string]any{
				"something": "value",
			})
			assert.Loosely(t, err, should.BeNil)

			rootTrack.handleNewData(mkDgram(t, &bbpb.Build{
				Output: &bbpb.Build_Output{
					Properties: baseProps,
				},
				SummaryMarkdown: "some words",
				Steps: []*bbpb.Step{
					{
						Name:   "Merge",
						Status: bbpb.Status_STARTED,
						MergeBuild: &bbpb.Step_MergeBuild{
							FromLogdogStream:      "sub/build.proto",
							LegacyGlobalNamespace: true,
						},
					},
				},
			}))
			// make sure to pull this through to avoid races
			<-merger.MergedBuildC

			expect := proto.Clone(base).(*bbpb.Build)
			expect.Steps = nil
			expect.UpdateTime = now
			expect.SummaryMarkdown = "some words"
			expect.Output.Logs[0].Url = "url://u/stdout"
			expect.Output.Properties, _ = structpb.NewStruct(map[string]any{
				"something": "value",
			})
			expect.Steps = []*bbpb.Step{
				{
					Name:   "Merge",
					Status: bbpb.Status_STARTED,
					MergeBuild: &bbpb.Step_MergeBuild{
						FromLogdogStream:      "url://u/sub/build.proto",
						LegacyGlobalNamespace: true,
					},
					SummaryMarkdown: "build.proto stream: \"url://u/sub/build.proto\" is empty",
				},
			}

			merger.onNewStream(mkDesc("u/sub/build.proto"))
			subTrack, ok := merger.states["url://u/sub/build.proto"]
			assert.Loosely(t, ok, should.BeTrue)

			t.Run(`Overwrites properties`, func(t *ftt.Test) {
				subProps, err := structpb.NewStruct(map[string]any{
					"new":       "prop",
					"something": "overwrite",
				})
				assert.Loosely(t, err, should.BeNil)
				subTrack.handleNewData(mkDgram(t, &bbpb.Build{
					Output: &bbpb.Build_Output{
						Properties: subProps,
						Status:     bbpb.Status_STARTED,
					},
					Status: bbpb.Status_STARTED,
					Steps: []*bbpb.Step{
						{Name: "SubStep"},
					},
				}))
				expect.Steps = append(expect.Steps, &bbpb.Step{Name: "SubStep"})
				expect.Output.Properties.Fields["new"] = structpb.NewStringValue("prop")
				expect.Output.Properties.Fields["something"] = structpb.NewStringValue("overwrite")
				expect.Steps[0].SummaryMarkdown = ""
				assert.Loosely(t, <-merger.MergedBuildC, should.Match(expect))
			})
		})
	})
}
