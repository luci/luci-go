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

-- expect to have PB and Utils passed in
local PB, Utils = ...
assert(PB)
assert(Utils)

local Policy = {
  -- config => key => {
  --   policy_ref: PB go.chromium.org.luci.server.quota.PolicyRef,
  --   pb: PB go.chromium.org.luci.server.quota.Policy,
  -- }
  CACHE = {},
}

local PolicyPB = "go.chromium.org.luci.server.quota.quotapb.Policy"
local PolicyRefPB = "go.chromium.org.luci.server.quota.quotapb.PolicyRef"

function Policy.get(policy_ref)
  assert(policy_ref ~= nil, "Policy:get called with <nil>")

  local configkey = policy_ref.config
  local policykey = policy_ref.key

  if configkey == "" and policykey == "" then
    return nil
  end

  local configTable = Policy.CACHE[configkey]
  if not configTable then
    configTable = {}
    Policy.CACHE[configkey] = configTable
  end

  local entry = configTable[policykey]
  if not entry then
    local raw = redis.call("HGET", configkey, policykey)
    if not raw then
      return nil -- will result in "ERR_UNKNOWN_POLICY"
    end
    entry = {
      policy_ref = policy_ref,
      pb = PB.unmarshal(PolicyPB, raw),
    }
    configTable[policykey] = entry
  end
  return entry
end

return Policy
