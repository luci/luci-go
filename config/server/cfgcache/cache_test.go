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

	"go.chromium.org/luci/common/clock/testclock"
	configpb "go.chromium.org/luci/common/proto/config" // some "random" v1 proto
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/validation"
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

var testEntryCustomConfigSet = Register(&Entry{
	Path:      "path.cfg",
	ConfigSet: "services/another-service",
	Type:      (*durationpb.Duration)(nil),
})

func TestProtoReflection(t *testing.T) {
	t.Parallel()

	ftt.Run("Proto reflection magic", t, func(t *ftt.Test) {
		t.Run("v1 protos", func(t *ftt.Test) {
			e := Entry{
				Path: "unused",
				Type: protov1.MessageV2((*configpb.Project)(nil)),
			}
			msg, err := e.validate(&validation.Context{}, `id: "zzz"`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, protov1.MessageV1(msg).(*configpb.Project).Id, should.Equal("zzz"))

			// Make sure proto.Merge() would do the correct thing too.
			msgV1 := &configpb.Project{}
			msgV1asV2 := protov1.MessageV2(msgV1)
			proto.Reset(msgV1asV2)
			proto.Merge(msgV1asV2, msg)
			assert.Loosely(t, msgV1.Id, should.Equal("zzz"))
		})

		t.Run("v2 protos", func(t *ftt.Test) {
			e := Entry{
				Path: "unused",
				Type: (*durationpb.Duration)(nil),
			}
			msg, err := e.validate(&validation.Context{}, "seconds: 123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, msg.(*durationpb.Duration).Seconds, should.Equal(123))
		})
	})

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		const (
			rev1 = "1704e5202d83e699573530524b078a03a160979f"
			rev2 = "407a70a0258bccc412a570f069c14caa64fab3bb"
		)

		configs := map[config.Set]cfgmem.Files{
			defaultServiceConfigSet:    {testEntry.Path: `seconds: 1`},
			"services/another-service": {testEntryCustomConfigSet.Path: `nanos: 5`},
		}

		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = cfgclient.Use(ctx, cfgmem.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		t.Run("Eager update success", func(t *ftt.Test) {
			meta := config.Meta{}

			pb, err := testEntry.Get(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(1))
			assert.Loosely(t, meta.Revision, should.Equal(rev1))

			meta = config.Meta{}

			pb, err = testEntryCustomConfigSet.Get(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Nanos, should.Equal(5))
		})

		t.Run("Eager update fail", func(t *ftt.Test) {
			configs[defaultServiceConfigSet][testEntry.Path] = `broken`
			_, err := testEntry.Get(ctx, nil)
			assert.Loosely(t, err, should.ErrLike("no such entity"))
		})

		t.Run("Update works", func(t *ftt.Test) {
			meta := config.Meta{}

			// Initial update.
			pb, err := testEntry.Update(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(1))
			assert.Loosely(t, meta.Revision, should.Equal(rev1))

			// Fetch works now.
			pb, err = testEntry.Fetch(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(1))
			assert.Loosely(t, meta.Revision, should.Equal(rev1))

			// Get works as well.
			pb, err = testEntry.Get(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(1))
			assert.Loosely(t, meta.Revision, should.Equal(rev1))

			// Noop update.
			pb, err = testEntry.Update(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(1))
			assert.Loosely(t, meta.Revision, should.Equal(rev1))

			// Real update.
			configs[defaultServiceConfigSet][testEntry.Path] = `seconds: 2`
			pb, err = testEntry.Update(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(2))
			assert.Loosely(t, meta.Revision, should.Equal(rev2))

			// Fetch returns the new value right away.
			pb, err = testEntry.Fetch(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(2))
			assert.Loosely(t, meta.Revision, should.Equal(rev2))

			// Get still uses in-memory cached copy.
			pb, err = testEntry.Get(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(1))
			assert.Loosely(t, meta.Revision, should.Equal(rev1))

			// Time passes, in-memory cached copy expires.
			tc.Add(2 * time.Minute)

			// Get returns the new value now too.
			pb, err = testEntry.Get(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(2))
			assert.Loosely(t, meta.Revision, should.Equal(rev2))
		})

		t.Run("Failing validation", func(t *ftt.Test) {
			configs[defaultServiceConfigSet][testEntry.Path] = `wat?`
			_, err := testEntry.Update(ctx, nil)
			assert.Loosely(t, err, should.ErrLike("validation errors"))
		})

		t.Run("Set works", func(t *ftt.Test) {
			err := testEntry.Set(ctx, &durationpb.Duration{Seconds: 666}, &config.Meta{
				Revision: "123",
			})
			assert.Loosely(t, err, should.BeNil)

			var meta config.Meta

			pb, err := testEntry.Get(ctx, &meta)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pb.(*durationpb.Duration).Seconds, should.Equal(666))
			assert.Loosely(t, meta.Revision, should.Equal("123"))
		})
	})
}
