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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/quota/quotapb"
)

func TestAccountKey(t *testing.T) {
	t.Parallel()

	ftt.Run(`AccountKey`, t, func(t *ftt.Test) {
		id := &quotapb.AccountID{
			AppId:        "app",
			Realm:        "proj:@project",
			Identity:     "user:joe@example.com",
			Namespace:    "namespace",
			Name:         "name",
			ResourceType: "things",
		}
		key := AccountKey(id)
		assert.Loosely(t, key, should.Match(`"a~a~app~proj:@project~user:joe@example.com~namespace~name~things`))

		newID, err := ParseAccountKey(key)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, newID, should.Match(id))
	})
}
