// Copyright 2023 The LUCI Authors.
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

package auth

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestIsValidGroupName(t *testing.T) {
	t.Parallel()

	ftt.Run("IsValidGroupName", t, func(t *ftt.Test) {
		assert.Loosely(t, IsValidGroupName("foo^"), should.BeFalse)
		assert.Loosely(t, IsValidGroupName("mdb/foo"), should.BeTrue)
		assert.Loosely(t, IsValidGroupName("foo-bar"), should.BeTrue)
	})
}
