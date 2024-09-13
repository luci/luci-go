// Copyright 2020 The LUCI Authors.
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
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey(`State`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		nowpb := timestamppb.New(testclock.TestRecentTimeUTC)

		origInfra := &bbpb.BuildInfra{
			Buildbucket: &bbpb.BuildInfra_Buildbucket{
				ServiceConfigRevision: "I am a string",
			},
		}
		st, ctx, err := Start(ctx, &bbpb.Build{
			Infra: origInfra,
		})
		So(err, ShouldBeNil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()

		Convey(`StartStep`, func() {
			step, _ := StartStep(ctx, "some step")
			defer func() { step.End(nil) }()

			So(st.buildPb.Steps, ShouldResembleProto, []*bbpb.Step{
				{Name: "some step", StartTime: nowpb, Status: bbpb.Status_STARTED},
			})

			Convey(`child with explicit parent`, func() {
				child, _ := step.StartStep(ctx, "child step")
				defer func() { child.End(nil) }()

				So(st.buildPb.Steps, ShouldResembleProto, []*bbpb.Step{
					{Name: "some step", StartTime: nowpb, Status: bbpb.Status_STARTED},
					{Name: "some step|child step", StartTime: nowpb, Status: bbpb.Status_STARTED},
				})
			})
		})

		Convey(`End`, func() {
			Convey(`cannot End twice`, func() {
				st.End(nil)
				So(func() { st.End(nil) }, ShouldPanicLike, "cannot mutate ended build")
				st = nil
			})
		})

		Convey(`Build.Infra()`, func() {
			build := st.Build()
			So(build.GetInfra().Buildbucket.ServiceConfigRevision, ShouldResemble, "I am a string")
			build.GetInfra().Buildbucket.ServiceConfigRevision = "narf"
			So(origInfra.Buildbucket.ServiceConfigRevision, ShouldResemble, "I am a string")

			Convey(`nil build`, func() {
				st, _, err := Start(ctx, nil)
				So(err, ShouldBeNil)
				defer func() {
					if st != nil {
						st.End(nil)
					}
				}()
				So(st.Build().GetInfra(), ShouldBeNil)
			})
		})
	})
}

func TestStateLogging(t *testing.T) {
	t.Parallel()

	Convey(`State logging`, t, func() {
		scFake, lc := streamclient.NewUnregisteredFake("fakeNS")

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		buildInfra := &bbpb.BuildInfra{
			Logdog: &bbpb.BuildInfra_LogDog{
				Hostname: "logs.chromium.org",
				Project:  "example",
				Prefix:   "builds/8888888888",
			},
		}
		st, ctx, err := Start(ctx, &bbpb.Build{
			Output: &bbpb.Build_Output{
				Logs: []*bbpb.Log{
					{Name: "something"},
					{Name: "other"},
				},
			},
			Infra: buildInfra,
		}, OptLogsink(lc))
		So(err, ShouldBeNil)
		defer func() { st.End(nil) }()
		So(st, ShouldNotBeNil)

		Convey(`existing logs are reserved`, func() {
			So(st.logNames.pool, ShouldResemble, map[string]int{
				"something": 1,
				"other":     1,
			})
		})

		Convey(`can open logs`, func() {
			log := st.Log("some log")
			fmt.Fprintln(log, "here's some stuff")
			So(st.buildPb, ShouldResembleProto, &bbpb.Build{
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_STARTED,
				Input:     &bbpb.Build_Input{},
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "something"},
						{Name: "other"},
						{Name: "some log", Url: "log/2"},
					},
					Status: bbpb.Status_STARTED,
				},
				Infra: buildInfra,
			})

			So(scFake.Data()["fakeNS/log/2"].GetStreamData(), ShouldContainSubstring, "here's some stuff")

			// Check the link.
			wantLink := "https://logs.chromium.org/logs/example/builds/8888888888/+/fakeNS/log/2"
			So(log.UILink(), ShouldEqual, wantLink)
		})

		Convey(`can open datagram logs`, func() {
			log := st.LogDatagram("some log")
			log.WriteDatagram([]byte("here's some stuff"))

			So(st.buildPb, ShouldResembleProto, &bbpb.Build{
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_STARTED,
				Input:     &bbpb.Build_Input{},
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "something"},
						{Name: "other"},
						{Name: "some log", Url: "log/2"},
					},
					Status: bbpb.Status_STARTED,
				},
				Infra: buildInfra,
			})

			So(scFake.Data()["fakeNS/log/2"].GetDatagrams(), ShouldContain, "here's some stuff")
		})

	})
}

type buildItem struct {
	vers  int64
	build *bbpb.Build
}

type buildWaiter chan buildItem

