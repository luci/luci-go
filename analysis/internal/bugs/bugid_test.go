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

package bugs

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidate(t *testing.T) {
	ftt.Run(`Validate`, t, func(t *ftt.Test) {
		id := BugID{
			System: "monorail",
			ID:     "chromium/123",
		}
		t.Run(`System missing`, func(t *ftt.Test) {
			id.System = ""
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`invalid bug tracking system`))
		})
		t.Run("ID invalid", func(t *ftt.Test) {
			id.ID = "!!!"
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`invalid monorail bug ID`))
		})
	})
}
