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

local DO_NOT_CAP_PROPOSED = PB.E["go.chromium.org.luci.server.quota.quotapb.Op.Options"].DO_NOT_CAP_PROPOSED
local IGNORE_POLICY_BOUNDS = PB.E["go.chromium.org.luci.server.quota.quotapb.Op.Options"].IGNORE_POLICY_BOUNDS
local WITH_POLICY_LIMIT_DELTA = PB.E["go.chromium.org.luci.server.quota.quotapb.Op.Options"].WITH_POLICY_LIMIT_DELTA

function enumMap(e)
  local ret = {}
  for k, v in pairs(PB.E[e]) do
    if type(k) == "string" then
      ret[k] = k
    end
  end
  return ret
end

local AccountStatus = enumMap("go.chromium.org.luci.server.quota.quotapb.OpResult.AccountStatus")
local OpStatus = enumMap("go.chromium.org.luci.server.quota.quotapb.OpResult.OpStatus")

function mkOp(fields)
  return PB.new("go.chromium.org.luci.server.quota.quotapb.RawOp", fields)
end

function setPolicy(policy_config, policy_key, policy_params)
  local ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
    config = policy_config,
    key = policy_key,
  })
  local policy = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", policy_params)

  redis.call("HSET", ref.config, ref.key, PB.marshal(policy))

  return ref
end

function testAccountGetEmpty()
  local account = Account:get(KEYS[1])
  lu.assertEquals(account.account_status, AccountStatus.CREATED)
  lu.assertEquals(account.key, KEYS[1], "account key is saved")
end

function testSaveEmptyAccount()
  local account = Account:get(KEYS[1])
  account:write()
  lu.assertEquals(#redis.DATA, 0, "save is a NOOP until we set a value.")
end

-- Use `./datatool -type Account -out msgpackpb+lua`
--
-- {
--  "balance": 100,
--  "updated_ts": "2022-12-12T08:41:26+00:00"
-- }
--
-- The msgpackpb+pretty of this is:
-- [
--   8i100,
--   [
--     32u1670834486,
--   ],
-- ]
local packedAccount = "\146d\145\206c\150\2336"

function testAccountSetValue()
  local account = Account:get(KEYS[1])
  account.pb.balance = 100

  account:write()
  lu.assertEquals(redis.DATA[KEYS[1]], packedAccount, "data is saved in packed form")
end

function testAccountLoad()
  redis.call("SET", KEYS[1], packedAccount)
  local account = Account:get(KEYS[1])

  lu.assertEquals(account.pb.balance, 100)
  lu.assertEquals(account.pb.updated_ts.seconds, 1670834486)
end

function testNilAccountSetPolicy()
  local account = Account:get(KEYS[1])
  local policy = {
    policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
      config = "policy_key",
      key = "policy_name",
    }),
    pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
      default = 100,
      limit = 1000,
      refill = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy.Refill", {
        units = 1,
        interval = 1,
      }),
      lifetime = PB.new("google.protobuf.Duration", {
        seconds = 123,
      })
    }),
  }

  account:setPolicy(policy)

  lu.assertEquals(account.pb.balance, policy.pb.default)

  account:write()

  lu.assertEquals(account.pb.updated_ts, Utils.NOW)

  lu.assertEquals(redis.DATA[KEYS[1]], "\149d\145\206c\150\2336\145\206c\150\2336\146\170policy_key\171policy_name\132\001d\002\205\003\232\003\146\001\001\005\145{")
  lu.assertEquals(redis.TTL[KEYS[1]], 123000)
end

function testNilAccountSetPolicyNoRefill()
  local account = Account:get(KEYS[1])
  local policy = {
    policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
      config = "policy_key",
      key = "policy_name",
    }),
    pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
      default = 100,
      limit = 1000,
      lifetime = PB.new("google.protobuf.Duration", {
        seconds = 123,
      })
    }),
  }

  account:setPolicy(policy)

  lu.assertEquals(account.pb.balance, policy.pb.default)
  lu.assertEquals(account.pb.updated_ts, Utils.NOW)

  account:write()

  lu.assertEquals(redis.DATA[KEYS[1]], "\149d\145\206c\150\2336\145\206c\150\2336\146\170policy_key\171policy_name\131\001d\002\205\003\232\005\145{")
  lu.assertEquals(redis.TTL[KEYS[1]], 123000)
end

