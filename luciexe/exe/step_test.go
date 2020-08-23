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

package exe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
	"golang.org/x/time/rate"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSteps(t *testing.T) {
	t.Parallel()

	Convey(`test steps`, t, func() {
		client := streamclient.NewFake("u")
		var lastBuild *bbpb.Build

		ptime := google.NewTimestamp(testclock.TestTimeUTC)

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		ctx = environ.With(ctx, nil)

		ctx, build := SinkBuildUpdates(ctx, &bbpb.Build{}, client.Client, rate.Inf, func(b *bbpb.Build) {
			lastBuild = b
		})

		Convey(`basic`, func() {
			err := WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				return s.Modify(ctx, func(sv *StepView) error {
					So(environ.FromCtx(ctx), ShouldResemble, environ.Env{
						luciexe.LogdogNamespaceEnv: "u/s/0/u",
					})
					sv.SummaryMarkdown = "hi"
					return nil
				})
			})
			So(err, ShouldBeNil)

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:            "foo",
					SummaryMarkdown: "hi",
					Status:          bbpb.Status_SUCCESS,
					StartTime:       ptime,
					EndTime:         ptime,
				},
			})
		})

		Convey(`sub step`, func() {
			err := WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				logging.Infof(ctx, "look, ma, free logs!")

				return WithStep(ctx, "bar", func(ctx context.Context, s *Step) error {
					logging.Infof(ctx, "and nested logs, too!")
					return nil
				})
			})
			So(err, ShouldBeNil)

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
					Logs: []*bbpb.Log{
						{Name: "log", Url: "s/0/l/log"},
					},
				},
				{
					Name:      "foo|bar",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
					Logs: []*bbpb.Log{
						{Name: "log", Url: "s/1/l/log"},
					},
				},
			})

			So(client.GetFakeData()["u/s/0/l/log"].GetStreamData(),
				ShouldContainSubstring, "look, ma, free logs!")
			So(client.GetFakeData()["u/s/1/l/log"].GetStreamData(),
				ShouldContainSubstring, "and nested logs, too!")
		})

		Convey(`duplicate step`, func() {
			err := WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				return nil
			})
			So(err, ShouldBeNil)
			err = WithStep(ctx, "foo (2)", func(ctx context.Context, s *Step) error {
				return nil
			})
			So(err, ShouldBeNil)
			err = WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				return nil
			})
			So(err, ShouldBeNil)
			err = WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				return nil
			})
			So(err, ShouldBeNil)
			err = WithStep(ctx, "foo (3)", func(ctx context.Context, s *Step) error {
				return nil
			})
			So(err, ShouldBeNil)

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
				{
					Name:      "foo (2)",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
				{
					Name:      "foo (3)",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
				{
					Name:      "foo (4)",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
				{
					Name:      "foo (3) (2)",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`explicit start`, func() {
			err := WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				So(s.step.Status, ShouldResemble, bbpb.Status_SCHEDULED)
				s.EnsureStarted(ctx)
				So(s.step.Status, ShouldResemble, bbpb.Status_STARTED)
				s.EnsureStarted(ctx)
				So(s.step.Status, ShouldResemble, bbpb.Status_STARTED)
				return nil
			})
			So(err, ShouldBeNil)

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`error`, func() {
			err := WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				return errors.New("borken")
			})
			So(err, ShouldErrLike, "borken")

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_FAILURE,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`tagged error`, func() {
			err := WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				return errors.New("borken", StatusInfraFailure)
			})
			So(err, ShouldErrLike, "borken")

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_INFRA_FAILURE,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`panic (non error)`, func() {
			So(func() {
				WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					panic("NORP")
				})
			}, ShouldPanic)

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_INFRA_FAILURE,
					StartTime: ptime,
					EndTime:   ptime,
					Logs: []*bbpb.Log{
						{Name: "log", Url: "s/0/l/log"},
					},
				},
			})

			So(client.GetFakeData()["u/s/0/l/log"].GetStreamData(),
				ShouldContainSubstring, "NORP")
		})

		Convey(`panic (error)`, func() {
			So(func() {
				WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					// note that tag does not matter; panics are alwyas INFRA_FAILURE.
					// Users should handle their own panics if they want to swallow them.
					panic(errors.New("NORP", StatusSuccess))
				})
			}, ShouldPanic)

			build.Detach(ctx)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_INFRA_FAILURE,
					StartTime: ptime,
					EndTime:   ptime,
					Logs: []*bbpb.Log{
						{Name: "log", Url: "s/0/l/log"},
					},
				},
			})

			So(client.GetFakeData()["u/s/0/l/log"].GetStreamData(),
				ShouldContainSubstring, "NORP")
		})

		Convey(`keeping step out of scope`, func() {
			var cheats *Step
			WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
				cheats = s
				return nil
			})

			So(cheats.Modify(ctx, nil), ShouldErrLike, ErrStepClosed)
			_, err := cheats.Log(ctx, "nope")
			So(err, ShouldErrLike, ErrStepClosed)
			_, err = cheats.LogBinary(ctx, "nope")
			So(err, ShouldErrLike, ErrStepClosed)
			_, err = cheats.LogDatagram(ctx, "nope")
			So(err, ShouldErrLike, ErrStepClosed)
		})

		Convey(`bad step name`, func() {
			So(WithStep(ctx, "foo|wat", nil), ShouldErrLike, "invalid name")
			So(WithStep(ctx, "", nil), ShouldErrLike, "invalid name")
		})

		Convey(`canceled context`, func() {
			ctx, cancel := context.WithCancel(ctx)
			cancel()

			So(WithStep(ctx, "foo", nil), ShouldErrLike, context.Canceled)
		})

		Convey(`logs`, func() {
			Convey(`text`, func() {
				WithStep(ctx, "some step", func(ctx context.Context, s *Step) error {
					l, err := s.Log(ctx, "cool_log")
					So(err, ShouldBeNil)

					fmt.Fprintf(l, "this is neat!\n")
					fmt.Fprintf(l, "with some lines\n")
					l.Close()

					So(client.GetFakeData()["u/s/0/l/cool_log"].GetStreamData(),
						ShouldResemble, "this is neat!\nwith some lines\n")
					return nil
				})

				build.Detach(ctx)

				So(lastBuild.Steps[0].Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "cool_log",
					Url:  "s/0/l/cool_log",
				})
			})

			Convey(`binary`, func() {
				WithStep(ctx, "some step", func(ctx context.Context, s *Step) error {
					l, err := s.LogBinary(ctx, "cool_log")
					So(err, ShouldBeNil)

					fmt.Fprintf(l, "this is neat!\n")
					fmt.Fprintf(l, "with some lines\n")
					l.Close()

					So(client.GetFakeData()["u/s/0/l/cool_log"].GetStreamData(),
						ShouldResemble, "this is neat!\nwith some lines\n")
					return nil
				})

				build.Detach(ctx)

				So(lastBuild.Steps[0].Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "cool_log",
					Url:  "s/0/l/cool_log",
				})
			})

			Convey(`file`, func() {
				WithStep(ctx, "some step", func(ctx context.Context, s *Step) error {
					fname := filepath.Join(t.TempDir(), "some_file")
					f, err := os.Create(fname)
					So(err, ShouldBeNil)
					fmt.Fprintf(f, "this is neat!\n")
					fmt.Fprintf(f, "with some lines\n")
					So(f.Close(), ShouldBeNil)

					So(s.LogFile(ctx, "a log", fname), ShouldBeNil)
					So(s.LogFile(ctx, "another log", fname), ShouldBeNil)

					So(client.GetFakeData()["u/s/0/l/0"].GetStreamData(),
						ShouldResemble, "this is neat!\nwith some lines\n")
					So(client.GetFakeData()["u/s/0/l/1"].GetStreamData(),
						ShouldResemble, "this is neat!\nwith some lines\n")
					return nil
				})

				build.Detach(ctx)

				So(lastBuild.Steps[0].Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "a log",
					Url:  "s/0/l/0",
				})
				So(lastBuild.Steps[0].Logs[1], ShouldResembleProto, &bbpb.Log{
					Name: "another log",
					Url:  "s/0/l/1",
				})
			})

			Convey(`datagram`, func() {
				WithStep(ctx, "some step", func(ctx context.Context, s *Step) error {
					l, err := s.LogDatagram(ctx, "dgram")
					So(err, ShouldBeNil)

					l.WriteDatagram([]byte("this is neat!"))
					l.WriteDatagram([]byte("with some datagrams"))
					l.Close()

					So(client.GetFakeData()["u/s/0/l/dgram"].GetDatagrams(), ShouldResemble, []string{
						"this is neat!",
						"with some datagrams",
					})
					return nil
				})

				build.Detach(ctx)

				So(lastBuild.Steps[0].Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "dgram",
					Url:  "s/0/l/dgram",
				})
			})

		})

	})
}
