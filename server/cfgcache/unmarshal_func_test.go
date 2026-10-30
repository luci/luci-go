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
	"strconv"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestUnmarshalFuncAsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("proto.Message default", func(t *testing.T) {
		t.Parallel()
		fn, err := (UnmarshalFunc[*durationpb.Duration](nil)).asGeneric()
		assert.NoErr(t, err)

		got, err := fn("seconds: 42")
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match(&durationpb.Duration{Seconds: 42}))

		_, err = fn("not_a_field: 42")
		assert.ErrIsLike(t, err, "as text proto")
	})

	t.Run("string default", func(t *testing.T) {
		t.Parallel()
		fn, err := (UnmarshalFunc[string](nil)).asGeneric()
		assert.NoErr(t, err)

		got, err := fn("hello")
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Equal("hello"))
	})

	t.Run("[]byte default", func(t *testing.T) {
		t.Parallel()
		fn, err := (UnmarshalFunc[[]byte](nil)).asGeneric()
		assert.NoErr(t, err)

		got, err := fn("world")
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match([]byte("world")))
	})

	t.Run("custom", func(t *testing.T) {
		t.Parallel()
		fn, err := UnmarshalFunc[int](strconv.Atoi).asGeneric()
		assert.NoErr(t, err)

		got, err := fn("123")
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Equal(123))

		_, err = fn("bad")
		assert.ErrIsLike(t, err, "invalid syntax")
	})

	t.Run("nil UnmarshalFunc for non-proto type returns error", func(t *testing.T) {
		t.Parallel()
		_, err := (UnmarshalFunc[int](nil)).asGeneric()
		assert.ErrIsLike(t, err, "Unmarshal must be set for non-proto type int")
	})
}