function testNilAccountSetPolicyInfinite()
  local account = Account:get(KEYS[1])
  local policy = {
    policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
      config = "policy_key",
      key = "policy_name",
    }),
    pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
      default = 100,
      limit = 1000,
      refill = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy.Refill", {
        units = 1,
        interval = 0, -- infinity
      }),
      lifetime = PB.new("google.protobuf.Duration", {
        seconds = 123,
      })
    }),
  }

  account:setPolicy(policy)
  lu.assertEquals(account.pb.balance, policy.pb.limit)

  -- note; this is not quite real, ApplyOps would be the only way to do this,
  -- and it will explicitly avoid decreasing the balance. However applyRefill
  -- on get should restore this.
  account.pb.balance = 2
  lu.assertEquals(account.pb.balance, 2)

  account:write()
  Account.CACHE = {}
  account = Account:get(KEYS[1])
  lu.assertEquals(account.pb.balance, policy.pb.limit)

  lu.assertEquals(redis.DATA[KEYS[1]], "\149\002\145\206c\150\2336\145\206c\150\2336\146\170policy_key\171policy_name\132\001d\002\205\003\232\003\145\001\005\145{")
  lu.assertEquals(redis.TTL[KEYS[1]], 123000)
end

function testAccountSetPolicyRefill()
  local account = Account:get(KEYS[1])
  account.pb.balance = 100
  account:write()

  lu.assertEquals(account.pb.balance, 100)
  lu.assertEquals(account.pb.updated_ts, Utils.NOW)

  Utils.NOW.seconds = Utils.NOW.seconds + 3

  local policy = {
    policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
      config = "policy_key",
      key = "policy_name",
    }),
    pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
      default = 100,
      limit = 1000,
      refill = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy.Refill", {
        units = 1,
        interval = 2,
      }),
      lifetime = PB.new("google.protobuf.Duration", {
        seconds = 123,
      })
    }),
  }

  account:setPolicy(policy)

  lu.assertEquals(account.pb.balance, 100)
  lu.assertEquals(account.pb.policy_ref.config, "policy_key")
  lu.assertEquals(account.pb.policy_ref.key, "policy_name")
  lu.assertEquals(account.pb.policy.limit, 1000)
  lu.assertEquals(account.pb.policy.refill.interval, 2)
  lu.assertEquals(account.pb.policy_change_ts, Utils.NOW)

  account:write()

  Utils.NOW.seconds = Utils.NOW.seconds + 3
  Account.CACHE = {}
  account = Account:get(KEYS[1])
  lu.assertEquals(account.pb.balance, 102)

  Utils.NOW.seconds = Utils.NOW.seconds + 1
  Account.CACHE = {}
  account = Account:get(KEYS[1])
  lu.assertEquals(account.pb.balance, 102)

  Utils.NOW.seconds = Utils.NOW.seconds + 1
  Account.CACHE = {}
  account = Account:get(KEYS[1])
  lu.assertEquals(account.pb.balance, 103)

  account:write()
  lu.assertEquals(redis.DATA[KEYS[1]], "\149g\145\206c\150\233>\145\206c\150\2339\146\170policy_key\171policy_name\132\001d\002\205\003\232\003\146\001\002\005\145{")
  lu.assertEquals(redis.TTL[KEYS[1]], 123000)
end

function testAccountBalanceExtreme()
  local account = Account:get(KEYS[1])
  account.pb.balance = 9007199254740991
  account:write() -- ok

  account.pb.balance = 9007199254740991 + 1
  lu.assertErrorMsgContains("overflow", account.write, account)

  account.pb.balance = -9007199254740991
  account:write() -- ok

  account.pb.balance = -9007199254740991 - 1
  lu.assertErrorMsgContains("underflows", account.write, account)
end