func newBuildWaiter() buildWaiter {
	// 100 depth is cheap way to queue all changes
	return make(chan buildItem, 100)
}

func (b buildWaiter) waitFor(target int64) *bbpb.Build {
	var last int64
	for {
		select {
		case cur := <-b:
			last = cur.vers
			if cur.vers >= target {
				return cur.build
			}

		case <-time.After(50 * time.Millisecond):
			panic(fmt.Errorf("buildWaiter.waitFor timed out: last version %d", last))
		}
	}
}

func (b buildWaiter) sendFn(vers int64, build *bbpb.Build) {
	b <- buildItem{vers, build}
}

func TestStateSend(t *testing.T) {
	t.Parallel()

	Convey(`Test that OptSend works`, t, func() {
		lastBuildVers := newBuildWaiter()

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ts := timestamppb.New(testclock.TestRecentTimeUTC)
		st, ctx, err := Start(ctx, nil, OptSend(rate.Inf, lastBuildVers.sendFn))
		So(err, ShouldBeNil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()

		Convey(`startup causes no send`, func() {
			So(func() { lastBuildVers.waitFor(1) }, ShouldPanicLike, "timed out")
		})

		Convey(`adding a step sends`, func() {
			step, _ := StartStep(ctx, "something")
			So(lastBuildVers.waitFor(2), ShouldResembleProto, &bbpb.Build{
				Status:    bbpb.Status_STARTED,
				StartTime: ts,
				Input:     &bbpb.Build_Input{},
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_STARTED,
				},
				Steps: []*bbpb.Step{
					{
						Name:      "something",
						StartTime: ts,
						Status:    bbpb.Status_STARTED,
					},
				},
			})

			Convey(`closing a step sends`, func() {
				step.End(nil)
				So(lastBuildVers.waitFor(3), ShouldResembleProto, &bbpb.Build{
					Status:    bbpb.Status_STARTED,
					StartTime: ts,
					Input:     &bbpb.Build_Input{},
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_STARTED,
					},
					Steps: []*bbpb.Step{
						{
							Name:      "something",
							StartTime: ts,
							EndTime:   ts,
							Status:    bbpb.Status_SUCCESS,
						},
					},
				})
			})

			Convey(`manipulating a step sends`, func() {
				step.SetSummaryMarkdown("hey")
				So(lastBuildVers.waitFor(3), ShouldResembleProto, &bbpb.Build{
					Status:    bbpb.Status_STARTED,
					StartTime: ts,
					Input:     &bbpb.Build_Input{},
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_STARTED,
					},
					Steps: []*bbpb.Step{
						{
							Name:            "something",
							StartTime:       ts,
							Status:          bbpb.Status_STARTED,
							SummaryMarkdown: "hey",
						},
					},
				})
			})
		})

		Convey(`ending build sends`, func() {
			st.End(nil)
			st = nil
			So(lastBuildVers.waitFor(1), ShouldResembleProto, &bbpb.Build{
				Status:    bbpb.Status_SUCCESS,
				StartTime: ts,
				EndTime:   ts,
				Input:     &bbpb.Build_Input{},
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_SUCCESS,
				},
			})
		})

	})
}

func TestStateView(t *testing.T) {
	t.Parallel()

	Convey(`Test State View functionality`, t, func() {
		st, _, err := Start(context.Background(), nil)
		So(err, ShouldBeNil)
		defer func() { st.End(nil) }()

		Convey(`SetSummaryMarkdown`, func() {
			st.SetSummaryMarkdown("hi")

			So(st.buildPb.SummaryMarkdown, ShouldResemble, "hi")

			st.SetSummaryMarkdown("there")

			So(st.buildPb.SummaryMarkdown, ShouldResemble, "there")
		})

		Convey(`SetCritical`, func() {
			st.SetCritical(bbpb.Trinary_YES)

			So(st.buildPb.Critical, ShouldResemble, bbpb.Trinary_YES)

			st.SetCritical(bbpb.Trinary_NO)

			So(st.buildPb.Critical, ShouldResemble, bbpb.Trinary_NO)

			st.SetCritical(bbpb.Trinary_UNSET)

			So(st.buildPb.Critical, ShouldResemble, bbpb.Trinary_UNSET)
		})

		Convey(`SetGitilesCommit`, func() {
			st.SetGitilesCommit(&bbpb.GitilesCommit{
				Host: "a host",
			})

			So(st.buildPb.Output.GitilesCommit, ShouldResembleProto, &bbpb.GitilesCommit{
				Host: "a host",
			})

			st.SetGitilesCommit(nil)

			So(st.buildPb.Output.GitilesCommit, ShouldBeNil)
		})
	})
}
