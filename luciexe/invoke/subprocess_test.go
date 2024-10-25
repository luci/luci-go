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
	"io"
	"os"
	"os/signal"
	"path"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const (
	selfTestEnvvar          = "LUCIEXE_INVOKE_TEST"
	terminateExitCode       = 71
	unexpectedErrorExitCode = 97
)

func TestMain(m *testing.M) {
	switch os.Getenv(selfTestEnvvar) {
	case "":
		m.Run()
	case "exiterr":
		os.Exit(unexpectedErrorExitCode)
	case "hang":
		<-time.After(time.Minute)
		fmt.Fprintln(os.Stderr, "ERROR: TIMER ENDED")
		os.Exit(1)
	case "signal":
		fmt.Fprintf(os.Stderr, "signal subprocess started\n")
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
		fmt.Fprintf(os.Stderr, "touched %s\n", os.Args[1])
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

		data, err := io.ReadAll(os.Stdin)
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
			if err := os.WriteFile(*out, outData, 0666); err != nil {
				panic(err)
			}
		}

		os.Exit(0)
	}
}

func TestSubprocess(t *testing.T) {
	ftt.Run(`Subprocess`, t, func(t *ftt.Test) {
		ctx, o, tdir := commonOptions(t)

		o.Env.Set(selfTestEnvvar, "1")

		selfArgs := []string{os.Args[0]}

		t.Run(`defaults`, func(t *ftt.Test) {
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sp.Step, should.BeNil)
			build, err := sp.Wait()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{}))
		})

		t.Run(`exiterr`, func(t *ftt.Test) {
			o.Env.Set(selfTestEnvvar, "exiterr")
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sp.Step, should.BeNil)
			build, err := sp.Wait()
			assert.Loosely(t, err, should.ErrLike("exit status 97"))
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{}))
		})

		t.Run(`collect`, func(t *ftt.Test) {
			o.CollectOutput = true
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sp.Step, should.BeNil)
			build, err := sp.Wait()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, build, should.NotBeNil)
			assert.Loosely(t, build.SummaryMarkdown, should.Equal("hi"))
		})

		t.Run(`clear fields in initial build`, func(t *ftt.Test) {
			o.CollectOutput = true
			initialBuildTime := time.Date(2020, time.January, 2, 3, 4, 5, 6, time.UTC)
			ctx, _ := testclock.UseTime(ctx, initialBuildTime)

			inputBuild := &bbpb.Build{
				Id:              11,
				Status:          bbpb.Status_CANCELED,
				StatusDetails:   &bbpb.StatusDetails{Timeout: &bbpb.StatusDetails_Timeout{}},
				SummaryMarkdown: "Heyo!",
				EndTime:         timestamppb.New(time.Date(2020, time.January, 2, 3, 4, 5, 10, time.UTC)),
				UpdateTime:      timestamppb.New(time.Date(2020, time.January, 2, 3, 4, 5, 11, time.UTC)),
				Steps:           []*bbpb.Step{{Name: "Step cool"}},
				Tags:            []*bbpb.StringPair{{Key: "foo", Value: "bar"}},
				Output: &bbpb.Build_Output{
					Logs: []*bbpb.Log{{Name: "stdout"}},
				},
			}
			sp, err := Start(ctx, selfArgs, inputBuild, o)
			assert.Loosely(t, err, should.BeNil)
			build, err := sp.Wait()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, build, should.Resemble(&bbpb.Build{
				Id:              11,
				Status:          bbpb.Status_STARTED,
				SummaryMarkdown: "hi",
				CreateTime:      timestamppb.New(initialBuildTime),
				StartTime:       timestamppb.New(initialBuildTime),
				Tags:            []*bbpb.StringPair{{Key: "foo", Value: "bar"}},
			}))
		})

		t.Run(`cancel context`, func(t *ftt.Test) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			start := time.Now()

			o.Env.Set(selfTestEnvvar, "hang")
			sp, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o)
			assert.Loosely(t, err, should.BeNil)
			cancel()
			_, err = sp.Wait()
			assert.Loosely(t, err, should.ErrLike("waiting for luciexe"))

			assert.Loosely(t, time.Now(), should.HappenWithin(time.Second, start))
		})

		t.Run(`cancel context before Start`, func(t *ftt.Test) {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			_, err := Start(ctx, selfArgs, &bbpb.Build{Id: 1}, o)
			assert.Loosely(t, err, should.ErrLike("prior to starting subprocess: context canceled"))
		})

		t.Run(`deadline`, func(t *ftt.Test) {
			o.Env.Set(selfTestEnvvar, "signal")

			ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
			ctx, cancel := clock.WithTimeout(ctx, 130*time.Second)

			ctx, shutdown := lucictx.TrackSoftDeadline(ctx, 0)
			defer shutdown()

			readyFile := path.Join(tdir, "readyToCatchSignal")
			sp, err := Start(ctx, append(selfArgs, readyFile), &bbpb.Build{Id: 1}, o)
			assert.Loosely(t, err, should.BeNil)
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
				} else {
					time.Sleep(time.Second)
				}
			}
			defer os.Remove(readyFile)

			t.Run(`interrupt`, func(t *ftt.Test) {
				shutdown()

				bld, err := sp.Wait()
				assert.Loosely(t, err, should.UnwrapToErrStringLike("luciexe process is interrupted"))
				assert.Loosely(t, sp.cmd.ProcessState.ExitCode(), should.Equal(terminateExitCode))
				assert.Loosely(t, bld, should.Resemble(&bbpb.Build{}))
			})

			t.Run(`timeout`, func(t *ftt.Test) {
				tc.Add(100 * time.Second) // hits soft deadline

				bld, err := sp.Wait()
				assert.Loosely(t, err, should.UnwrapToErrStringLike("luciexe process timed out"))
				assert.Loosely(t, sp.cmd.ProcessState.ExitCode(), should.Equal(terminateExitCode))
				assert.Loosely(t, bld, should.Resemble(&bbpb.Build{
					StatusDetails: &bbpb.StatusDetails{Timeout: &bbpb.StatusDetails_Timeout{}},
				}))
			})

			t.Run(`closure`, func(t *ftt.Test) {
				cancel()

				bld, err := sp.Wait()
				assert.Loosely(t, err, should.UnwrapToErrStringLike("luciexe process's context is cancelled"))
				// The exit code for killed process varies on different platform.
				assert.Loosely(t, sp.cmd.ProcessState.ExitCode(), should.NotEqual(unexpectedErrorExitCode))
				assert.Loosely(t, bld, should.Resemble(&bbpb.Build{}))
			})
		})
	})
}