function testAccountApplyOpFinitePolicy()
  local account = Account:get(KEYS[1])

  local updateAccountPolicy = function(config, key, limit)
    local p = {
      pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
        limit = limit,
      }),
      policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
        config = config,
        key = key,
      }),
    }
    account:setPolicy(p)
  end

  local setOp = function(cur, amt, options, relative_to, ref)
    account.pb.balance = cur
    return mkOp({
      delta = amt,
      relative_to = relative_to or "ZERO",
      options = options or 0,
      policy_ref = ref,
    })
  end

  updateAccountPolicy("policy_key", "policy_name", 1000)

  lu.assertEquals(account:applyOp(setOp(20, 300)), nil)  -- increase < limit
  lu.assertEquals(account.pb.balance, 300)
  lu.assertEquals(account:applyOp(setOp(20, 5)), nil)    -- decrease > 0
  lu.assertEquals(account.pb.balance, 5)
  lu.assertEquals(account:applyOp(setOp(20, 1000)), nil) -- increase == limit
  lu.assertEquals(account.pb.balance, 1000)
  lu.assertEquals(account:applyOp(setOp(20, 0)), nil)    -- decrease == 0
  lu.assertEquals(account.pb.balance, 0)

  lu.assertEquals(account:applyOp(setOp(-100, 300)), nil)  -- increase < limit
  lu.assertEquals(account.pb.balance, 300)
  lu.assertEquals(account:applyOp(setOp(-100, -50)), nil)  -- increase < limit
  lu.assertEquals(account.pb.balance, -50)
  lu.assertEquals(account:applyOp(setOp(-100, 0)), nil)    -- increase == 0
  lu.assertEquals(account.pb.balance, 0)
  lu.assertEquals(account:applyOp(setOp(-100, 1000)), nil) -- increase == limit
  lu.assertEquals(account.pb.balance, 1000)

  lu.assertEquals(account:applyOp(setOp(2000, 300)), nil)  -- decrease < limit
  lu.assertEquals(account.pb.balance, 300)
  lu.assertEquals(account:applyOp(setOp(2000, 1000)), nil) -- decrease == limit
  lu.assertEquals(account.pb.balance, 1000)
  lu.assertEquals(account:applyOp(setOp(2000, 0)), nil)    -- decrease == 0
  lu.assertEquals(account.pb.balance, 0)

  -- soft cap
  lu.assertEquals(account:applyOp(setOp(2000, 1500)), nil) -- decrease < limit
  lu.assertEquals(account.pb.balance, 1000)
  -- hard cap
  lu.assertEquals(account:applyOp(setOp(2000, 1500, DO_NOT_CAP_PROPOSED)), nil) -- decrease < limit
  lu.assertEquals(account.pb.balance, 1500)

  local rslt = {}
  Account.applyOp(account, setOp(20, -1), rslt)
  lu.assertEquals(rslt.status, "ERR_UNDERFLOW")
  lu.assertEquals(account.pb.balance, 20)

  local rslt = {}
  Account.applyOp(account, setOp(-100, -200), rslt)
  lu.assertEquals(rslt.status, "ERR_UNDERFLOW")
  lu.assertEquals(account.pb.balance, -100)

  local rslt = {}
  Account.applyOp(account, setOp(20, 1001, DO_NOT_CAP_PROPOSED), rslt)
  lu.assertEquals(rslt.status, "ERR_OVERFLOW")
  lu.assertEquals(account.pb.balance, 20)

  local rslt = {}
  Account.applyOp(account, setOp(1500, 2000, DO_NOT_CAP_PROPOSED), rslt)
  lu.assertEquals(rslt.status, "ERR_OVERFLOW")
  lu.assertEquals(account.pb.balance, 1500)

  -- WITH_POLICY_LIMIT_DELTA test cases
  local rslt = {}
  updateAccountPolicy("config1", "key1", 5)
  local ref = setPolicy("config2", "key2", {limit = 10})
  Account.applyOp(account, setOp(3, -2, WITH_POLICY_LIMIT_DELTA, "CURRENT_BALANCE", ref), rslt) -- policy limit increase 5 < 10
  lu.assertEquals(rslt.previous_balance_adjusted, 8)
  lu.assertEquals(account.pb.balance, 6)

  local rslt = {}
  local ref = setPolicy("config3", "key3", {limit = 5})
  Account.applyOp(account, setOp(3, 1, WITH_POLICY_LIMIT_DELTA, "CURRENT_BALANCE", ref), rslt) -- policy limit decrease 10 > 5
  lu.assertEquals(rslt.previous_balance_adjusted, -2)
  lu.assertEquals(account.pb.balance, -1)

  local rslt = {}
  local ref = setPolicy("config4", "key4", {limit = 5})
  Account.applyOp(account, setOp(3, 1, WITH_POLICY_LIMIT_DELTA, "CURRENT_BALANCE", ref), rslt) -- old limit == new limit
  lu.assertEquals(rslt.previous_balance_adjusted, 3)
  lu.assertEquals(account.pb.balance, 4)

  account:setPolicy(nil) -- unset policy
  local rslt = {}
  local ref = setPolicy("config5", "key5", {limit = 5})
  Account.applyOp(account, setOp(3, 1, WITH_POLICY_LIMIT_DELTA, "CURRENT_BALANCE", ref), rslt) -- no-op since old policy_Ref == nil
  lu.assertEquals(rslt.previous_balance_adjusted, nil)
  lu.assertEquals(account.pb.balance, 4)
end

