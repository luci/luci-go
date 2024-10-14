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
	"io"
	"math/rand"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFloats(t *testing.T) {
	t.Parallel()

	ftt.Run("floats", t, func(t *ftt.Test) {
		b := &bytes.Buffer{}

		t.Run("good", func(t *ftt.Test) {
			f1 := float64(1.234)
			n, err := WriteFloat64(b, f1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(8))
			f, n, err := ReadFloat64(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(8))
			assert.Loosely(t, f, should.Equal(f1))
		})

		t.Run("bad", func(t *ftt.Test) {
			_, n, err := ReadFloat64(b)
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.BeZero)
		})
	})
}

func TestFloatSortability(t *testing.T) {
	t.Parallel()

	ftt.Run("floats maintain sort order", t, func(t *ftt.Test) {
		vec := make(sort.Float64Slice, randomTestSize)
		r := rand.New(rand.NewSource(*seed))
		for i := range vec {
			vec[i] = r.Float64()
		}

		bin := make(sort.StringSlice, len(vec))
		b := &bytes.Buffer{}
		for i := range bin {
			b.Reset()
			n, err := WriteFloat64(b, vec[i])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(8))
			bin[i] = b.String()
		}

		vec.Sort()
		bin.Sort()

		for i := range vec {
			r, _, err := ReadFloat64(bytes.NewBufferString(bin[i]))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, vec[i], should.Equal(r))
		}
	})
}
