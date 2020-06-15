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

package cfgcache

import (
	"context"
	"testing"
	"time"

	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb" // some "random" v2 proto

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/clock/testclock"
	configpb "go.chromium.org/luci/common/proto/config" // some "random" v1 proto
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testEntry = Register(&Entry{
	Path: "path.cfg",
	Type: (*durationpb.Duration)(nil),
	Validator: func(ctx *validation.Context, msg proto.Message) error {
		dur := msg.(*durationpb.Duration)
		if dur.Seconds == 0 {
			ctx.Errorf("must be positive")
		}
		return nil
	},
})

func TestProtoReflection(t *testing.T) {
	t.Parallel()

	Convey("Proto reflection magic", t, func() {
		Convey("v1 protos", func() {
			e := Entry{
				Path: "unused",
				Type: protov1.MessageV2((*configpb.Project)(nil)),
			}
			msg, err := e.validate(&validation.Context{}, `id: "zzz"`)
			So(err, ShouldBeNil)
			So(protov1.MessageV1(msg).(*configpb.Project).Id, ShouldEqual, "zzz")

			// Make sure proto.Merge() would do the correct thing too.
			msgV1 := &configpb.Project{}
			msgV1asV2 := protov1.MessageV2(msgV1)
			proto.Reset(msgV1asV2)
			proto.Merge(msgV1asV2, msg)
			So(msgV1.Id, ShouldEqual, "zzz")
		})

		Convey("v2 protos", func() {
			e := Entry{
				Path: "unused",
				Type: (*durationpb.Duration)(nil),
			}
			msg, err := e.validate(&validation.Context{}, "seconds: 123")
			So(err, ShouldBeNil)
			So(msg.(*durationpb.Duration).Seconds, ShouldEqual, 123)
		})
	})

	Convey("With mocks", t, func() {
		const (
			rev1 = "1704e5202d83e699573530524b078a03a160979f"
			rev2 = "407a70a0258bccc412a570f069c14caa64fab3bb"
		)

		configs := map[config.Set]cfgmem.Files{
			serviceConfigSet: {testEntry.Path: `seconds: 1`},
		}

		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		Convey("Eager update success", func() {
			meta := config.Meta{}

			pb, err := testEntry.Get(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 1)
			So(meta.Revision, ShouldEqual, rev1)
		})

		Convey("Eager update fail", func() {
			configs[serviceConfigSet][testEntry.Path] = `broken`
			_, err := testEntry.Get(ctx, nil)
			So(err, ShouldErrLike, "no such entity")
		})

		Convey("Update works", func() {
			meta := config.Meta{}

			// Initial update.
			pb, err := testEntry.Update(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 1)
			So(meta.Revision, ShouldEqual, rev1)

			// Fetch works now.
			pb, err = testEntry.Fetch(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 1)
			So(meta.Revision, ShouldEqual, rev1)

			// Get works as well.
			pb, err = testEntry.Get(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 1)
			So(meta.Revision, ShouldEqual, rev1)

			// Noop update.
			pb, err = testEntry.Update(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 1)
			So(meta.Revision, ShouldEqual, rev1)

			// Real update.
			configs[serviceConfigSet][testEntry.Path] = `seconds: 2`
			pb, err = testEntry.Update(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 2)
			So(meta.Revision, ShouldEqual, rev2)

			// Fetch returns the new value right away.
			pb, err = testEntry.Fetch(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 2)
			So(meta.Revision, ShouldEqual, rev2)

			// Get still uses in-memory cached copy.
			pb, err = testEntry.Get(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 1)
			So(meta.Revision, ShouldEqual, rev1)

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new value now too.
			pb, err = testEntry.Get(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 2)
			So(meta.Revision, ShouldEqual, rev2)
		})

		Convey("Failing validation", func() {
			configs[serviceConfigSet][testEntry.Path] = `wat?`
			_, err := testEntry.Update(ctx, nil)
			So(err, ShouldErrLike, "validation errors")
		})

		Convey("Set works", func() {
			err := testEntry.Set(ctx, &durationpb.Duration{Seconds: 666}, &config.Meta{
				Revision: "123",
			})
			So(err, ShouldBeNil)

			var meta config.Meta

			pb, err := testEntry.Get(ctx, &meta)
			So(err, ShouldBeNil)
			So(pb.(*durationpb.Duration).Seconds, ShouldEqual, 666)
			So(meta.Revision, ShouldEqual, "123")
		})
	})
}
