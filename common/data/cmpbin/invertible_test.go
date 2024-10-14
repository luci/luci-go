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

package cmpbin

import (
	"bytes"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestInvertible(t *testing.T) {
	t.Parallel()

	ftt.Run("Test InvertibleByteBuffer", t, func(t *ftt.Test) {
		inv := Invertible(&bytes.Buffer{})

		t.Run("normal writing", func(t *ftt.Test) {
			t.Run("Write", func(t *ftt.Test) {
				n, err := inv.Write([]byte("hello"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(5))
				assert.Loosely(t, inv.String(), should.Equal("hello"))
			})
			t.Run("WriteString", func(t *ftt.Test) {
				n, err := inv.WriteString("hello")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(5))
				assert.Loosely(t, inv.String(), should.Equal("hello"))
			})
			t.Run("WriteByte", func(t *ftt.Test) {
				for i := byte('a'); i < 'f'; i++ {
					err := inv.WriteByte(i)
					assert.Loosely(t, err, should.BeNil)
				}
				assert.Loosely(t, inv.String(), should.Equal("abcde"))

				t.Run("ReadByte", func(t *ftt.Test) {
					for i := 0; i < 5; i++ {
						b, err := inv.ReadByte()
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, b, should.Equal(byte('a')+byte(i)))
					}
				})
			})
		})
		t.Run("inverted writing", func(t *ftt.Test) {
			inv.SetInvert(true)
			t.Run("Write", func(t *ftt.Test) {
				n, err := inv.Write([]byte("hello"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(5))
				assert.Loosely(t, inv.String(), should.Equal("\x97\x9a\x93\x93\x90"))
			})
			t.Run("WriteString", func(t *ftt.Test) {
				n, err := inv.WriteString("hello")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, n, should.Equal(5))
				assert.Loosely(t, inv.String(), should.Equal("\x97\x9a\x93\x93\x90"))
			})
			t.Run("WriteByte", func(t *ftt.Test) {
				for i := byte('a'); i < 'f'; i++ {
					err := inv.WriteByte(i)
					assert.Loosely(t, err, should.BeNil)
				}
				assert.Loosely(t, inv.String(), should.Equal("\x9e\x9d\x9c\x9b\x9a"))

				t.Run("ReadByte", func(t *ftt.Test) {
					for i := 0; i < 5; i++ {
						b, err := inv.ReadByte()
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, b, should.Equal(byte('a')+byte(i))) // inverted back to normal
					}
				})
			})
		})
		t.Run("Toggleable", func(t *ftt.Test) {
			inv.SetInvert(true)
			n, err := inv.Write([]byte("hello"))
			assert.Loosely(t, err, should.BeNil)
			inv.SetInvert(false)
			n, err = inv.Write([]byte("hello"))
			assert.Loosely(t, n, should.Equal(5))
			assert.Loosely(t, inv.String(), should.Equal("\x97\x9a\x93\x93\x90hello"))
		})
	})
}
