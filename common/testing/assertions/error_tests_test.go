// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package assertions

import (
	"errors"
	"testing"

	multierror "go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type customError struct{}

func (customError) Error() string { return "customError noob" }
func TestShouldErrLike(t *testing.T) {
	t.Parallel()
	ce := customError{}
	e := errors.New("e is for error")
	f := errors.New("f is not for error")
	me := multierror.MultiError{
		e,
		nil,
		ce,
	}
	ftt.Run("Test ShouldContainErr", t, func(t *ftt.Test) {
		t.Run("too many params", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(nil, nil, nil), should.ContainSubstring("requires 0 or 1"))
		})
		t.Run("no expectation", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(multierror.MultiError(nil)), should.ContainSubstring("Expected '<nil>' to NOT be nil"))
			assert.Loosely(t, ShouldContainErr(me), should.BeEmpty)
		})
		t.Run("nil expectation", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(multierror.MultiError(nil), nil), should.ContainSubstring("expected MultiError to contain"))
			assert.Loosely(t, ShouldContainErr(me, nil), should.BeEmpty)
		})
		t.Run("nil actual", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(nil, nil), should.ContainSubstring("Expected '<nil>' to NOT be nil"))
			assert.Loosely(t, ShouldContainErr(nil, "wut"), should.ContainSubstring("Expected '<nil>' to NOT be nil"))
		})
		t.Run("not a multierror", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(100, "wut"), should.ContainSubstring("Expected '100' to be: 'errors.MultiError'"))
		})
		t.Run("string actual", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(me, "is for error"), should.BeEmpty)
			assert.Loosely(t, ShouldContainErr(me, "customError"), should.BeEmpty)
			assert.Loosely(t, ShouldContainErr(me, "is not for error"), should.ContainSubstring("expected MultiError to contain"))
		})
		t.Run("error actual", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(me, e), should.BeEmpty)
			assert.Loosely(t, ShouldContainErr(me, ce), should.BeEmpty)
			assert.Loosely(t, ShouldContainErr(me, f), should.ContainSubstring("expected MultiError to contain"))
		})
		t.Run("bad expected type", func(t *ftt.Test) {
			assert.Loosely(t, ShouldContainErr(me, 20), should.ContainSubstring("unexpected argument type int"))
		})
	})
	ftt.Run("Test ShouldErrLike", t, func(t *ftt.Test) {
		t.Run("too many params", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(nil, nil, nil), should.ContainSubstring("only accepts `nil` on the right hand side"))
		})
		t.Run("no expectation", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(nil), should.Equal("ShouldErrLike requires 1 or more expected values, got 0"))
			assert.Loosely(t, ShouldErrLike(e), should.Equal("ShouldErrLike requires 1 or more expected values, got 0"))
			assert.Loosely(t, ShouldErrLike(ce), should.Equal("ShouldErrLike requires 1 or more expected values, got 0"))
		})
		t.Run("nil expectation", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(nil, nil), should.BeEmpty)
			assert.Loosely(t, ShouldErrLike(e, nil), should.ContainSubstring("Expected: nil"))
			assert.Loosely(t, ShouldErrLike(ce, nil), should.ContainSubstring("Expected: nil"))
		})
		t.Run("nil actual", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(nil, "wut"), should.ContainSubstring("Expected '<nil>' to NOT be nil"))
			assert.Loosely(t, ShouldErrLike(nil, ""), should.ContainSubstring("Expected '<nil>' to NOT be nil"))
		})
		t.Run("not an err", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(100, "wut"), should.ContainSubstring("Expected: 'error interface support'"))
		})
		t.Run("string actual", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(e, "is for error"), should.BeEmpty)
			assert.Loosely(t, ShouldErrLike(ce, "customError"), should.BeEmpty)
			assert.Loosely(t, ShouldErrLike(e, ""), should.BeEmpty)
			assert.Loosely(t, ShouldErrLike(ce, ""), should.BeEmpty)
		})
		t.Run("error actual", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(e, e), should.BeEmpty)
			assert.Loosely(t, ShouldErrLike(ce, ce), should.BeEmpty)
		})
		t.Run("bad expected type", func(t *ftt.Test) {
			assert.Loosely(t, ShouldErrLike(e, 20), should.ContainSubstring("unexpected argument type int"))
		})
	})
}