function testAccountApplyOpInfinitePolicy()
  local account = Account:get(KEYS[1])

  local p = {
    pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
      limit = 1000,
      refill = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy.Refill", {
        units = 1,
      })
    }),
    policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
      config = "policy_key",
      key = "policy_name",
    }),
  }
  account:setPolicy(p)

  -- note: we do not need to test cases where the current balance is something
  -- other than `pb.policy.limit`. We rely on Account construction and setPolicy
  -- to call applyRefill(), and for applyOp to always leave balance >= limit.
  local setOp = function(amt, options)
    account.pb.balance = p.pb.limit
    return mkOp({
      delta = amt,
      relative_to = "ZERO",
      options = options or 0,
    })
  end

  lu.assertEquals(account:applyOp(setOp(0)), nil)
  lu.assertEquals(account.pb.balance, 1000)
  lu.assertEquals(account:applyOp(setOp(1000)), nil)
  lu.assertEquals(account.pb.balance, 1000)
  lu.assertEquals(account:applyOp(setOp(500)), nil)
  lu.assertEquals(account.pb.balance, 1000)
  lu.assertEquals(account:applyOp(setOp(1234)), nil)
  lu.assertEquals(account.pb.balance, 1000)

  -- we can, however, start with a higher than limit balance.
  local op = setOp(1234, DO_NOT_CAP_PROPOSED)
  account.pb.balance = 2000
  lu.assertEquals(account:applyOp(op), nil)
  lu.assertEquals(account.pb.balance, 1234)

  local rslt = {}
  Account.applyOp(account, setOp(-1000), rslt)
  lu.assertEquals(rslt.status, "ERR_UNDERFLOW")
  lu.assertEquals(account.pb.balance, 1000)

  local rslt = {}
  Account.applyOp(account, setOp(1001, DO_NOT_CAP_PROPOSED), rslt)
  lu.assertEquals(rslt.status, "ERR_OVERFLOW")
  lu.assertEquals(account.pb.balance, 1000)

  local rslt = {}
  Account.applyOp(account, setOp(-1), rslt)
  lu.assertEquals(rslt.status, "ERR_UNDERFLOW")
  lu.assertEquals(account.pb.balance, 1000)
end

function testAccountApplyOpZeroPolicy()
  local account = Account:get(KEYS[1])

  local p = {
    pb = PB.new("go.chromium.org.luci.server.quota.quotapb.Policy", {
      limit = 0,
    }),
    policy_ref = PB.new("go.chromium.org.luci.server.quota.quotapb.PolicyRef", {
      config = "policy_key",
      key = "policy_name",
    }),
  }
  account:setPolicy(p)

  local setOp = function(cur, amt, options)
    account.pb.balance = cur
    return mkOp({
      delta = amt,
      relative_to = "ZERO",
      options = options,
    })
  end

  lu.assertEquals(account:applyOp(setOp(0, 0)), nil)
  lu.assertEquals(account.pb.balance, 0)
  lu.assertEquals(account:applyOp(setOp(100, 1)), nil)
  lu.assertEquals(account.pb.balance, 0)
  lu.assertEquals(account:applyOp(setOp(100, 1, DO_NOT_CAP_PROPOSED)), nil)
  lu.assertEquals(account.pb.balance, 1)
  lu.assertEquals(account:applyOp(setOp(2000, 1000)), nil)
  lu.assertEquals(account.pb.balance, 0)
  lu.assertEquals(account:applyOp(setOp(2000, 1000, DO_NOT_CAP_PROPOSED)), nil)
  lu.assertEquals(account.pb.balance, 1000)
  lu.assertEquals(account:applyOp(setOp(-2000, -1000)), nil)
  lu.assertEquals(account.pb.balance, -1000)

  local rslt = {}
  Account.applyOp(account, setOp(0, 1000, DO_NOT_CAP_PROPOSED), rslt)
  lu.assertEquals(rslt.status, "ERR_OVERFLOW")
  lu.assertEquals(account.pb.balance, 0)

  local rslt = {}
  Account.applyOp(account, setOp(-100, 1, DO_NOT_CAP_PROPOSED), rslt)
  lu.assertEquals(rslt.status, "ERR_OVERFLOW")
  lu.assertEquals(account.pb.balance, -100)

  local rslt = {}
  Account.applyOp(account, setOp(0, 1, DO_NOT_CAP_PROPOSED), rslt)
  lu.assertEquals(rslt.status, "ERR_OVERFLOW")
  lu.assertEquals(account.pb.balance, 0)

  local rslt = {}
  Account.applyOp(account, setOp(0, -1), rslt)
  lu.assertEquals(rslt.status, "ERR_UNDERFLOW")
  lu.assertEquals(account.pb.balance, 0)
end

