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
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func mkDgram(build *bbpb.Build) *logpb.LogEntry {
	data, err := proto.Marshal(build)
	So(err, ShouldBeNil)
	return &logpb.LogEntry{
		Content: &logpb.LogEntry_Datagram{Datagram: &logpb.Datagram{
			Data: data,
		}},
	}
}

// cleanupProtoError inspects the SummaryMarkdown field of build proto
// and if it contains proto err, it will strip out the proto error message.
// The reason we are doing this is that the proto lib discourage us to perform
// error string comparison. See:
// https://github.com/protocolbuffers/protobuf-go/blob/cd108d00a8df3bba55927ef35ca07438b895d7aa/internal/errors/errors.go#L26-L34
func cleanupProtoError(builds ...*bbpb.Build) {
	re := regexp.MustCompile(`Error in build protocol:.*\sproto:`)
	for _, build := range builds {
		if build.Output.GetSummaryMarkdown() != "" {
			So(build.Output.SummaryMarkdown, ShouldEqual, build.SummaryMarkdown)
		}
		if loc := re.FindStringIndex(build.SummaryMarkdown); loc != nil {
			build.SummaryMarkdown = build.SummaryMarkdown[:loc[1]]
			if build.Output.GetSummaryMarkdown() != "" {
				build.Output.SummaryMarkdown = build.SummaryMarkdown
			}
		}
	}
}

func assertStateEqual(actual, expected *buildState) {
	cleanupProtoError(actual.build, expected.build)
	So(actual.build, ShouldResembleProto, expected.build)
	So(actual.closed, ShouldEqual, expected.closed)
	So(actual.final, ShouldEqual, expected.final)
	So(actual.invalid, ShouldEqual, expected.invalid)
}

