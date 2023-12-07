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

	"go.chromium.org/luci/server/quota/quotapb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRequestDedupKey(t *testing.T) {
	t.Parallel()

	Convey(`RequestDedupKey`, t, func() {
		user, err := identity.MakeIdentity("user:charlie@example.com")
		So(err, ShouldBeNil)
		requestID := "spam-in-a-can"

		key := RequestDedupKey(&quotapb.RequestDedupKey{
			Ident:     string(user),
			RequestId: requestID,
		})
		So(key, ShouldResemble, `"a~r~user:charlie@example.com~spam-in-a-can`)

		entry, err := ParseRequestDedupKey(key)
		So(err, ShouldBeNil)
		So(entry.Ident, ShouldResemble, string(user))
		So(entry.RequestId, ShouldResemble, requestID)
	})
}
