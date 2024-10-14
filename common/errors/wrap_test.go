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

package errors

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testWrapped struct {
	error
}

func (w *testWrapped) Error() string {
	if w.error == nil {
		return "wrapped: nil"
	}
	return fmt.Sprintf("wrapped: %v", w.error.Error())
}

func (w *testWrapped) Unwrap() error {
	return w.error
}

func testWrap(err error) error {
	return &testWrapped{err}
}

func TestWrapped(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test Wrapped`, t, func(t *ftt.Test) {
		t.Run(`A nil error`, func(t *ftt.Test) {
			var err error

			t.Run(`Unwraps to nil.`, func(t *ftt.Test) {
				assert.Loosely(t, Unwrap(err), should.BeNil)
			})

			t.Run(`When wrapped, does not unwrap to nil.`, func(t *ftt.Test) {
				assert.Loosely(t, Unwrap(testWrap(err)), should.NotBeNil)
			})
		})

		t.Run(`A non-wrapped error.`, func(t *ftt.Test) {
			err := New("test error")

			t.Run(`Unwraps to itself.`, func(t *ftt.Test) {
				assert.Loosely(t, Unwrap(err), should.Equal(err))
			})

			t.Run(`When wrapped, unwraps to itself.`, func(t *ftt.Test) {
				assert.Loosely(t, Unwrap(testWrap(err)), should.Equal(err))
			})

			t.Run(`When double-wrapped, unwraps to itself.`, func(t *ftt.Test) {
				assert.Loosely(t, Unwrap(testWrap(testWrap(err))), should.Equal(err))
			})
		})
	})
}
