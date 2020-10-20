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

package invoke

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const (
	selfTestEnvvar          = "LUCIEXE_INVOKE_TEST"
	terminateExitCode       = 71
	unexpectedErrorExitCode = 97
)

func init() {
	switch os.Getenv(selfTestEnvvar) {
	case "":
	case "hang":
		<-time.After(time.Minute)
		fmt.Fprintln(os.Stderr, "ERROR: TIMER ENDED")
		os.Exit(1)
	case "signal":
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, signals.Interrupts()...)
		touch := func(name string) error {
			f, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0644)
			if err != nil {
				return err
			}
			return f.Close()
		}
		if err := touch(os.Args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: creating file %s\n", err)
			os.Exit(unexpectedErrorExitCode)
		}
		select {
		case <-signalCh:
			os.Exit(terminateExitCode)
		case <-time.After(time.Minute):
			fmt.Fprintln(os.Stderr, "ERROR: Timeout waiting for Signal")
			os.Exit(unexpectedErrorExitCode)
		}
	default:
		out := flag.String("output", "", "write the output here")
		flag.Parse()

		data, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			panic(err)
		}

		in := &bbpb.Build{}
		if err := proto.Unmarshal(data, in); err != nil {
			panic(err)
		}
		in.SummaryMarkdown = "hi"

		if *out != "" {
			outData, err := proto.Marshal(in)
			if err != nil {
				panic(err)
			}
			if err := ioutil.WriteFile(*out, outData, 0666); err != nil {
				panic(err)
			}
		}

		os.Exit(0)
	}
}

func TestSubprocess(t *testing.T) {
	Convey(`Subprocess`, t, func() {
		ctx, o, tdir, closer := commonOptions()
		defer closer()

		o.Env.Set(selfTestEnvvar, "1")

		selfArgs := []string{os.Args[0]}

		Convey(`defaults`, func() {
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o, nil)
			So(err, ShouldBeNil)
			So(sp.Step, ShouldBeNil)
			build, err := sp.Wait()
			So(err, ShouldBeNil)
			So(build, ShouldBeNil)
		})

		Convey(`collect`, func() {
			o.CollectOutput = true
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o, nil)
			So(err, ShouldBeNil)
			So(sp.Step, ShouldBeNil)
			build, err := sp.Wait()
			So(err, ShouldBeNil)
			So(build, ShouldNotBeNil)
			So(build.SummaryMarkdown, ShouldEqual, "hi")
		})

		Convey(`clear fields in initial build`, func() {
			o.CollectOutput = true
			initialBuildTime := time.Date(2020, time.January, 2, 3, 4, 5, 6, time.UTC)
			ctx, _ := testclock.UseTime(ctx, initialBuildTime)
			inputBuild := &bbpb.Build{
				Id:              11,
				Status:          bbpb.Status_CANCELED,
				StatusDetails:   &bbpb.StatusDetails{Timeout: &bbpb.StatusDetails_Timeout{}},
				SummaryMarkdown: "Heyo!",
				CreateTime:      timestamppb.New(time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)),
				StartTime:       timestamppb.New(time.Date(2020, time.January, 2, 3, 4, 5, 1, time.UTC)),
				EndTime:         timestamppb.New(time.Date(2020, time.January, 2, 3, 4, 5, 10, time.UTC)),
				UpdateTime:      timestamppb.New(time.Date(2020, time.January, 2, 3, 4, 5, 11, time.UTC)),
				Steps:           []*bbpb.Step{{Name: "Step cool"}},
				Tags:            []*bbpb.StringPair{{Key: "foo", Value: "bar"}},
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{{Name: "stdout"}},
				},
			}
			sp, err := Start(ctx, selfArgs, inputBuild, o, nil)
			So(err, ShouldBeNil)
			build, err := sp.Wait()
			So(err, ShouldBeNil)
			So(build, ShouldResembleProto, &bbpb.Build{
				Id:              11,
				Status:          bbpb.Status_STARTED,
				SummaryMarkdown: "hi",
				CreateTime:      timestamppb.New(initialBuildTime),
				StartTime:       timestamppb.New(initialBuildTime),
			})
		})

		Convey(`cancel context`, func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			start := time.Now()

			o.Env.Set(selfTestEnvvar, "hang")
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o, nil)
			So(err, ShouldBeNil)
			cancel()
			_, err = sp.Wait()
			So(err, ShouldErrLike, "waiting for luciexe")

			So(time.Now(), ShouldHappenWithin, time.Second, start)
		})

		Convey(`deadline`, func() {
			o.Env.Set(selfTestEnvvar, "signal")
			deadlineEventCh := make(chan lucictx.DeadlineEvent, 1)
			sendEvent := func(evt lucictx.DeadlineEvent) {
				deadlineEventCh <- evt
			}
			readyFile := path.Join(tdir, "readyToCatchSignal")
			sp, err := Start(ctx, append(selfArgs, readyFile), &bbpb.Build{Id: 1}, o, deadlineEventCh)
			So(err, ShouldBeNil)
			timer := time.After(time.Minute)
			for {
				select {
				case <-timer:
					panic("subprocess is never ready to catch signal")
				default:
					_, err = os.Stat(readyFile)
				}
				if err == nil {
					break
				}
			}
			defer os.Remove(readyFile)
			Convey(`interrupt`, func() {
				go sendEvent(lucictx.InterruptEvent)
				_, err = sp.Wait()
				So(err, ShouldErrLike, "luciexe process is interrupted")
				So(sp.cmd.ProcessState.ExitCode(), ShouldEqual, terminateExitCode)
			})
			Convey(`timeout`, func() {
				go sendEvent(lucictx.TimeoutEvent)
				_, err = sp.Wait()
				So(err, ShouldErrLike, "luciexe process times out")
				So(sp.cmd.ProcessState.ExitCode(), ShouldEqual, terminateExitCode)
			})
			Convey(`closure`, func() {
				go sendEvent(lucictx.ClosureEvent)
				_, err = sp.Wait()
				So(err, ShouldErrLike, "luciexe process's context is cancelled")
				// The exit code for killed process varies on different platform.
				So(sp.cmd.ProcessState.ExitCode(), ShouldNotEqual, unexpectedErrorExitCode)
			})
		})
	})
}
