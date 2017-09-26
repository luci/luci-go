// Copyright 2016 The LUCI Authors.
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

package lucictx

import (
	"encoding/json"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPredefinedTypes(t *testing.T) {
	t.Parallel()

	Convey("Test predefined types", t, func() {
		c := context.Background()
		Convey("local_auth", func() {
			So(GetLocalAuth(c), ShouldBeNil)

			localAuth := LocalAuth{
				RPCPort: 100,
				Secret:  []byte("foo"),
				Accounts: []LocalAuthAccount{
					{ID: "test", Email: "some@example.com"},
				},
				DefaultAccountID: "test",
			}

			c = SetLocalAuth(c, &localAuth)
			rawJSON := json.RawMessage{}
			Get(c, "local_auth", &rawJSON)
			So(string(rawJSON), ShouldEqual, `{"rpc_port":100,"secret":"Zm9v",`+
				`"accounts":[{"id":"test","email":"some@example.com"}],"default_account_id":"test"}`)

			So(GetLocalAuth(c), ShouldResemble, &localAuth)
		})

		Convey("swarming", func() {
			So(GetSwarming(c), ShouldBeNil)

			c = SetSwarming(c, &Swarming{[]byte("foo")})
			rawJSON := json.RawMessage{}
			Get(c, "swarming", &rawJSON)
			So(string(rawJSON), ShouldEqual, `{"secret_bytes":"Zm9v"}`)

			So(GetSwarming(c), ShouldResemble, &Swarming{[]byte("foo")})
		})
	})
}
