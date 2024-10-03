// Copyright 2022 The LUCI Authors.
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

package resultdb

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestInvocationFromTestResultName(t *testing.T) {
	ftt.Run("Valid input", t, func(t *ftt.Test) {
		result, err := InvocationFromTestResultName("invocations/build-1234/tests/a/results/b")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.Equal("build-1234"))
	})
	ftt.Run("Invalid input", t, func(t *ftt.Test) {
		_, err := InvocationFromTestResultName("")
		assert.Loosely(t, err, should.ErrLike("invalid test result name"))

		_, err = InvocationFromTestResultName("projects/chromium/resource/b")
		assert.Loosely(t, err, should.ErrLike("invalid test result name"))

		_, err = InvocationFromTestResultName("invocations/build-1234")
		assert.Loosely(t, err, should.ErrLike("invalid test result name"))

		_, err = InvocationFromTestResultName("invocations//")
		assert.Loosely(t, err, should.ErrLike("invalid test result name"))

		_, err = InvocationFromTestResultName("invocations/")
		assert.Loosely(t, err, should.ErrLike("invalid test result name"))

		_, err = InvocationFromTestResultName("invocations")
		assert.Loosely(t, err, should.ErrLike("invalid test result name"))
	})
}
