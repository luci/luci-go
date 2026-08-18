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

package errors_test

import (
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	aerr := errors.New("test error A")
	berr := errors.New("test error B")

	ftt.Run(`Filter works.`, t, func(t *ftt.Test) {
		assert.Loosely(t, errors.Filter(nil, nil), should.BeNil)
		assert.Loosely(t, errors.Filter(nil, nil, aerr, berr), should.BeNil)
		assert.Loosely(t, errors.Filter(aerr, nil), should.Equal(aerr))
		assert.Loosely(t, errors.Filter(aerr, nil, aerr, berr), should.BeNil)
		assert.Loosely(t, errors.Filter(aerr, berr), should.Equal(aerr))
		assert.Loosely(t, errors.Filter(errors.MultiError{aerr, berr}, berr), should.ErrLike(
			errors.MultiError{aerr, nil}))
		assert.Loosely(t, errors.Filter(errors.MultiError{aerr, aerr}, aerr), should.BeNil)
		assert.Loosely(t, errors.Filter(errors.MultiError{errors.MultiError{aerr, aerr}, aerr}, aerr), should.BeNil)
	})

	ftt.Run(`FilterFunc works.`, t, func(t *ftt.Test) {
		assert.Loosely(t, errors.FilterFunc(nil, func(error) bool { return false }), should.BeNil)
		assert.Loosely(t, errors.FilterFunc(aerr, func(error) bool { return true }), should.BeNil)
		assert.Loosely(t, errors.FilterFunc(aerr, func(error) bool { return false }), should.Equal(aerr))

		// Make sure MultiError gets evaluated before its contents do.
		assert.Loosely(t, errors.FilterFunc(errors.MultiError{aerr, berr}, func(e error) bool {
			if me, ok := e.(errors.MultiError); ok {
				return len(me) == 2
			}
			return false
		}), should.BeNil)
	})
}
