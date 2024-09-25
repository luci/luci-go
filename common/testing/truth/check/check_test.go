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

package check_test

import (
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/internal/test_helper"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	t.Run(`success`, func(t *testing.T) {
		t.Parallel()

		assert.That(t, check.That(t, 100, should.Equal(100)), should.BeTrue)
		assert.That(t, check.Loosely(t, 100, should.Equal(int64(100))), should.BeTrue)

		var err error
		assert.That(t, check.NoErr(t, err), should.BeTrue)

		err = errors.New("something")
		assert.That(t, check.ErrIsLike(t, err, "something"), should.BeTrue)

		assert.That(t, check.ErrIsLike(t, fmt.Errorf("extra: %w", err), err), should.BeTrue)
	})

	t.Run(`failure`, func(t *testing.T) {
		t.Parallel()

		t.Run(`That`, func(t *testing.T) {
			et := test_helper.NewExpectFailure(t)

			assert.That(t, check.That(et, 100, should.Equal(200)), should.BeFalse)

			et.Check("check.That should.Equal[int] FAILED")
		})

		t.Run(`Loosely`, func(t *testing.T) {
			et := test_helper.NewExpectFailure(t)

			assert.That(t, check.Loosely(et, 100, should.Equal(200)), should.BeFalse)

			et.Check("check.Loosely should.Equal[int] FAILED")
		})

		t.Run(`Loosely (bad type)`, func(t *testing.T) {
			et := test_helper.NewExpectFailure(t)

			assert.That(t, check.Loosely(et, 100, should.Equal("100")), should.BeFalse)

			et.Check(
				"check.Loosely comparison.Func[string].CastCompare FAILED",
				"ActualType: int",
			)
		})

		t.Run(`NoErr`, func(t *testing.T) {
			et := test_helper.NewExpectFailure(t)

			assert.That(t, check.NoErr(et, errors.New("morp")), should.BeFalse)

			et.Check("check.That should.ErrLikeError(nil) FAILED")
		})

		t.Run(`ErrIsLike string`, func(t *testing.T) {
			et := test_helper.NewExpectFailure(t)

			assert.That(t, check.ErrIsLike(et, errors.New("morp"), "dorp"), should.BeFalse)

			et.Check("check.That should.ErrLikeString FAILED")
		})

		t.Run(`ErrIsLike error`, func(t *testing.T) {
			et := test_helper.NewExpectFailure(t)

			assert.That(t, check.ErrIsLike(et, errors.New("morp"), errors.New("dorp")), should.BeFalse)

			et.Check("check.That should.ErrLikeError[*errors.errorString] FAILED")
		})
	})
}
