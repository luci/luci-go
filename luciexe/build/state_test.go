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

	"golang.org/x/time/rate"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

func TestState(t *testing.T) {
	t.Parallel()

	ftt.Run(`State`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()

		t.Run(`StartStep`, func(t *ftt.Test) {
			step, _ := StartStep(ctx, "some step")
			defer func() { step.End(nil) }()

			assert.Loosely(t, st.buildPb.Steps, should.Match([]*bbpb.Step{
				{Name: "some step", StartTime: nowpb, Status: bbpb.Status_STARTED},
			}))

			t.Run(`child with explicit parent`, func(t *ftt.Test) {
				child, _ := step.StartStep(ctx, "child step")
				defer func() { child.End(nil) }()

				assert.Loosely(t, st.buildPb.Steps, should.Match([]*bbpb.Step{
					{Name: "some step", StartTime: nowpb, Status: bbpb.Status_STARTED},
					{Name: "some step|child step", StartTime: nowpb, Status: bbpb.Status_STARTED},
				}))
			})
		})

		t.Run(`End`, func(t *ftt.Test) {
			t.Run(`cannot End twice`, func(t *ftt.Test) {
				st.End(nil)
				assert.Loosely(t, func() { st.End(nil) }, should.PanicLike("cannot mutate ended build"))
				st = nil
			})
		})

		t.Run(`Build.Infra()`, func(t *ftt.Test) {
			build := st.Build()
			assert.Loosely(t, build.GetInfra().Buildbucket.ServiceConfigRevision, should.Match("I am a string"))
			build.GetInfra().Buildbucket.ServiceConfigRevision = "narf"
			assert.Loosely(t, origInfra.Buildbucket.ServiceConfigRevision, should.Match("I am a string"))
		})
	})

	ftt.Run(`nil build`, t, func(t *ftt.Test) {
		st, _, err := Start(context.Background(), nil)
		assert.Loosely(t, err, should.BeNil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()
		assert.Loosely(t, st.Build().GetInfra(), should.BeNil)
	})
}

func TestStateLogging(t *testing.T) {
	t.Parallel()

	ftt.Run(`State logging`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		defer func() { st.End(nil) }()
		assert.Loosely(t, st, should.NotBeNil)

		t.Run(`existing logs are reserved`, func(t *ftt.Test) {
			assert.Loosely(t, st.logNames.pool, should.Match(map[string]int{
				"something": 1,
				"other":     1,
			}))
		})

		t.Run(`can open logs`, func(t *ftt.Test) {
			log := st.Log("some log")
			fmt.Fprintln(log, "here's some stuff")
			assert.Loosely(t, st.buildPb, should.Match(&bbpb.Build{
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
			}))

			assert.Loosely(t, scFake.Data()["fakeNS/log/2"].GetStreamData(), should.ContainSubstring("here's some stuff"))

			// Check the link.
			wantLink := "https://logs.chromium.org/logs/example/builds/8888888888/+/fakeNS/log/2"
			assert.Loosely(t, log.UILink(), should.Equal(wantLink))
		})

		t.Run(`can open datagram logs`, func(t *ftt.Test) {
			log := st.LogDatagram("some log")
			log.WriteDatagram([]byte("here's some stuff"))

			assert.Loosely(t, st.buildPb, should.Match(&bbpb.Build{
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
			}))

			assert.Loosely(t, scFake.Data()["fakeNS/log/2"].GetDatagrams(), should.Contain("here's some stuff"))
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

	ftt.Run(`Test that OptSend works`, t, func(t *ftt.Test) {
		lastBuildVers := newBuildWaiter()

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ts := timestamppb.New(testclock.TestRecentTimeUTC)
		st, ctx, err := Start(ctx, nil, OptSend(rate.Inf, lastBuildVers.sendFn))
		assert.Loosely(t, err, should.BeNil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()

		t.Run(`startup causes no send`, func(t *ftt.Test) {
			assert.Loosely(t, func() { lastBuildVers.waitFor(1) }, should.PanicLike("timed out"))
		})

		t.Run(`adding a step sends`, func(t *ftt.Test) {
			step, _ := StartStep(ctx, "something")
			assert.Loosely(t, lastBuildVers.waitFor(2), should.Match(&bbpb.Build{
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
			}))

			t.Run(`closing a step sends`, func(t *ftt.Test) {
				step.End(nil)
				assert.Loosely(t, lastBuildVers.waitFor(3), should.Match(&bbpb.Build{
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
				}))
			})

			t.Run(`manipulating a step sends`, func(t *ftt.Test) {
				step.SetSummaryMarkdown("hey")
				assert.Loosely(t, lastBuildVers.waitFor(3), should.Match(&bbpb.Build{
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
				}))
			})
		})

		t.Run(`ending build sends`, func(t *ftt.Test) {
			st.End(nil)
			st = nil
			assert.Loosely(t, lastBuildVers.waitFor(1), should.Match(&bbpb.Build{
				Status:    bbpb.Status_SUCCESS,
				StartTime: ts,
				EndTime:   ts,
				Input:     &bbpb.Build_Input{},
				Output: &bbpb.Build_Output{
					Status: bbpb.Status_SUCCESS,
				},
			}))
		})
	})
}

func TestStateView(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test State View functionality`, t, func(t *ftt.Test) {
		st, _, err := Start(context.Background(), nil)
		assert.Loosely(t, err, should.BeNil)
		defer func() { st.End(nil) }()

		t.Run(`SetSummaryMarkdown`, func(t *ftt.Test) {
			st.SetSummaryMarkdown("hi")

			assert.Loosely(t, st.buildPb.SummaryMarkdown, should.Match("hi"))

			st.SetSummaryMarkdown("there")

			assert.Loosely(t, st.buildPb.SummaryMarkdown, should.Match("there"))
		})

		t.Run(`SetCritical`, func(t *ftt.Test) {
			st.SetCritical(bbpb.Trinary_YES)

			assert.Loosely(t, st.buildPb.Critical, should.Match(bbpb.Trinary_YES))

			st.SetCritical(bbpb.Trinary_NO)

			assert.Loosely(t, st.buildPb.Critical, should.Match(bbpb.Trinary_NO))

			st.SetCritical(bbpb.Trinary_UNSET)

			assert.Loosely(t, st.buildPb.Critical, should.Match(bbpb.Trinary_UNSET))
		})

		t.Run(`SetGitilesCommit`, func(t *ftt.Test) {
			st.SetGitilesCommit(&bbpb.GitilesCommit{
				Host: "a host",
			})

			assert.Loosely(t, st.buildPb.Output.GitilesCommit, should.Match(&bbpb.GitilesCommit{
				Host: "a host",
			}))

			st.SetGitilesCommit(nil)

			assert.Loosely(t, st.buildPb.Output.GitilesCommit, should.BeNil)
		})
	})
}
