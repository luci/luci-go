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
	"fmt"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/logdog/api/logpb"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	. "github.com/smartystreets/goconvey/convey"
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

func TestBuildState(t *testing.T) {
	t.Parallel()

	Convey(`buildState`, t, func() {
		now, err := ptypes.TimestampProto(testclock.TestRecentTimeLocal)
		So(err, ShouldBeNil)

		merger := &agent{
			clkNow: func() *timestamp.Timestamp { return now },
			calculateURLs: func(ns, stream string) (url, viewURL string) {
				return fmt.Sprintf("url://%s/%s", ns, stream), fmt.Sprintf("view://%s/%s", ns, stream)
			},
			amCollectingData: true,
		}

		Convey(`opened in error state`, func() {
			bs := newBuildState(merger, "ns", errors.New("nope"))
			So(bs, ShouldResemble, &buildState{
				merger: merger,
				latest: &bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "\n\nError in build protocol: nope",
				},
				logdogNS: "ns",
				closed:   true,
			})
		})

		Convey(`basic`, func() {
			bs := newBuildState(merger, "ns", nil)
			So(bs, ShouldResemble, &buildState{merger: merger, logdogNS: "ns"})

			Convey(`ignores updates when merger is not collecting data`, func() {
				merger.amCollectingData = false
				bs.parse(mkDgram(&bbpb.Build{
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
				build, valid := bs.GetLatest()
				So(valid, ShouldBeTrue)
				So(build, ShouldResemble, &bbpb.Build{
					Status:          bbpb.Status_SCHEDULED,
					SummaryMarkdown: "build.proto not found",
				})

				Convey(`can still close, though`, func() {
					bs.parse(nil)
					build, valid := bs.GetLatest()
					So(valid, ShouldBeTrue)
					So(build, ShouldResemble, &bbpb.Build{
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "Never recieved any build data.",
					})
				})
			})

			Convey(`no updates`, func() {
				Convey(`GetLatest`, func() {
					build, valid := bs.GetLatest()
					So(valid, ShouldBeTrue)
					So(build, ShouldResemble, &bbpb.Build{
						Status:          bbpb.Status_SCHEDULED,
						SummaryMarkdown: "build.proto not found",
					})
				})

				Convey(`StepCount`, func() {
					So(bs.StepCount(), ShouldEqual, 0)
				})

				Convey(`parse(nil)`, func() {
					bs.parse(nil)
					build, valid := bs.GetLatest()
					So(valid, ShouldBeTrue)
					So(build, ShouldResemble, &bbpb.Build{
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "Never recieved any build data.",
					})
					So(bs.closed, ShouldBeTrue)
					So(bs.invalid, ShouldBeFalse)

					Convey(`convey close noop`, func() {
						// not a valid state, but bs is closed, so parse should do nothing
						// when invoked.
						bs.latest = nil
						bs.parse(nil)
						So(bs.latest, ShouldBeNil)
					})
				})
			})

			Convey(`valid update`, func() {
				bs.parse(mkDgram(&bbpb.Build{
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

				build, valid := bs.GetLatest()
				So(valid, ShouldBeTrue)
				So(build, ShouldResemble, &bbpb.Build{
					SummaryMarkdown: "some stuff",
					Steps: []*bbpb.Step{
						{Name: "Parent"},
						{Name: "Parent|Child"},
						{
							Name: "Parent|Merge",
							Logs: []*bbpb.Log{{
								Name:    "$build.proto",
								Url:     "url://ns/Parent/Merge/build.proto",
								ViewUrl: "view://ns/Parent/Merge/build.proto",
							}},
						},
					},
					UpdateTime: now,
				})

				Convey(`followed by garbage`, func() {
					bs.parse(&logpb.LogEntry{
						Content: &logpb.LogEntry_Datagram{Datagram: &logpb.Datagram{
							Data: []byte("narpnarp"),
						}},
					})
					So(bs.closed, ShouldBeTrue)
					build, valid := bs.GetLatest()
					So(valid, ShouldBeFalse)
					So(build, ShouldResemble, &bbpb.Build{
						SummaryMarkdown: ("some stuff\n\n" +
							"Error in build protocol: parsing Build: proto: " +
							"can't skip unknown wire type 6"),
						Status: bbpb.Status_INFRA_FAILURE,
						Steps: []*bbpb.Step{
							{Name: "Parent", EndTime: now, Status: bbpb.Status_CANCELED,
								SummaryMarkdown: "step was never finalized; did the build crash?"},
							{Name: "Parent|Child", EndTime: now, Status: bbpb.Status_CANCELED,
								SummaryMarkdown: "step was never finalized; did the build crash?"},
							{
								Name:            "Parent|Merge",
								EndTime:         now,
								Status:          bbpb.Status_CANCELED,
								SummaryMarkdown: "step was never finalized; did the build crash?",
								Logs: []*bbpb.Log{{
									Name:    "$build.proto",
									Url:     "url://ns/Parent/Merge/build.proto",
									ViewUrl: "view://ns/Parent/Merge/build.proto",
								}},
							},
						},
						UpdateTime: now,
						EndTime:    now,
					})
				})

				Convey(`parse(nil)`, func() {
					bs.parse(nil)
					build, valid := bs.GetLatest()
					So(valid, ShouldBeTrue)
					So(build, ShouldResemble, &bbpb.Build{
						SummaryMarkdown: ("some stuff\n\nError in build protocol: " +
							"Expected a terminal build status, " +
							"got STATUS_UNSPECIFIED."),
						Status: bbpb.Status_INFRA_FAILURE,
						Steps: []*bbpb.Step{
							{Name: "Parent", EndTime: now, Status: bbpb.Status_CANCELED,
								SummaryMarkdown: "step was never finalized; did the build crash?"},
							{Name: "Parent|Child", EndTime: now, Status: bbpb.Status_CANCELED,
								SummaryMarkdown: "step was never finalized; did the build crash?"},
							{
								Name:            "Parent|Merge",
								EndTime:         now,
								Status:          bbpb.Status_CANCELED,
								SummaryMarkdown: "step was never finalized; did the build crash?",
								Logs: []*bbpb.Log{{
									Name:    "$build.proto",
									Url:     "url://ns/Parent/Merge/build.proto",
									ViewUrl: "view://ns/Parent/Merge/build.proto",
								}},
							},
						},
						EndTime:    now,
						UpdateTime: now,
					})
				})

			})

			Convey(`invalid build data`, func() {
				bs.parse(&logpb.LogEntry{
					Content: &logpb.LogEntry_Datagram{Datagram: &logpb.Datagram{
						Data: []byte("narpnarp"),
					}},
				})
				So(bs.closed, ShouldBeTrue)
				build, valid := bs.GetLatest()
				So(valid, ShouldBeFalse)
				So(build, ShouldResemble, &bbpb.Build{
					SummaryMarkdown: ("\n\nError in build protocol: " +
						"parsing Build: proto: can't skip unknown wire type 6"),
					Status:     bbpb.Status_INFRA_FAILURE,
					UpdateTime: now,
					EndTime:    now,
				})

				Convey(`ignores further updates`, func() {
					bs.parse(mkDgram(&bbpb.Build{SummaryMarkdown: "hi"}))
					build, valid := bs.GetLatest()
					So(valid, ShouldBeFalse)
					So(build, ShouldResemble, &bbpb.Build{
						SummaryMarkdown: ("\n\nError in build protocol: " +
							"parsing Build: proto: can't skip unknown wire type 6"),
						Status:     bbpb.Status_INFRA_FAILURE,
						UpdateTime: now,
						EndTime:    now,
					})
				})
			})

			Convey(`bad log url`, func() {
				bs.parse(mkDgram(&bbpb.Build{
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
				So(bs.closed, ShouldBeTrue)
				build, valid := bs.GetLatest()
				So(valid, ShouldBeFalse)
				So(build, ShouldResemble, &bbpb.Build{
					SummaryMarkdown: ("\n\nError in build protocol: " +
						"step[\"hi\"].logs[\"log\"].Url = \"!!badnews!!\": " +
						"segment (at 0) must begin with alphanumeric character"),
					Steps: []*bbpb.Step{
						{
							Name:            "hi",
							Status:          bbpb.Status_INFRA_FAILURE,
							EndTime:         now,
							SummaryMarkdown: "bad log url: \"!!badnews!!\"",
							Logs: []*bbpb.Log{{
								Name: "log",
								Url:  "!!badnews!!",
							}},
						},
					},
					Status:     bbpb.Status_INFRA_FAILURE,
					UpdateTime: now,
					EndTime:    now,
				})
			})

		})

	})
}
