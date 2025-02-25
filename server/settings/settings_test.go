// Copyright 2015 The LUCI Authors.
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

package settings

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type exampleSettings struct {
	Greetings string `json:"greetings"`
}

type anotherSettings struct{}

func TestSettings(t *testing.T) {
	ftt.Run("with in-memory settings", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), time.Unix(1444945245, 0))
		ctx = memlogger.Use(ctx)
		log := logging.Get(ctx).(*memlogger.MemLogger)

		settings := New(&MemoryStorage{Expiration: time.Second})
		s := exampleSettings{}

		t.Run("settings API works", func(t *ftt.Test) {
			// Nothing is set yet.
			assert.Loosely(t, settings.Get(ctx, "key", &s), should.Equal(ErrNoSettings))

			// Set something.
			assert.Loosely(t, settings.Set(ctx, "key", &exampleSettings{"hi"}), should.BeNil)

			// Old value (the lack of there of) is still cached.
			assert.Loosely(t, settings.Get(ctx, "key", &s), should.Equal(ErrNoSettings))

			// Non-caching version works.
			assert.Loosely(t, settings.GetUncached(ctx, "key", &s), should.BeNil)
			assert.Loosely(t, s, should.Match(exampleSettings{"hi"}))

			// Advance time to make old value expired.
			tc.Add(2 * time.Second)
			assert.Loosely(t, settings.Get(ctx, "key", &s), should.BeNil)
			assert.Loosely(t, s, should.Match(exampleSettings{"hi"}))

			// Not a pointer.
			assert.Loosely(t, settings.Get(ctx, "key", s), should.Equal(ErrBadType))

			// Not *exampleSettings.
			assert.Loosely(t, settings.Get(ctx, "key", &anotherSettings{}), should.Equal(ErrBadType))
		})

		t.Run("SetIfChanged works", func(t *ftt.Test) {
			// Initial value. New change notification.
			assert.Loosely(t, settings.SetIfChanged(ctx, "key", &exampleSettings{"hi"}), should.BeNil)
			assert.Loosely(t, len(log.Messages()), should.Equal(1))
			log.Reset()

			// Noop change. No change notification.
			assert.Loosely(t, settings.SetIfChanged(ctx, "key", &exampleSettings{"hi"}), should.BeNil)
			assert.Loosely(t, len(log.Messages()), should.BeZero)

			// Some real change. New change notification.
			assert.Loosely(t, settings.SetIfChanged(ctx, "key", &exampleSettings{"boo"}), should.BeNil)
			assert.Loosely(t, len(log.Messages()), should.Equal(1))
		})
	})
}

func TestContext(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := context.Background()
		s := exampleSettings{}

		assert.Loosely(t, Get(ctx, "key", &exampleSettings{}), should.Equal(ErrNoSettings))
		assert.Loosely(t, GetUncached(ctx, "key", &exampleSettings{}), should.Equal(ErrNoSettings))
		assert.Loosely(t, Set(ctx, "key", &exampleSettings{}), should.Equal(ErrNoSettings))
		assert.Loosely(t, SetIfChanged(ctx, "key", &exampleSettings{}), should.Equal(ErrNoSettings))

		ctx = Use(ctx, New(&MemoryStorage{}))
		assert.Loosely(t, Set(ctx, "key", &exampleSettings{"hi"}), should.BeNil)
		assert.Loosely(t, SetIfChanged(ctx, "key", &exampleSettings{"hi"}), should.BeNil)

		assert.Loosely(t, Get(ctx, "key", &s), should.BeNil)
		assert.Loosely(t, s, should.Match(exampleSettings{"hi"}))

		assert.Loosely(t, GetUncached(ctx, "key", &s), should.BeNil)
		assert.Loosely(t, s, should.Match(exampleSettings{"hi"}))
	})
}
