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

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/luciexe/build/internal/testpb"
)

func mustNewStruct(data map[string]any) *structpb.Struct {
	ret, err := structpb.NewStruct(data)
	So(err, ShouldBeNil)
	return ret
}

func TestOutputProperties(t *testing.T) {
	// Intentionally forgo t.Parallel() due to global reservation structures.

	Convey(`Properties Output`, t, func() {
		defer propModifierReservations.clear()

		var setter func(context.Context, *testpb.Module)
		var merger func(context.Context, *testpb.Module)
		MakePropertyModifier("ns", &setter, &merger)

		lastBuildVers := newBuildWaiter()

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		opt := OptSend(rate.Inf, lastBuildVers.sendFn)

		Convey(`empty output`, func() {
			st, ctx, err := Start(ctx, nil, opt)
			So(err, ShouldBeNil)
			defer func() { st.End(nil) }()

			setter(ctx, nil)
			merger(ctx, nil)

			step, _ := StartStep(ctx, "wat")
			defer func() { step.End(nil) }()

			So(lastBuildVers.waitFor(2).Output.Properties, ShouldBeNil)
		})

		Convey(`populate output`, func() {
			st, ctx, err := Start(ctx, nil, opt)
			So(err, ShouldBeNil)
			defer func() { st.End(nil) }()

			setter(ctx, &testpb.Module{
				Field: "stuff",
			})

			So(lastBuildVers.waitFor(1).Output.Properties, ShouldResembleProto, mustNewStruct(map[string]any{
				"ns": map[string]any{
					"field": "stuff",
				},
			}))

			Convey(`merge`, func() {
				merger(ctx, &testpb.Module{
					JsonNameField: "things",
				})

				So(lastBuildVers.waitFor(2).Output.Properties, ShouldResembleProto, mustNewStruct(map[string]any{
					"ns": map[string]any{
						"field": "stuff",
						"$cool": "things",
					},
				}))
			})
		})

		Convey(`top level output`, func() {
			var topWriter func(*testpb.TopLevel)
			var topMerger func(*testpb.TopLevel)

			st, ctx, err := Start(ctx, nil, opt, OptOutputProperties(&topWriter, &topMerger))
			So(err, ShouldBeNil)
			defer func() { st.End(nil) }()

			setter(ctx, &testpb.Module{
				Field: "stuff",
			})

			topMerger(&testpb.TopLevel{
				JsonNameField: "nice",
			})
			topWriter(&testpb.TopLevel{ // overwrites previous value
				Field: "woot",
			})

			So(lastBuildVers.waitFor(3).Output.Properties, ShouldResembleProto, mustNewStruct(map[string]any{
				"field": "woot",
				"ns": map[string]any{
					"field": "stuff",
				},
			}))
		})

		Convey(`top level conflicts with reservations`, func() {
			var badWriter func(context.Context, *testpb.Module)
			MakePropertyModifier("$cool", &badWriter, nil)

			var topWriter func(*testpb.TopLevel)
			var topMerger func(*testpb.TopLevel)
			_, _, err := Start(ctx, nil, opt, OptOutputProperties(&topWriter, &topMerger))
			So(err, ShouldErrLike, `output property "$cool" conflicts with field`)
		})

		Convey(`late binding panics`, func() {
			var topMerger func(*testpb.TopLevel)

			st, ctx, err := Start(ctx, nil, opt, OptOutputProperties(nil, &topMerger))
			So(err, ShouldBeNil)
			defer func() { st.End(nil) }()

			var badSetter func(context.Context, *testpb.Module)
			MakePropertyModifier("field", &badSetter, nil) // note that "field" would overlap TopLevel.Field.

			So(func() {
				badSetter(ctx, &testpb.Module{
					Field: "whoops!",
				})
			}, ShouldPanicLike,
				`MakePropertyModifier[writing] for namespace "field"`,
				`was created after the current build started`,
				`luciexe/build/output_properties_test.go`)

		})

	})

}
