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

package luciexe

import (
	"context"
	"fmt"
	"go.chromium.org/luci/common/retry/transient"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/milo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func newAnn(stepNames ...string) *milo.Step {
	ann := &milo.Step{
		Substep: make([]*milo.Step_Substep, len(stepNames)),
	}
	for i, n := range stepNames {
		ann.Substep[i] = &milo.Step_Substep{
			Substep: &milo.Step_Substep_Step{
				Step: &milo.Step{Name: n},
			},
		}
	}
	return ann
}

func newAnnBytes(stepNames ...string) []byte {
	ret, err := proto.Marshal(newAnn(stepNames...))
	if err != nil {
		panic(err)
	}
	return ret
}

func TestBuildUpdater(t *testing.T) {
	t.Parallel()

	FocusConvey(`buildUpdater`, t, func(c C) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Ensure tests don't hang.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)

		ctx, clk := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		bu := buildUpdater(func(ctx context.Context, build *pb.Build) error {
			return nil
		})

		FocusConvey(`Run`, func() {
			errC := make(chan error)
			done := make(chan struct{})
			builds := make(chan *pb.Build)

			start := func() {
				go func() {
					errC <- bu.Run(ctx, builds)
				}()
			}

			run := func(err1, err2 error) error {
				bu = buildUpdater(func(ctx context.Context, build *pb.Build) error {
					if build.Id == 1 {
						return err1
					}
					return err2
				})
				start()
				builds <- &pb.Build{Id: 1}
				builds <- &pb.Build{Id: 2}
				cancel()
				return <-errC
			}

			Convey("two successful requests", func() {
				So(run(nil, nil), ShouldBeNil)
			})

			Convey("first failed, second succeeded", func() {
				So(run(transient.Tag.Apply(fmt.Errorf("transient")), nil), ShouldBeNil)
			})

			Convey("first succeeded, second failed", func() {
				So(run(nil, fmt.Errorf("fatal")), ShouldErrLike, "fatal")
			})

			FocusConvey("minDistance", func() {
				var sleepDuration time.Duration
				open := true
				clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					if testclock.HasTags(t, "update-build-distance") {
						sleepDuration += d
						clk.Add(d)

						if open {
							close(done)
							open = false
						}

					}
				})

				start()
				builds <- &pb.Build{Id: 1}
				So(<-errC, ShouldBeNil)
				So(sleepDuration, ShouldBeGreaterThanOrEqualTo, time.Second)
			})

			Convey("errSleep", func() {
				attempt := 0
				clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					switch {
					case testclock.HasTags(t, "update-build-distance"):
						clk.Add(d)
					case testclock.HasTags(t, "update-build-error"):
						clk.Add(d)
						attempt++
						if attempt == 4 {
							builds <- &pb.Build{Id: 1}
						}
					}
				})

				bu = func(ctx context.Context, build *pb.Build) error {
					if build.Id == 1 {
						return transient.Tag.Apply(fmt.Errorf("err"))
					}

					close(done)
					return nil
				}

				start()
				builds <- &pb.Build{Id: 1}
				So(<-errC, ShouldBeNil)
			})

			Convey("first is fatal, second never occurs", func() {
				fatal := status.Error(codes.InvalidArgument, "too large")
				calls := 0
				bu = func(ctx context.Context, build *pb.Build) error {
					calls++
					return fatal
				}
				start()
				builds <- &pb.Build{Id: 1}
				cancel()
				So(errors.Unwrap(<-errC), ShouldEqual, fatal)
				So(calls, ShouldEqual, 1)
			})

			Convey("ctx is canceled", func() {
				start()
				builds <- &pb.Build{Id: 1}
				cancel()
				So(<-errC, ShouldBeNil)
			})
		})
	})
}
