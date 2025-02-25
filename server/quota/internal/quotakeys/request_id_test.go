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

package quotakeys

import (
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/quota/quotapb"
)

func TestRequestDedupKey(t *testing.T) {
	t.Parallel()

	ftt.Run(`RequestDedupKey`, t, func(t *ftt.Test) {
		user, err := identity.MakeIdentity("user:charlie@example.com")
		assert.Loosely(t, err, should.BeNil)
		requestID := "spam-in-a-can"

		key := RequestDedupKey(&quotapb.RequestDedupKey{
			Ident:     string(user),
			RequestId: requestID,
		})
		assert.Loosely(t, key, should.Match(`"a~r~user:charlie@example.com~spam-in-a-can`))

		entry, err := ParseRequestDedupKey(key)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, entry.Ident, should.Match(string(user)))
		assert.Loosely(t, entry.RequestId, should.Match(requestID))
	})
}
