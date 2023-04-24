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

// Package quotatestmonkeypatch should be imported for its side-effects in
// tests.
//
// This will monkey-patch the production lua script used by the
// go.chromium.org/luci/server/quota library to include a compatible (but slow)
// version of msgpack, which is missing from the current implementation of
// miniredis.
//
// Do not use this in your production binaries.
//
// Usage
//
//	import _ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"
package quotatestmonkeypatch

import (
	"strings"
	"text/template"

	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/internal/lua"
	"go.chromium.org/luci/server/quota/internal/luatest/testdata/luamsgpack"
)

func init() {
	ctx := struct {
		LuaMsgpack   string
		UpdateScript string
	}{
		luamsgpack.GetAssetString("MessagePack.lua"),
		lua.GetAssetString("update-accounts.gen.lua"),
	}
	buf := &strings.Builder{}
	// we put UpdateScript lexically earlier in the file to make stack traces
	// easier to figure out.
	err := template.Must(template.New("test-script.lua").Parse(`
	return (function(cmsgpack, DUMP); {{.UpdateScript}}
	end)((function()
	{{.LuaMsgpack}}
	end)(), function(...)
		local toprint = {}
	  for i, v in next, arg do
			if type(v) == "table" then
			  v = cjson.encode(v)
			end
			toprint[i] = v
		end
		print(unpack(toprint))
  end)
	`)).Execute(buf, ctx)
	if err != nil {
		panic(err)
	}
	quota.UpdateAccountsScript = redis.NewScript(-1, buf.String())
}
