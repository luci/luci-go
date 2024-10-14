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

package parallel

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMultiError(t *testing.T) {
	t.Parallel()

	ftt.Run("MultiError works", t, func(t *ftt.Test) {
		t.Run("multiErrorFromErrors with errors works", func(t *ftt.Test) {
			mec := make(chan error, 4)
			mec <- nil
			mec <- fmt.Errorf("first error")
			mec <- nil
			mec <- fmt.Errorf("what")
			close(mec)

			err := multiErrorFromErrors(mec)
			assert.Loosely(t, err.Error(), should.Equal(`first error (and 1 other error)`))
		})

		t.Run("multiErrorFromErrors with nil works", func(t *ftt.Test) {
			assert.Loosely(t, multiErrorFromErrors(nil), should.BeNil)

			c := make(chan error)
			close(c)
			assert.Loosely(t, multiErrorFromErrors(c), should.BeNil)

			c = make(chan error, 2)
			c <- nil
			c <- nil
			close(c)
			assert.Loosely(t, multiErrorFromErrors(c), should.BeNil)
		})
	})
}
