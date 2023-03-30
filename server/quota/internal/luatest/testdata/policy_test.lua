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

require 'test_fixtures'

Account = G.Account
Policy = G.Policy
Utils = G.Utils

function testPolicyGetMissing()
  local ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
    config = "policy_config",
    key = "policy_name",
  })
  lu.assertNil(Policy.get(ref))
end

function testPolicyGet()
  local ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
    config = "policy_config",
    key = "policy_name",
  })
  local policy = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
    default = 20,
    limit = 100,
    refill = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy.Refill",  {
      units = 1,
      interval = 100,
    }),
    lifetime = PB.new("google.protobuf.Duration", {
      seconds = 3600,
    }),
  })
  redis.call("HSET", ref.config, ref.key, PB.marshal(policy))

  local expectedPolicy = "\132\001\020\002d\003\146\001d\005\145\205\014\016"
  lu.assertEquals(redis.DATA[ref.config][ref.key], expectedPolicy)

  local policy = Policy.get(ref)
  lu.assertEquals(policy.pb.default, 20)
  lu.assertEquals(policy.pb.limit, 100)
  lu.assertEquals(policy.pb.refill.units, 1)
  lu.assertEquals(policy.pb.refill.interval, 100)
  lu.assertEquals(policy.pb.lifetime.seconds, 3600)
end

return lu.LuaUnit.run("-v")
