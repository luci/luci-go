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

	"go.chromium.org/luci/server/quota/quotapb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPolicyConfigID(t *testing.T) {
	t.Parallel()

	Convey(`PolicyConfigID`, t, func() {
		Convey(`content addressed`, func() {
			id := &quotapb.PolicyConfigID{
				AppId:         "app",
				Realm:         "proj:@project",
				VersionScheme: 1,
				Version:       "deadbeef",
			}
			key := PolicyConfigID(id)
			So(key, ShouldResemble, `"a~p~app~proj:@project~1~deadbeef`)

			newID, err := ParsePolicyConfigID(key)
			So(err, ShouldBeNil)
			So(newID, ShouldResembleProto, id)
		})

		Convey(`manually versioned`, func() {
			id := &quotapb.PolicyConfigID{
				AppId:   "app",
				Realm:   "proj:@project",
				Version: "deadbeef",
			}
			key := PolicyConfigID(id)
			So(key, ShouldResemble, `"a~p~app~proj:@project~0~deadbeef`)

			newID, err := ParsePolicyConfigID(key)
			So(err, ShouldBeNil)
			So(newID, ShouldResembleProto, id)
		})
	})
}

func TestPolicyKey(t *testing.T) {
	t.Parallel()

	Convey(`PolicyKey`, t, func() {
		key := &quotapb.PolicyKey{
			Namespace:    "ns",
			Name:         "name",
			ResourceType: "resource",
		}
		keyStr := PolicyKey(key)
		So(keyStr, ShouldResemble, `ns~name~resource`)

		newKey, err := ParsePolicyKey(keyStr)
		So(err, ShouldBeNil)
		So(newKey, ShouldResembleProto, key)
	})
}

func TestPolicyID(t *testing.T) {
	t.Parallel()

	Convey(`PolicyID`, t, func() {
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
		So(ref, ShouldResembleProto, &quotapb.PolicyRef{
			Config: `"a~p~app~proj:@project~1~deadbeef`,
			Key:    `ns~name~resource`,
		})

		newID, err := ParsePolicyRef(ref)
		So(err, ShouldBeNil)
		So(newID, ShouldResembleProto, id)
	})
}
