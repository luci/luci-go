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

		got, err := fn([]byte("seconds: 42"))
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match(&durationpb.Duration{Seconds: 42}))

		_, err = fn([]byte("not_a_field: 42"))
		assert.ErrIsLike(t, err, "as text proto")
	})

	t.Run("string default", func(t *testing.T) {
		t.Parallel()
		fn, err := (UnmarshalFunc[string](nil)).asGeneric()
		assert.NoErr(t, err)

		got, err := fn([]byte("hello"))
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Equal("hello"))
	})

	t.Run("[]byte default", func(t *testing.T) {
		t.Parallel()
		fn, err := (UnmarshalFunc[[]byte](nil)).asGeneric()
		assert.NoErr(t, err)

		in := []byte("world")
		got, err := fn(in)
		assert.NoErr(t, err)
		gotBytes := got.([]byte)
		assert.That(t, gotBytes, should.Match([]byte("world")))
		assert.That(t, &gotBytes[0], should.NotEqual(&in[0]))
	})

	t.Run("custom", func(t *testing.T) {
		t.Parallel()
		fn, err := UnmarshalFunc[int](func(raw []byte) (int, error) {
			return strconv.Atoi(string(raw))
		}).asGeneric()
		assert.NoErr(t, err)

		got, err := fn([]byte("123"))
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Equal(123))

		_, err = fn([]byte("bad"))
		assert.ErrIsLike(t, err, "invalid syntax")
	})

	t.Run("nil UnmarshalFunc for non-proto type returns error", func(t *testing.T) {
		t.Parallel()
		_, err := (UnmarshalFunc[int](nil)).asGeneric()
		assert.ErrIsLike(t, err, "Unmarshal must be set for non-proto type int")
	})
}
