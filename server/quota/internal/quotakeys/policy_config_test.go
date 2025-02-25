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

func TestPolicyConfigID(t *testing.T) {
	t.Parallel()

	ftt.Run(`PolicyConfigID`, t, func(t *ftt.Test) {
		t.Run(`content addressed`, func(t *ftt.Test) {
			id := &quotapb.PolicyConfigID{
				AppId:         "app",
				Realm:         "proj:@project",
				VersionScheme: 1,
				Version:       "deadbeef",
			}
			key := PolicyConfigID(id)
			assert.Loosely(t, key, should.Match(`"a~p~app~proj:@project~1~deadbeef`))

			newID, err := ParsePolicyConfigID(key)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newID, should.Match(id))
		})

		t.Run(`manually versioned`, func(t *ftt.Test) {
			id := &quotapb.PolicyConfigID{
				AppId:   "app",
				Realm:   "proj:@project",
				Version: "deadbeef",
			}
			key := PolicyConfigID(id)
			assert.Loosely(t, key, should.Match(`"a~p~app~proj:@project~0~deadbeef`))

			newID, err := ParsePolicyConfigID(key)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newID, should.Match(id))
		})
	})
}

func TestPolicyKey(t *testing.T) {
	t.Parallel()

	ftt.Run(`PolicyKey`, t, func(t *ftt.Test) {
		key := &quotapb.PolicyKey{
			Namespace:    "ns",
			Name:         "name",
			ResourceType: "resource",
		}
		keyStr := PolicyKey(key)
		assert.Loosely(t, keyStr, should.Match(`ns~name~resource`))

		newKey, err := ParsePolicyKey(keyStr)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, newKey, should.Match(key))
	})
}

func TestPolicyID(t *testing.T) {
	t.Parallel()

	ftt.Run(`PolicyID`, t, func(t *ftt.Test) {
		id := &quotapb.PolicyID{
			Config: &quotapb.PolicyConfigID{
				AppId:         "app",
				Realm:         "proj:@project",
				VersionScheme: 1,
				Version:       "deadbeef",
			},
			Key: &quotapb.PolicyKey{
				Namespace:    "ns",
				Name:         "name",
				ResourceType: "resource",
			},
		}
		ref := PolicyRef(id)
		assert.Loosely(t, ref, should.Match(&quotapb.PolicyRef{
			Config: `"a~p~app~proj:@project~1~deadbeef`,
			Key:    `ns~name~resource`,
		}))

		newID, err := ParsePolicyRef(ref)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, newID, should.Match(id))
	})
}
