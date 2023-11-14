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

package protoutil

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimestamps(t *testing.T) {
	t.Parallel()

	Convey("Durations", t, func() {
		build := &pb.Build{
			CreateTime: &timestamppb.Timestamp{Seconds: 1000},
			StartTime:  &timestamppb.Timestamp{Seconds: 1010},
			EndTime:    &timestamppb.Timestamp{Seconds: 1030},
		}
		dur, ok := SchedulingDuration(build)
		So(ok, ShouldBeTrue)
		So(dur, ShouldEqual, 10*time.Second)

		dur, ok = RunDuration(build)
		So(ok, ShouldBeTrue)
		So(dur, ShouldEqual, 20*time.Second)
	})
}

func TestSetStatus(t *testing.T) {
	t.Parallel()

	Convey("SetStatus", t, func() {
		build := &pb.Build{}
		now := testclock.TestRecentTimeUTC

		Convey("STARTED", func() {
			SetStatus(now, build, pb.Status_STARTED)
			So(build, ShouldResembleProto, &pb.Build{
				Status:     pb.Status_STARTED,
				StartTime:  timestamppb.New(now),
				UpdateTime: timestamppb.New(now),
			})

			Convey("no-op", func() {
				SetStatus(now.Add(time.Minute), build, pb.Status_STARTED)
				So(build, ShouldResembleProto, &pb.Build{
					Status:     pb.Status_STARTED,
					StartTime:  timestamppb.New(now),
					UpdateTime: timestamppb.New(now),
				})
			})
		})

		Convey("CANCELED", func() {
			SetStatus(now, build, pb.Status_CANCELED)
			So(build, ShouldResembleProto, &pb.Build{
				Status:     pb.Status_CANCELED,
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
			})
		})
	})
}

func TestExePayloadPath(t *testing.T) {
	t.Parallel()

	Convey("from agent.purposes", t, func() {
		b := &pb.Build{
			Infra: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
							"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
						},
					},
				},
			},
		}
		So(ExePayloadPath(b), ShouldEqual, "kitchen-checkout")
	})

	Convey("from agent.purposes", t, func() {
		b := &pb.Build{
			Infra: &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
			},
		}
		So(ExePayloadPath(b), ShouldEqual, "kitchen-checkout")
	})
}

func TestMergeSummary(t *testing.T) {
	t.Parallel()

	Convey("Variants of MergeSummary", t, func() {
		Convey("No cancel message", func() {
			b := &pb.Build{
				SummaryMarkdown: "summary",
			}
			So(MergeSummary(b), ShouldEqual, "summary")
		})

		Convey("No summary message", func() {
			b := &pb.Build{
				CancellationMarkdown: "cancellation",
			}
			So(MergeSummary(b), ShouldEqual, "cancellation")
		})

		Convey("Summary and cancel message", func() {
			b := &pb.Build{
				SummaryMarkdown:      "summary",
				CancellationMarkdown: "cancellation",
			}
			So(MergeSummary(b), ShouldEqual, "summary\ncancellation")
		})

		Convey("Summary and task message", func() {
			b := &pb.Build{
				SummaryMarkdown: "summary",
				Infra: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							SummaryMarkdown: "bot_died",
						},
					},
				},
			}
			So(MergeSummary(b), ShouldEqual, "summary\nbot_died")
		})

		Convey("Neither summary nor CancelMessage", func() {
			b := &pb.Build{}
			So(MergeSummary(b), ShouldBeEmpty)
		})

		Convey("merged summary too long", func() {
			b := &pb.Build{
				SummaryMarkdown: strings.Repeat("l", SummaryMarkdownMaxLength+1),
			}
			So(MergeSummary(b), ShouldEqual, strings.Repeat("l", SummaryMarkdownMaxLength-3)+"...")
		})

		Convey("Summary duplication", func() {
			b := &pb.Build{
				SummaryMarkdown: "summary",
				Output: &pb.Build_Output{
					SummaryMarkdown: "summary",
				},
				CancellationMarkdown: "cancellation",
			}
			So(MergeSummary(b), ShouldEqual, "summary\ncancellation")
		})
	})
}
