// Copyright 2021 The LUCI Authors.
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
)

func TestTryjobIDs(t *testing.T) {
	t.Parallel()

	ftt.Run("TryjobIDs", t, func(t *ftt.Test) {
		t.Run("Dedupe", func(t *ftt.Test) {
			ids := MakeTryjobIDs(7, 6, 3, 1, 3, 4, 9, 2, 1, 5, 8, 8, 8, 4, 9)
			ids.Dedupe()
			assert.Loosely(t, ids, should.Resemble(TryjobIDs{1, 2, 3, 4, 5, 6, 7, 8, 9}))

			ids = TryjobIDs{6, 1, 2, 2, 3, 4}
			ids.Dedupe()
			assert.Loosely(t, ids, should.Resemble(TryjobIDs{1, 2, 3, 4, 6}))
		})
	})
}
