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

package distribution

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFixedWidthBucketer(t *testing.T) {
	ftt.Run("Invalid values panic", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { FixedWidthBucketer(10, -1) }, should.Panic)
		assert.Loosely(t, func() { FixedWidthBucketer(-1, 1) }, should.Panic)
	})

	ftt.Run("One size", t, func(t *ftt.Test) {
		b := FixedWidthBucketer(10, 1)
		assert.Loosely(t, b.NumBuckets(), should.Equal(3))
		assert.Loosely(t, b.Bucket(-100), should.BeZero)
		assert.Loosely(t, b.Bucket(-1), should.BeZero)
		assert.Loosely(t, b.Bucket(0), should.Equal(1))
		assert.Loosely(t, b.Bucket(5), should.Equal(1))
		assert.Loosely(t, b.Bucket(10), should.Equal(2))
		assert.Loosely(t, b.Bucket(100), should.Equal(2))
	})
}

func TestGeometricBucketer(t *testing.T) {
	ftt.Run("Invalid values panic", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { GeometricBucketer(10, -1) }, should.Panic)
		assert.Loosely(t, func() { GeometricBucketer(-1, 10) }, should.Panic)
		assert.Loosely(t, func() { GeometricBucketer(0, 10) }, should.Panic)
		assert.Loosely(t, func() { GeometricBucketer(1, 10) }, should.Panic)
		assert.Loosely(t, func() { GeometricBucketer(0.5, 10) }, should.Panic)
	})

	ftt.Run("One size", t, func(t *ftt.Test) {
		b := GeometricBucketer(4, 4)
		assert.Loosely(t, b.NumBuckets(), should.Equal(6))
		assert.Loosely(t, b.Bucket(-100), should.BeZero)
		assert.Loosely(t, b.Bucket(-1), should.BeZero)
		assert.Loosely(t, b.Bucket(0), should.BeZero)
		assert.Loosely(t, b.Bucket(1), should.Equal(1))
		assert.Loosely(t, b.Bucket(3), should.Equal(1))
		assert.Loosely(t, b.Bucket(4), should.Equal(2))
		assert.Loosely(t, b.Bucket(15), should.Equal(2))
		assert.Loosely(t, b.Bucket(16), should.Equal(3))
		assert.Loosely(t, b.Bucket(63), should.Equal(3))
		assert.Loosely(t, b.Bucket(64), should.Equal(4))
	})
}

func TestGeometricBucketerWithScale(t *testing.T) {
	ftt.Run("Invalid values panic", t, func(t *ftt.Test) {
		assert.Loosely(t, func() { GeometricBucketerWithScale(10, 10, -1.0) }, should.Panic)
		assert.Loosely(t, func() { GeometricBucketerWithScale(10, 10, 0) }, should.Panic)
	})

	ftt.Run("pass", t, func(t *ftt.Test) {
		b := GeometricBucketerWithScale(4, 4, 100)
		assert.Loosely(t, b.NumBuckets(), should.Equal(6))
		assert.Loosely(t, b.Bucket(-100), should.BeZero)
		assert.Loosely(t, b.Bucket(-1), should.BeZero)
		assert.Loosely(t, b.Bucket(0), should.BeZero)
		assert.Loosely(t, b.Bucket(100), should.Equal(1))
		assert.Loosely(t, b.Bucket(300), should.Equal(1))
		assert.Loosely(t, b.Bucket(400), should.Equal(2))
		assert.Loosely(t, b.Bucket(1500), should.Equal(2))
		assert.Loosely(t, b.Bucket(1600), should.Equal(3))
		assert.Loosely(t, b.Bucket(6300), should.Equal(3))
		assert.Loosely(t, b.Bucket(6400), should.Equal(4))
	})
}