func TestBuildState(t *testing.T) {
	t.Parallel()

	Convey(`buildState`, t, func() {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		So(err, ShouldBeNil)
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		merger, err := New(ctx, "u/", &bbpb.Build{Output: &bbpb.Build_Output{}}, func(ns, stream types.StreamName) (url, viewURL string) {
			return fmt.Sprintf("url://%s%s", ns, stream), fmt.Sprintf("view://%s%s", ns, stream)
		})
		So(err, ShouldBeNil)
		defer merger.Close()

		informChan := make(chan struct{}, 1)
		merger.informNewData = func() {
			informChan <- struct{}{}
		}
		wait := func() {
			<-informChan
		}

		Convey(`opened in error state`, func() {
			bs := newBuildStateTracker(ctx, merger, "ns/", false, errors.New("nope"))
			wait() // for final build
			So(bs.workClosed, ShouldBeTrue)
			bs.Drain()
			assertStateEqual(bs.latestState, &buildState{
				build: &bbpb.Build{
					SummaryMarkdown: "\n\nError in build protocol: nope",
					Status:          bbpb.Status_INFRA_FAILURE,
					UpdateTime:      now,
					EndTime:         now,
					Output: &bbpb.Build_Output{
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "\n\nError in build protocol: nope",
					},
				},
				closed: true,
				final:  true,
			})
		})

		Convey(`basic`, func() {
			bs := newBuildStateTracker(ctx, merger, "ns/", false, nil)

			Convey(`ignores updates when merger cancels context`, func() {
				cancel()
				wait() // cancel generated an 'informNewData'

				bs.handleNewData(mkDgram(&bbpb.Build{
					SummaryMarkdown: "some stuff",
					Steps: []*bbpb.Step{
						{Name: "Parent"},
						{Name: "Parent|Child"},
						{
							Name: "Parent|Merge",
							Logs: []*bbpb.Log{{
								Name: "$build.proto",
								Url:  "Parent/Merge/build.proto",
							}},
						},
					},
				}))
				// No wait, because this handleNewData was ignored.
				bs.Drain()
				assertStateEqual(bs.latestState, &buildState{
					build: &bbpb.Build{
						EndTime:         now,
						UpdateTime:      now,
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "Never received any build data.",
						Output: &bbpb.Build_Output{
							Status:          bbpb.Status_INFRA_FAILURE,
							SummaryMarkdown: "Never received any build data.",
						},
					},
					closed: true,
					final:  true,
				})

				Convey(`can still close, though`, func() {
					bs.handleNewData(nil)
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						closed: true,
						final:  true,
						build: &bbpb.Build{
							EndTime:         now,
							UpdateTime:      now,
							Status:          bbpb.Status_INFRA_FAILURE,
							SummaryMarkdown: "Never received any build data.",
							Output: &bbpb.Build_Output{
								Status:          bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: "Never received any build data.",
							},
						},
					})
				})
			})

			Convey(`no updates`, func() {
				Convey(`handleNewData(nil)`, func() {
					bs.handleNewData(nil)
					wait()
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						closed: true,
						final:  true,
						build: &bbpb.Build{
							EndTime:         now,
							UpdateTime:      now,
							Status:          bbpb.Status_INFRA_FAILURE,
							SummaryMarkdown: "Never received any build data.",
							Output: &bbpb.Build_Output{
								Status:          bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: "Never received any build data.",
							},
						}},
					)

					Convey(`convey close noop`, func() {
						// not a valid state, but bs is closed, so handleNewData should do nothing
						// when invoked.
						bs.latestState = &buildState{
							closed: true,
							final:  true,
							build:  &bbpb.Build{SummaryMarkdown: "wat"},
						}
						bs.handleNewData(nil)
						assertStateEqual(bs.latestState, &buildState{
							closed: true,
							final:  true,
							build:  &bbpb.Build{SummaryMarkdown: "wat"},
						})
					})
				})
			})

			Convey(`valid update`, func() {
				bs.handleNewData(mkDgram(&bbpb.Build{
					SummaryMarkdown: "some stuff",
					Steps: []*bbpb.Step{
						{Name: "Parent"},
						{Name: "Parent|Child"},
						{
							Name: "Parent|Merge",
							Logs: []*bbpb.Log{{
								Name: "$build.proto",
								Url:  "Parent/Merge/build.proto",
							}},
						},
					},
					Output: &bbpb.Build_Output{
						Logs: []*bbpb.Log{{
							Name: "stderr",
							Url:  "stderr",
						}},
					},
				}))
				wait()

				So(bs.getLatestBuild(), ShouldResembleProto, &bbpb.Build{
					SummaryMarkdown: "some stuff",
					Steps: []*bbpb.Step{
						{Name: "Parent"},
						{Name: "Parent|Child"},
						{
							Name: "Parent|Merge",
							Logs: []*bbpb.Log{},
							MergeBuild: &bbpb.Step_MergeBuild{
								FromLogdogStream: "url://ns/Parent/Merge/build.proto",
							},
						},
					},
					UpdateTime: now,
					Output: &bbpb.Build_Output{
						Logs: []*bbpb.Log{{
							Name:    "stderr",
							Url:     "url://ns/stderr",
							ViewUrl: "view://ns/stderr",
						}},
					},
				})

				Convey(`followed by garbage`, func() {
					bs.handleNewData(&logpb.LogEntry{
						Content: &logpb.LogEntry_Datagram{Datagram: &logpb.Datagram{
							Data: []byte("narpnarp"),
						}},
					})
					wait()
					wait() // for final build
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						closed:  true,
						final:   true,
						invalid: true,
						build: &bbpb.Build{
							SummaryMarkdown: ("some stuff\n\n" +
								"Error in build protocol: parsing Build: proto:"),
							Status: bbpb.Status_INFRA_FAILURE,
							Steps: []*bbpb.Step{
								{Name: "Parent", EndTime: now, Status: bbpb.Status_CANCELED,
									SummaryMarkdown: "step was never finalized; did the build crash?"},
								{Name: "Parent|Child", EndTime: now, Status: bbpb.Status_CANCELED,
									SummaryMarkdown: "step was never finalized; did the build crash?"},
								{
									Name: "Parent|Merge",
									Logs: []*bbpb.Log{},
									MergeBuild: &bbpb.Step_MergeBuild{
										FromLogdogStream: "url://ns/Parent/Merge/build.proto",
									},
								},
							},
							UpdateTime: now,
							EndTime:    now,
							Output: &bbpb.Build_Output{
								Logs: []*bbpb.Log{{
									Name:    "stderr",
									Url:     "url://ns/stderr",
									ViewUrl: "view://ns/stderr",
								}},
								Status: bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: ("some stuff\n\n" +
									"Error in build protocol: parsing Build: proto:"),
							},
						},
					})
				})

				Convey(`handleNewData(nil)`, func() {
					bs.handleNewData(nil)
					wait()
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						closed: true,
						final:  true,
						build: &bbpb.Build{
							SummaryMarkdown: ("some stuff\n\nError in build protocol: " +
								"Expected a terminal build status, " +
								"got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."),
							Status: bbpb.Status_INFRA_FAILURE,
							Steps: []*bbpb.Step{
								{Name: "Parent", EndTime: now, Status: bbpb.Status_CANCELED,
									SummaryMarkdown: "step was never finalized; did the build crash?"},
								{Name: "Parent|Child", EndTime: now, Status: bbpb.Status_CANCELED,
									SummaryMarkdown: "step was never finalized; did the build crash?"},
								{
									Name: "Parent|Merge",
									Logs: []*bbpb.Log{},
									MergeBuild: &bbpb.Step_MergeBuild{
										FromLogdogStream: "url://ns/Parent/Merge/build.proto",
									},
								},
							},
							EndTime:    now,
							UpdateTime: now,
							Output: &bbpb.Build_Output{
								Logs: []*bbpb.Log{{
									Name:    "stderr",
									Url:     "url://ns/stderr",
									ViewUrl: "view://ns/stderr",
								}},
								Status: bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: ("some stuff\n\nError in build protocol: " +
									"Expected a terminal build status, " +
									"got STATUS_UNSPECIFIED, while top level status is STATUS_UNSPECIFIED."),
							},
						},
					})
				})
			})

			Convey(`invalid build data`, func() {
				bs.handleNewData(&logpb.LogEntry{
					Content: &logpb.LogEntry_Datagram{Datagram: &logpb.Datagram{
						Data: []byte("narpnarp"),
					}},
				})
				wait()
				wait() // for final build
				bs.Drain()
				assertStateEqual(bs.latestState, &buildState{
					closed:  true,
					final:   true,
					invalid: true,
					build: &bbpb.Build{
						SummaryMarkdown: ("\n\nError in build protocol: parsing Build: proto:"),
						Status:          bbpb.Status_INFRA_FAILURE,
						UpdateTime:      now,
						EndTime:         now,
						Output: &bbpb.Build_Output{
							Status:          bbpb.Status_INFRA_FAILURE,
							SummaryMarkdown: ("\n\nError in build protocol: parsing Build: proto:"),
						},
					},
				})

				Convey(`ignores further updates`, func() {
					bs.handleNewData(mkDgram(&bbpb.Build{SummaryMarkdown: "hi"}))
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						invalid: true,
						final:   true,
						closed:  true,
						build: &bbpb.Build{
							SummaryMarkdown: ("\n\nError in build protocol: parsing Build: proto:"),
							Status:          bbpb.Status_INFRA_FAILURE,
							UpdateTime:      now,
							EndTime:         now,
							Output: &bbpb.Build_Output{
								Status:          bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: ("\n\nError in build protocol: parsing Build: proto:"),
							},
						},
					})
				})
			})

			Convey(`accept absolute url`, func() {
				bs.handleNewData(mkDgram(&bbpb.Build{
					Steps: []*bbpb.Step{
						{
							Name: "hi",
							Logs: []*bbpb.Log{{
								Name: "foo",
								Url:  "log/foo",
							}},
						},
						{
							Name: "heyo",
							Logs: []*bbpb.Log{{
								Name:    "bar",
								Url:     "url://another_ns/log/bar", // absolute url populated
								ViewUrl: "view://another_ns/log/bar",
							}},
						},
					},
					Output: &bbpb.Build_Output{
						Logs: []*bbpb.Log{
							{
								Name: "stderr",
								Url:  "stderr",
							},
							{
								Name:    "another stderr",
								Url:     "url://another_ns/stderr", // absolute url populated
								ViewUrl: "view://another_ns/stderr",
							},
						},
					},
				}))
				wait()

				So(bs.getLatestBuild(), ShouldResembleProto, &bbpb.Build{
					Steps: []*bbpb.Step{
						{
							Name: "hi",
							Logs: []*bbpb.Log{{
								Name:    "foo",
								Url:     "url://ns/log/foo",
								ViewUrl: "view://ns/log/foo",
							}},
						},
						{
							Name: "heyo",
							Logs: []*bbpb.Log{{
								Name:    "bar",
								Url:     "url://another_ns/log/bar",
								ViewUrl: "view://another_ns/log/bar",
							}},
						},
					},
					UpdateTime: now,
					Output: &bbpb.Build_Output{
						Logs: []*bbpb.Log{
							{
								Name:    "stderr",
								Url:     "url://ns/stderr",
								ViewUrl: "view://ns/stderr",
							},
							{
								Name:    "another stderr",
								Url:     "url://another_ns/stderr",
								ViewUrl: "view://another_ns/stderr",
							},
						},
					},
				})
			})

			Convey(`bad log url`, func() {
				Convey(`step log`, func() {
					bs.handleNewData(mkDgram(&bbpb.Build{
						Steps: []*bbpb.Step{
							{
								Name: "hi",
								Logs: []*bbpb.Log{{
									Name: "log",
									Url:  "!!badnews!!",
								}},
							},
						},
					}))
					wait()
					wait() // for final build
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						closed:  true,
						final:   true,
						invalid: true,
						build: &bbpb.Build{
							SummaryMarkdown: ("\n\nError in build protocol: " +
								"step[\"hi\"].logs[\"log\"]: bad log url \"!!badnews!!\": " +
								"segment (at 0) must begin with alphanumeric character"),
							Steps: []*bbpb.Step{
								{
									Name:    "hi",
									Status:  bbpb.Status_INFRA_FAILURE,
									EndTime: now,
									SummaryMarkdown: ("bad log url \"!!badnews!!\": " +
										"segment (at 0) must begin with alphanumeric character"),
									Logs: []*bbpb.Log{{
										Name: "log",
										Url:  "!!badnews!!",
									}},
								},
							},
							Status:     bbpb.Status_INFRA_FAILURE,
							UpdateTime: now,
							EndTime:    now,
							Output: &bbpb.Build_Output{
								Status: bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: ("\n\nError in build protocol: " +
									"step[\"hi\"].logs[\"log\"]: bad log url \"!!badnews!!\": " +
									"segment (at 0) must begin with alphanumeric character"),
							},
						},
					})
				})

				Convey(`build log`, func() {
					bs.handleNewData(mkDgram(&bbpb.Build{
						Output: &bbpb.Build_Output{
							Logs: []*bbpb.Log{{
								Name: "log",
								Url:  "!!badnews!!",
							}},
						},
					}))
					wait()
					wait() // for final build
					bs.Drain()
					assertStateEqual(bs.latestState, &buildState{
						closed:  true,
						final:   true,
						invalid: true,
						build: &bbpb.Build{
							SummaryMarkdown: ("\n\nError in build protocol: " +
								"build.output.logs[\"log\"]: bad log url \"!!badnews!!\": " +
								"segment (at 0) must begin with alphanumeric character"),
							Output: &bbpb.Build_Output{
								Logs: []*bbpb.Log{{
									Name: "log",
									Url:  "!!badnews!!",
								}},
								Status: bbpb.Status_INFRA_FAILURE,
								SummaryMarkdown: ("\n\nError in build protocol: " +
									"build.output.logs[\"log\"]: bad log url \"!!badnews!!\": " +
									"segment (at 0) must begin with alphanumeric character"),
							},
							Status:     bbpb.Status_INFRA_FAILURE,
							UpdateTime: now,
							EndTime:    now,
						},
					})
				})
			})
		})
	})
}
