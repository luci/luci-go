// Copyright 2024 The LUCI Authors.
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

package assert_test

import (
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/internal/testtools"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAssert(t *testing.T) {
	t.Parallel()

	t.Run(`success`, func(t *testing.T) {
		t.Parallel()

		assert.That(t, 100, should.Equal(100))
		assert.Loosely(t, 100, should.Equal(int64(100)))

		var err error
		assert.NoErr(t, err)

		err = errors.New("something")
		assert.ErrIsLike(t, err, "something")

		assert.ErrIsLike(t, fmt.Errorf("extra: %w", err), err)
	})

	t.Run(`failure`, func(t *testing.T) {
		t.Parallel()

		t.Run(`That`, func(t *testing.T) {
			et := testtools.NewExpectFailure(t)

			assert.That(et, 100, should.Equal(200))

			et.Check("assert.That should.Equal[int] FAILED")
		})

		t.Run(`Loosely`, func(t *testing.T) {
			et := testtools.NewExpectFailure(t)

			assert.Loosely(et, 100, should.Equal(200))

			et.Check("assert.Loosely should.Equal[int] FAILED")
		})

		t.Run(`Loosely (bad type)`, func(t *testing.T) {
			et := testtools.NewExpectFailure(t)

			assert.Loosely(et, 100, should.Equal("100"))

			et.Check(
				"assert.Loosely comparison.Func[string].CastCompare FAILED",
				"ActualType: int",
			)
		})

		t.Run(`NoErr`, func(t *testing.T) {
			et := testtools.NewExpectFailure(t)

			assert.NoErr(et, errors.New("morp"))

			et.Check("assert.That should.ErrLikeError(nil) FAILED")
		})

		t.Run(`ErrIsLike string`, func(t *testing.T) {
			et := testtools.NewExpectFailure(t)

			assert.ErrIsLike(et, errors.New("morp"), "dorp")

			et.Check("assert.That should.ErrLikeString FAILED")
		})

		t.Run(`ErrIsLike error`, func(t *testing.T) {
			et := testtools.NewExpectFailure(t)

			assert.ErrIsLike(et, errors.New("morp"), errors.New("dorp"))

			et.Check("assert.That should.ErrLikeError[*errors.errorString] FAILED")
		})
	})
}
