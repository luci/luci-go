// Copyright 2017 The LUCI Authors.
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

package common

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

func TestNewHash(t *testing.T) {
	t.Parallel()

	ftt.Run("Unspecified", t, func(t *ftt.Test) {
		_, err := NewHash(api.HashAlgo_HASH_ALGO_UNSPECIFIED)
		assert.Loosely(t, err, should.ErrLike("not specified"))
	})

	ftt.Run("Unknown", t, func(t *ftt.Test) {
		_, err := NewHash(12345)
		assert.Loosely(t, err, should.ErrLike("unsupported"))
	})

	ftt.Run("SHA1", t, func(t *ftt.Test) {
		algo, err := NewHash(api.HashAlgo_SHA1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, algo, should.NotBeNil)
		assert.Loosely(t, ObjectRefFromHash(algo), should.Match(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		}))
	})

	ftt.Run("SHA256", t, func(t *ftt.Test) {
		algo, err := NewHash(api.HashAlgo_SHA256)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, algo, should.NotBeNil)
		assert.Loosely(t, ObjectRefFromHash(algo), should.Match(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		}))
	})
}
