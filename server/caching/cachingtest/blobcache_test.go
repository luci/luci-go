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

package cachingtest

import (
	"context"
	"errors"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/caching"
)

func TestWorks(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		c := context.Background()
		b := NewBlobCache()

		res, err := b.Get(c, "key")
		assert.Loosely(t, res, should.BeNil)
		assert.Loosely(t, err, should.Equal(caching.ErrCacheMiss))

		assert.Loosely(t, b.Set(c, "key", []byte("blah"), 0), should.BeNil)

		res, err = b.Get(c, "key")
		assert.Loosely(t, res, should.Match([]byte("blah")))
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		fail := errors.New("fail")

		c := context.Background()
		b := NewBlobCache()
		b.Err = fail

		_, err := b.Get(c, "key")
		assert.Loosely(t, err, should.Equal(fail))

		assert.Loosely(t, b.Set(c, "key", nil, 0), should.Equal(fail))
	})
}
