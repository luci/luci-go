-- Copyright 2022 The LUCI Authors
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--      http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

-- Updates multiple quota accounts in one go.

-- cmsgpack will either be global (production) or will be passed via
-- quotatestmonkeypatch.
local PB = loadfile('../../quotapb/quotapb.lua')(cmsgpack)
PB.setInternTable(KEYS)

local Utils = loadfile('utils.lua')(PB)

local req = PB.unmarshal("go.chromium.org.luci.server.quota.quotapb.UpdateAccountsInput", ARGV[1])

return Utils.WithRequestID(req, function()
  -- We load these 'lazily' because WithRequestID may return early without ever
  -- calling this function.
  local Policy = loadfile('policy.lua')(PB, Utils)
  local Account = loadfile('account.lua')(PB, Utils, Policy)
  return Account.ApplyOps(req.ops)
end)