function testAccountApplyOpAddPolicy()
  local ref = setPolicy("policy_config", "policy_name", {
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

  local account = Account:get(KEYS[1])

  account:applyOp(mkOp({
    policy_ref = ref,
    relative_to = "CURRENT_BALANCE",
    delta = 13,
  }))

  lu.assertEquals(account.pb.balance, 33)
end

function testApplyOpsOK()
  local pol_one = setPolicy("policy_config", "one", {
    default = 10,
    limit = 100,
  })
  local pol_two = setPolicy("policy_config", "two", {
    default = 20,
    limit = 200,
  })

  local rsp, allOK = Account.ApplyOps({
    mkOp({
      account_ref = "acct1",
      policy_ref = pol_one,
      relative_to = "CURRENT_BALANCE",
      delta = 100, -- should hit limit, stop at 100.
    }),
    mkOp({
      account_ref = "acct2",
      policy_ref = pol_two,
      relative_to = "CURRENT_BALANCE",
      delta = 100, -- stop at 120 under limit.
    }),
  })

  lu.assertTrue(allOK)
  lu.assertEquals(#rsp.results, 2)
  lu.assertEquals(rsp.results[1].account_status, AccountStatus.CREATED)
  lu.assertEquals(rsp.results[1].status_msg, "")
  lu.assertEquals(rsp.results[1].status, OpStatus.SUCCESS)
  lu.assertEquals(rsp.results[1].previous_balance, 0)
  lu.assertEquals(rsp.results[1].new_balance, 100)

  lu.assertEquals(rsp.results[2].account_status, AccountStatus.CREATED)
  lu.assertEquals(rsp.results[2].status_msg, "")
  lu.assertEquals(rsp.results[2].status, OpStatus.SUCCESS)
  lu.assertEquals(rsp.results[2].previous_balance, 0)
  lu.assertEquals(rsp.results[2].new_balance, 120)
end

function testApplyOpsFail()
  local pol_one = setPolicy("policy_config", "one", {
    default = 10,
    limit = 100,
  })

  local rsp, allOK = Account.ApplyOps({
    mkOp({
      account_ref = "acct1",
      relative_to = "CURRENT_BALANCE",
      options = DO_NOT_CAP_PROPOSED + IGNORE_POLICY_BOUNDS,
      delta = 1000,
    }),

    mkOp({
      account_ref = "acct2",
      policy_ref = pol_one,
      relative_to = "CURRENT_BALANCE",
      options = DO_NOT_CAP_PROPOSED,
      delta = 1000,
    }),

    mkOp({
      account_ref = "acct3",
      policy_ref = pol_one,
      relative_to = "CURRENT_BALANCE",
      delta = -1000,
    }),

    mkOp({
      account_ref = "acct4",
      policy_ref = {config = "nope", key = "missing"},
      relative_to = "CURRENT_BALANCE",
      delta = 100,
    }),

    mkOp({
      account_ref = "acct5",
      relative_to = "CURRENT_BALANCE",
      delta = 100,
    }),

    mkOp({
      account_ref = "acct6",
      relative_to = "ZERO",
      delta = 100,
    }),
  })

  lu.assertFalse(allOK)
  lu.assertEquals(#rsp.results, 6)
  lu.assertEquals(rsp.results[1].account_status, AccountStatus.CREATED)
  lu.assertStrContains(rsp.results[1].status_msg, "IGNORE_POLICY_BOUNDS and DO_NOT_CAP_PROPOSED both set")
  lu.assertEquals(rsp.results[1].status, OpStatus.ERR_UNKNOWN)

  lu.assertEquals(rsp.results[2].account_status, AccountStatus.CREATED)
  lu.assertEquals(rsp.results[2].status_msg, "")
  lu.assertEquals(rsp.results[2].status, OpStatus.ERR_OVERFLOW)

  lu.assertEquals(rsp.results[3].account_status, AccountStatus.CREATED)
  lu.assertEquals(rsp.results[3].status, OpStatus.ERR_UNDERFLOW)
  lu.assertEquals(rsp.results[3].status_msg, "")

  lu.assertEquals(rsp.results[4].account_status, AccountStatus.CREATED)
  lu.assertEquals(rsp.results[4].status, OpStatus.ERR_UNKNOWN_POLICY)
  lu.assertEquals(rsp.results[4].status_msg, "")

  lu.assertEquals(rsp.results[5].account_status, AccountStatus.CREATED)
  lu.assertEquals(rsp.results[5].status_msg, "")
  lu.assertEquals(rsp.results[5].status, OpStatus.ERR_POLICY_REQUIRED)
end

function testApplyOpsMultiSameAccount()
  local req = PB.new("go.chromium.org.luci.server.quota.quotapb.ApplyOpsRequest", {

  })
end

return lu.LuaUnit.run("-v")
