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
	"context"
	"encoding/json"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPredefinedTypes(t *testing.T) {
	t.Parallel()

	ftt.Run("Test predefined types", t, func(t *ftt.Test) {
		c := context.Background()
		t.Run("local_auth", func(t *ftt.Test) {
			assert.Loosely(t, GetLocalAuth(c), should.BeNil)

			localAuth := &LocalAuth{
				RpcPort: 100,
				Secret:  []byte("foo"),
				Accounts: []*LocalAuthAccount{
					{Id: "test", Email: "some@example.com"},
				},
				DefaultAccountId: "test",
			}

			c = SetLocalAuth(c, localAuth)
			data := getCurrent(c).sections["local_auth"]
			var v any
			assert.Loosely(t, json.Unmarshal(*data, &v), should.BeNil)
			assert.Loosely(t, v, should.Resemble(map[string]any{
				"accounts": []any{map[string]any{
					"email": "some@example.com",
					"id":    "test",
				}},
				"default_account_id": "test",
				"secret":             "Zm9v",
				"rpc_port":           100.0,
			}))

			assert.Loosely(t, GetLocalAuth(c), should.Resemble(localAuth))
		})

		t.Run("swarming", func(t *ftt.Test) {
			assert.Loosely(t, GetSwarming(c), should.BeNil)

			s := &Swarming{
				SecretBytes: []byte("foo"),
				Task: &Task{
					Server: "https://backend.appspot.com",
					TaskId: "task_id",
					BotDimensions: []string{
						"k1:v1",
					},
				},
			}
			c = SetSwarming(c, s)
			data := getCurrent(c).sections["swarming"]
			var v any
			assert.Loosely(t, json.Unmarshal(*data, &v), should.BeNil)
			assert.Loosely(t, v, should.Resemble(map[string]any{
				"secret_bytes": "Zm9v",
				"task": map[string]any{
					"server":         "https://backend.appspot.com",
					"task_id":        "task_id",
					"bot_dimensions": []any{"k1:v1"},
				},
			}))
			assert.Loosely(t, GetSwarming(c), should.Resemble(s))
		})

		t.Run("resultdb", func(t *ftt.Test) {
			assert.Loosely(t, GetResultDB(c), should.BeNil)

			resultdb := &ResultDB{
				Hostname: "test.results.cr.dev",
				CurrentInvocation: &ResultDBInvocation{
					Name:        "invocations/build:1",
					UpdateToken: "foobarbazsecretoken",
				}}
			c = SetResultDB(c, resultdb)
			data := getCurrent(c).sections["resultdb"]
			var v any
			assert.Loosely(t, json.Unmarshal(*data, &v), should.BeNil)
			assert.Loosely(t, v, should.Resemble(map[string]any{
				"current_invocation": map[string]any{
					"name":         "invocations/build:1",
					"update_token": "foobarbazsecretoken",
				},
				"hostname": "test.results.cr.dev",
			}))

			assert.Loosely(t, GetResultDB(c), should.Resemble(resultdb))
		})

		t.Run("realm", func(t *ftt.Test) {
			assert.Loosely(t, GetRealm(c), should.BeNil)

			r := &Realm{
				Name: "test:realm",
			}
			c = SetRealm(c, r)
			data := getCurrent(c).sections["realm"]
			assert.Loosely(t, string(*data), should.Equal(`{"name":"test:realm"}`))
			assert.Loosely(t, GetRealm(c), should.Resemble(r))
			proj, realm := CurrentRealm(c)
			assert.Loosely(t, proj, should.Equal("test"))
			assert.Loosely(t, realm, should.Equal("realm"))
		})

		t.Run("buildbucket", func(t *ftt.Test) {
			assert.Loosely(t, GetBuildbucket(c), should.BeNil)

			b := &Buildbucket{
				Hostname:           "hostname",
				ScheduleBuildToken: "a token",
			}
			c = SetBuildbucket(c, b)
			data := getCurrent(c).sections["buildbucket"]
			var v any
			assert.Loosely(t, json.Unmarshal(*data, &v), should.BeNil)
			assert.Loosely(t, v, should.Resemble(map[string]any{
				"hostname":             "hostname",
				"schedule_build_token": "a token",
			}))
			assert.Loosely(t, GetBuildbucket(c), should.Resemble(b))
		})
	})
}
