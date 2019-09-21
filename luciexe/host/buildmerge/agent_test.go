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
	structpb "github.com/golang/protobuf/ptypes/struct"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey(`buildState`, t, func() {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		So(err, ShouldBeNil)
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
		ctx, cancel := context.WithCancel(ctx)

		base := &bbpb.Build{
			Input: &bbpb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"test": {Kind: &structpb.Value_StringValue{
							StringValue: "value",
						}},
					},
				},
			},
		}
		// we omit view url here to keep tests simpler
		merger := New(ctx, "u/", base, func(ns, stream types.StreamName) (url, viewURL string) {
			return fmt.Sprintf("url://%s%s", ns, stream), ""
		})
		defer merger.Close()
		defer cancel()

		getFinal := func() (lastBuild *bbpb.Build) {
			for build := range merger.MergedBuildC {
				lastBuild = build
			}
			return
		}

		Convey(`can close without any data`, func() {
			merger.Close()
			build := <-merger.MergedBuildC

			So(build, ShouldResemble, base)
		})

		Convey(`bad stream type`, func() {
			merger.onNewStream(&logpb.LogStreamDescriptor{
				Name:        "u/build.proto",
				StreamType:  logpb.StreamType_TEXT, // should be DATAGRAM
				ContentType: luciexe.BuildProtoContentType,
			})
			// NOTE: here and below we do ShouldBeTrue on `ok` instead of using
			// ShouldNotBeNil on `tracker`. This is because ShouldNotBeNil is
			// currently (as of Sep'19) implemented in terms of ShouldBeNil, which
			// ends up traversing the entire `tracker` struct with `reflect`. This
			// causes the race detector to claim that we're reading the contents of
			// the atomic.Value in tracker without a lock (which is true).
			tracker, ok := merger.states["url://u/build.proto"]
			So(ok, ShouldBeTrue)

			So(tracker.getLatest().build, ShouldResemble, &bbpb.Build{
				EndTime:         now,
				UpdateTime:      now,
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: "\n\nError in build protocol: stream \"u/build.proto\" has type \"TEXT\", expected \"DATAGRAM\"",
			})
		})

		Convey(`bad content type`, func() {
			merger.onNewStream(&logpb.LogStreamDescriptor{
				Name:        "u/build.proto",
				StreamType:  logpb.StreamType_DATAGRAM,
				ContentType: "i r bad",
			})
			tracker, ok := merger.states["url://u/build.proto"]
			So(ok, ShouldBeTrue)

			So(tracker.getLatest().build, ShouldResemble, &bbpb.Build{
				EndTime:         now,
				UpdateTime:      now,
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: "\n\nError in build protocol: stream \"u/build.proto\" has content type \"i r bad\", expected \"" + luciexe.BuildProtoContentType + "\"",
			})
		})

		Convey(`ignores out-of-namespace streams`, func() {
			merger.onNewStream(&logpb.LogStreamDescriptor{Name: "uprefix"})
			merger.onNewStream(&logpb.LogStreamDescriptor{Name: "nope/something"})
			So(merger.states, ShouldBeEmpty)
		})

		Convey(`ignores new registrations on closure`, func() {
			merger.Close()
			merger.onNewStream(mkDesc("u/build.proto"))
			So(merger.states, ShouldBeEmpty)
		})

		Convey(`will merge+relay root proto only`, func() {
			merger.onNewStream(mkDesc("u/build.proto"))
			tracker, ok := merger.states["url://u/build.proto"]
			So(ok, ShouldBeTrue)

			tracker.handleNewData(mkDgram(&bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "Hello"},
				},
			}))

			mergedBuild := <-merger.MergedBuildC
			expect := *base
			expect.Steps = append(expect.Steps, &bbpb.Step{Name: "Hello"})
			expect.UpdateTime = now
			So(mergedBuild, ShouldResemble, &expect)

			merger.Close()
			<-merger.MergedBuildC // final build
		})

		Convey(`can emit changes for merge steps`, func() {
			merger.onNewStream(mkDesc("u/build.proto"))
			merger.onNewStream(mkDesc("u/sub/build.proto"))

			rootTrack, ok := merger.states["url://u/build.proto"]
			So(ok, ShouldBeTrue)
			subTrack, ok := merger.states["url://u/sub/build.proto"]
			So(ok, ShouldBeTrue)

			// No merge step yet
			rootTrack.handleNewData(mkDgram(&bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "Hello"},
				},
			}))
			expect := *base
			expect.Steps = append(expect.Steps, &bbpb.Step{Name: "Hello"})
			expect.UpdateTime = now
			So(<-merger.MergedBuildC, ShouldResemble, &expect)

			// order of updates doesn't matter, so we'll update the sub build first
			subTrack.handleNewData(mkDgram(&bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "SubStep"},
				},
			}))
			// the root stream doesn't have the merge step yet, so it doesn't show up.
			So(<-merger.MergedBuildC, ShouldResemble, &expect)

			// Ok, now add the merge step
			rootTrack.handleNewData(mkDgram(&bbpb.Build{
				Steps: []*bbpb.Step{
					{Name: "Hello"},
					{Name: "Merge", Logs: []*bbpb.Log{
						{Name: "$build.proto", Url: "sub/build.proto"},
					}},
				},
			}))
			expect.Steps = append(expect.Steps, &bbpb.Step{
				Name: "Merge",
				Logs: []*bbpb.Log{{
					Name: "$build.proto", Url: "url://u/sub/build.proto",
				}},
			})
			expect.Steps = append(expect.Steps, &bbpb.Step{Name: "Merge|SubStep"})
			expect.UpdateTime = now
			So(<-merger.MergedBuildC, ShouldResemble, &expect)

			Convey(`and shut down`, func() {
				merger.Close()
				expect.EndTime = now
				expect.Status = bbpb.Status_INFRA_FAILURE
				expect.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED."
				for _, step := range expect.Steps {
					step.EndTime = now
					if step.Name != "Merge" {
						step.Status = bbpb.Status_CANCELED
						step.SummaryMarkdown = "step was never finalized; did the build crash?"
					} else {
						step.Status = bbpb.Status_INFRA_FAILURE
						step.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED."
					}
				}
				So(getFinal(), ShouldResemble, &expect)
			})

			Convey(`can handle recursive merge steps`, func() {
				merger.onNewStream(mkDesc("u/sub/super_deep/build.proto"))
				superTrack, ok := merger.states["url://u/sub/super_deep/build.proto"]
				So(ok, ShouldBeTrue)

				subTrack.handleNewData(mkDgram(&bbpb.Build{
					Steps: []*bbpb.Step{
						{Name: "SubStep"},
						{Name: "SuperDeep", Logs: []*bbpb.Log{
							{Name: "$build.proto", Url: "super_deep/build.proto"},
						}},
					},
				}))
				expect.Steps = append(expect.Steps, &bbpb.Step{
					Name:            "Merge|SuperDeep",
					Status:          bbpb.Status_SCHEDULED,
					SummaryMarkdown: "build.proto not found",
					Logs: []*bbpb.Log{{
						Name: "$build.proto", Url: "url://u/sub/super_deep/build.proto",
					}},
				})
				So(<-merger.MergedBuildC, ShouldResemble, &expect)

				superTrack.handleNewData(mkDgram(&bbpb.Build{
					Steps: []*bbpb.Step{
						{Name: "Hi!"},
					},
				}))
				expect.Steps[len(expect.Steps)-1].Status = bbpb.Status_STATUS_UNSPECIFIED
				expect.Steps[len(expect.Steps)-1].SummaryMarkdown = ""
				expect.Steps = append(expect.Steps, &bbpb.Step{
					Name: "Merge|SuperDeep|Hi!",
				})
				So(<-merger.MergedBuildC, ShouldResemble, &expect)

				Convey(`and shut down`, func() {
					merger.Close()

					expect.EndTime = now
					expect.Status = bbpb.Status_INFRA_FAILURE
					expect.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED."
					for _, step := range expect.Steps {
						step.EndTime = now
						switch step.Name {
						case "Merge", "Merge|SuperDeep":
							step.Status = bbpb.Status_INFRA_FAILURE
							step.SummaryMarkdown = "\n\nError in build protocol: Expected a terminal build status, got STATUS_UNSPECIFIED."
						default:
							step.Status = bbpb.Status_CANCELED
							step.SummaryMarkdown = "step was never finalized; did the build crash?"
						}
					}
					So(getFinal(), ShouldResemble, &expect)
				})
			})
		})
	})
}
