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

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey(`State`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		nowpb := timestamppb.New(testclock.TestRecentTimeUTC)
		st, ctx := Start(ctx, nil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()

		Convey(`StartStep`, func() {
			step, _ := StartStep(ctx, "some step")
			defer func() { step.End(nil) }()

			So(st.buildPb.Steps, assertions.ShouldResembleProto, []*bbpb.Step{
				{Name: "some step", StartTime: nowpb, Status: bbpb.Status_STARTED},
			})
		})

		Convey(`End`, func() {
			Convey(`cannot End twice`, func() {
				st.End(nil)
				So(func() { st.End(nil) }, assertions.ShouldPanicLike, "cannot mutate ended build")
				st = nil
			})
		})
	})
}

func TestStateLogging(t *testing.T) {
	Convey(`State logging`, t, func() {
		lc := streamclient.NewFake("fakeNS")
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		st, ctx := Start(ctx, &bbpb.Build{
			Output: &bbpb.Build_Output{
				Logs: []*bbpb.Log{
					{Name: "something"},
					{Name: "other"},
				},
			},
		}, OptLogsink(lc.Client))
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

			So(st.buildPb, assertions.ShouldResembleProto, &bbpb.Build{
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_STARTED,
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "something"},
						{Name: "other"},
						{Name: "some log", Url: "log/2"},
					},
				},
			})

			So(lc.GetFakeData()["fakeNS/log/2"].GetStreamData(), ShouldContainSubstring, "here's some stuff")
		})

		Convey(`can open datagram logs`, func() {
			log := st.LogDatagram("some log")
			log.WriteDatagram([]byte("here's some stuff"))

			So(st.buildPb, assertions.ShouldResembleProto, &bbpb.Build{
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_STARTED,
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{
						{Name: "something"},
						{Name: "other"},
						{Name: "some log", Url: "log/2"},
					},
				},
			})

			So(lc.GetFakeData()["fakeNS/log/2"].GetDatagrams(), ShouldContain, "here's some stuff")
		})

	})
}
