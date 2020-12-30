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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPropertiesNoop(t *testing.T) {
	// NOTE: We use bbpb.Build as our "properties" message to avoid having to have
	// a special 'testing' message. The actual contents of the message doesn't
	// matter, just that it's a proto.Message

	Convey(`Properties Noop`, t, func() {
		// reset `reservations` after each test
		defer func() {
			for _, v := range reservations {
				v.mu.Lock()
				v.reservations = map[string]string{}
				v.mu.Unlock()
			}
		}()

		Convey(`MakePropertyReader`, func() {
			Convey(`simple`, func() {
				var fn func(context.Context) (*bbpb.Build, error)
				MakePropertyReader("ns", &fn)

				So(fn, ShouldNotBeNil)

				msg, err := fn(context.Background())
				So(err, ShouldBeNil)
				So(msg, ShouldResembleProto, &bbpb.Build{})
			})

			Convey(`bad args`, func() {
				Convey(`empty ns`, func() {
					So(func() {
						MakePropertyReader("", nil)
					}, ShouldPanicLike, "empty namespace not allowed")
				})

				Convey(`duplicate namespace`, func() {
					var fn func(context.Context) (*bbpb.Build, error)
					MakePropertyReader("ns", &fn)

					So(func() {
						MakePropertyReader("ns", &fn)
					}, ShouldPanicLike, "cannot reserve PropertyReader namespace")
				})

				Convey(`not pointer`, func() {
					var fn func(context.Context) (*bbpb.Build, error)
					So(func() {
						MakePropertyReader("ns", fn)
					}, ShouldPanicLike, "fnptr is not")
				})

				Convey(`not function`, func() {
					var fn int
					So(func() {
						MakePropertyReader("ns", &fn)
					}, ShouldPanicLike, "fnptr is not")
				})

				Convey(`bad in arg count`, func() {
					var fn func()
					So(func() {
						MakePropertyReader("ns", &fn)
					}, ShouldPanicLike, "fnptr is not")
				})

				Convey(`bad out arg count`, func() {
					var fn func(context.Context)
					So(func() {
						MakePropertyReader("ns", &fn)
					}, ShouldPanicLike, "fnptr is not")
				})

				Convey(`bad in arg type`, func() {
					var fn func(int)
					So(func() {
						MakePropertyReader("ns", &fn)
					}, ShouldPanicLike, "fnptr is not")
				})

				Convey(`bad proto type`, func() {
					// bbpb.Build doesn't implement proto.Message (missing *)
					var fn func(context.Context) (bbpb.Build, error)
					So(func() {
						MakePropertyReader("ns", &fn)
					}, ShouldPanicLike, "fnptr is not")
				})
			})
		})

		Convey(`MakePropertyModifier`, func() {
			ctx := memlogger.Use(context.Background())
			logs := logging.Get(ctx).(*memlogger.MemLogger)

			Convey(`working`, func() {
				var writer func(context.Context, *bbpb.Build)
				var merger func(context.Context, *bbpb.Build)
				So(func() {
					MakePropertyModifier("ns", &writer, &merger)
				}, ShouldNotPanic)

				Convey(`write nil`, func() {
					writer(ctx, nil)
					So(logs.Messages(), ShouldBeEmpty)
				})

				Convey(`merge nil`, func() {
					merger(ctx, nil)
					So(logs.Messages(), ShouldBeEmpty)
				})

				Convey(`write msg`, func() {
					writer(ctx, &bbpb.Build{SummaryMarkdown: "hidey-ho"})
					So(logs, memlogger.ShouldHaveLog, logging.Info,
						"writing output property \"ns\": \"{\\\"summaryMarkdown\\\":\\\"hidey-ho\\\"}\"")
				})

				Convey(`merge msg`, func() {
					merger(ctx, &bbpb.Build{SummaryMarkdown: "hidey-ho"})
					memlogger.MustDumpStdout(ctx)
					So(logs, memlogger.ShouldHaveLog, logging.Info, "merging output property \"ns\"")
				})
			})
		})
	})
}
