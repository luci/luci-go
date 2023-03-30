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

//go:generate go run go.chromium.org/luci/tools/cmd/luapp update-accounts.lua
//go:generate go run go.chromium.org/luci/tools/cmd/assets -ext *.gen.lua

package lua

import "github.com/gomodule/redigo/redis"

// UpdateAccountsScript is a redis.Script for 'update-accounts.lua'.
//
// Prefer this to using the raw asset directly, because this has already
// calculated the script SHA1.
var UpdateAccountsScript = redis.NewScript(-1, GetAssetString("update-accounts.gen.lua"))
