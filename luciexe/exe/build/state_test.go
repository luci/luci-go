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
	"strings"
	"sync"
	"testing"

	"golang.org/x/time/rate"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto/google"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	Convey(`test build`, t, func() {
		var sentBuild *bbpb.Build

		ctx := context.Background()

		sink := Sink{
			SendLimit: rate.Inf,
			SendFunc: func(b *bbpb.Build) error {
				sentBuild = b
				return nil
			},
		}

		Convey(`basic modify`, func() {
			lastBuild, _, err := sink.Use(ctx, func(ctx context.Context, state *State) error {
				return state.Modify(func(b *View) error {
					b.SummaryMarkdown = "hi"
					return nil
				})
			})
			So(err, ShouldBeNil)

			So(lastBuild.SummaryMarkdown, ShouldEqual, "hi")
			So(sentBuild, ShouldResembleProto, lastBuild)
		})

		Convey(`parallel modify`, func() {
			expectedVals := stringset.New(100)

			lastBuild, _, err := sink.Use(ctx, func(ctx context.Context, state *State) error {
				var wg sync.WaitGroup
				for i := 0; i < 100; i++ {
					s := fmt.Sprintf("%d", i)
					expectedVals.Add(s)
					wg.Add(1)
					go func() {
						defer wg.Done()
						state.Modify(func(b *View) error {
							b.SummaryMarkdown += s + "\n"
							return nil
						})
					}()
				}
				wg.Wait()
				return nil
			})
			So(err, ShouldBeNil)

			actualVals := stringset.NewFromSlice(strings.Split(lastBuild.SummaryMarkdown, "\n")...)
			actualVals.Del("")

			So(actualVals, ShouldResemble, expectedVals)
		})

		Convey(`keep state ref past end`, func() {
			var cheats *State
			_, _, err := sink.Use(ctx, func(ctx context.Context, state *State) error {
				cheats = state
				return nil
			})
			So(err, ShouldBeNil)
			So(cheats.Modify(nil), ShouldEqual, ErrBuildDetached)
		})

	})

	Convey(`test nil build`, t, func() {
		ptime := google.NewTimestamp(testclock.TestTimeUTC)

		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)

		lastBuild, _, err := Sink{}.Use(ctx, func(ctx context.Context, state *State) error {
			state.Modify(func(bv *View) error {
				bv.SummaryMarkdown = "hello"
				return nil
			})

			return nil
		})
		So(err, ShouldBeNil)

		So(lastBuild, ShouldResembleProto, &bbpb.Build{
			Output:          &bbpb.Build_Output{},
			Status:          bbpb.Status_SUCCESS,
			EndTime:         ptime,
			SummaryMarkdown: "hello",
		})

	})
}
