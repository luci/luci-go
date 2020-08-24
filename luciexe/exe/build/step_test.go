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
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/system/environ"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSteps(t *testing.T) {
	t.Parallel()

	Convey(`test steps`, t, func() {
		ptime := google.NewTimestamp(testclock.TestTimeUTC)

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		ctx = environ.With(ctx, nil)

		Convey(`basic`, func() {
			lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					return s.Modify(ctx, func(sv *StepView) error {
						sv.SummaryMarkdown = "hi"
						return nil
					})
				})
			})
			So(err, ShouldBeNil)

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
			lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					return WithStep(ctx, "bar", func(ctx context.Context, s *Step) error {
						return nil
					})
				})
			})
			So(err, ShouldBeNil)

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
				{
					Name:      "foo|bar",
					Status:    bbpb.Status_SUCCESS,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`duplicate step`, func() {
			lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
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
				return nil
			})
			So(err, ShouldBeNil)

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
			lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					So(s.step.Status, ShouldResemble, bbpb.Status_SCHEDULED)
					s.EnsureStarted(ctx)
					So(s.step.Status, ShouldResemble, bbpb.Status_STARTED)
					s.EnsureStarted(ctx)
					So(s.step.Status, ShouldResemble, bbpb.Status_STARTED)
					return nil
				})
			})
			So(err, ShouldBeNil)

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
			lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					return errors.New("borken")
				})
			})
			So(err, ShouldErrLike, "borken")

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
			lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					return errors.New("borken", StatusInfraFailure)
				})
			})
			So(err, ShouldErrLike, "borken")

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
			lastBuild, recovered, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					panic("NORP")
				})
			})
			So(err, ShouldEqual, ErrCallbackPaniced)
			So(recovered, ShouldEqual, "NORP")

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_INFRA_FAILURE,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`panic (error)`, func() {
			lastBuild, recovered, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				return WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					// note that tag does not matter; panics are alwyas INFRA_FAILURE.
					// Users should handle their own panics if they want to swallow them.
					panic(errors.New("NORP", StatusSuccess))
				})
			})
			So(err, ShouldEqual, ErrCallbackPaniced)
			So(recovered, ShouldErrLike, "NORP")

			So(lastBuild.Steps, ShouldResemble, []*bbpb.Step{
				{
					Name:      "foo",
					Status:    bbpb.Status_INFRA_FAILURE,
					StartTime: ptime,
					EndTime:   ptime,
				},
			})
		})

		Convey(`keeping step out of scope`, func() {
			Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				var cheats *Step
				WithStep(ctx, "foo", func(ctx context.Context, s *Step) error {
					cheats = s
					return nil
				})
				So(cheats.Modify(ctx, nil), ShouldErrLike, ErrStepClosed)
				return nil
			})
		})

		Convey(`bad step name`, func() {
			Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				So(WithStep(ctx, "foo|wat", nil), ShouldErrLike, "invalid name")
				So(WithStep(ctx, "", nil), ShouldErrLike, "invalid name")
				return nil
			})
		})

		Convey(`canceled context`, func() {
			Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
				ctx, cancel := context.WithCancel(ctx)
				cancel()

				So(WithStep(ctx, "foo", nil), ShouldErrLike, context.Canceled)
				return nil
			})
		})

	})
}
