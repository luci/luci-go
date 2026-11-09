// Copyright 2026 The LUCI Authors.
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

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
)

func TestToMemoryFuncAsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("NoMemoryRefresh with nil ToMemory returns nil callback", func(t *testing.T) {
		t.Parallel()
		cb, err := (ToMemoryFunc[*durationpb.Duration, NoMemoryRefresh](nil)).asGeneric(nil)
		assert.NoErr(t, err)
		assert.Loosely(t, cb, should.BeNil)
	})

	t.Run("NoMemoryRefresh with non-nil ToMemory returns error", func(t *testing.T) {
		t.Parallel()
		fn := ToMemoryFunc[*durationpb.Duration, NoMemoryRefresh](func(ctx context.Context, cs config.Set, msg *durationpb.Duration) (NoMemoryRefresh, error) {
			return NoMemoryRefresh{}, nil
		})
		_, err := fn.asGeneric(nil)
		assert.ErrIsLike(t, err, "ToMemory must not be set when InMemory is NoMemoryRefresh")
	})

	t.Run("mismatched StoreMsg and InMemory with nil ToMemory returns error", func(t *testing.T) {
		t.Parallel()
		_, err := (ToMemoryFunc[*durationpb.Duration, int64](nil)).asGeneric(nil)
		assert.ErrIsLike(t, err, "ToMemory must be set when StoreMsg and InMemory types differ")
	})

	t.Run("default pass-through stores message and nil on deletion", func(t *testing.T) {
		t.Parallel()
		var gotCS config.Set
		var gotPtr **durationpb.Duration
		cb, err := (ToMemoryFunc[*durationpb.Duration, *durationpb.Duration](nil)).asGeneric(func(cs config.Set, val **durationpb.Duration) {
			gotCS = cs
			gotPtr = val
		})
		assert.NoErr(t, err)

		msg := &durationpb.Duration{Seconds: 42}
		assert.NoErr(t, cb(t.Context(), "services/my-app", msg))
		assert.That(t, gotCS, should.Equal(config.Set("services/my-app")))
		assert.Loosely(t, gotPtr, should.NotBeNil)
		assert.That(t, *gotPtr, should.Match(msg))

		assert.NoErr(t, cb(t.Context(), "services/my-app", nil))
		assert.Loosely(t, gotPtr, should.BeNil)
	})

	t.Run("custom ToMemory converts value and propagates errors", func(t *testing.T) {
		t.Parallel()
		var gotVal *int64
		fn := ToMemoryFunc[*durationpb.Duration, int64](func(ctx context.Context, cs config.Set, msg *durationpb.Duration) (int64, error) {
			if msg.Seconds < 0 {
				return 0, errors.New("negative duration")
			}
			return msg.Seconds * 10, nil
		})
		cb, err := fn.asGeneric(func(cs config.Set, val *int64) {
			gotVal = val
		})
		assert.NoErr(t, err)

		assert.NoErr(t, cb(t.Context(), "projects/p1", &durationpb.Duration{Seconds: 3}))
		assert.Loosely(t, gotVal, should.NotBeNil)
		assert.That(t, *gotVal, should.Equal(int64(30)))

		gotVal = nil
		assert.ErrIsLike(t, cb(t.Context(), "projects/p1", &durationpb.Duration{Seconds: -1}), "negative duration")
		assert.Loosely(t, gotVal, should.BeNil)
	})
}
