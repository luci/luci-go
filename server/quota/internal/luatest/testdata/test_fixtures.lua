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

lu = require 'luaunit/luaunit'
-- This makes tracebacks work correctly; otherwise they show the location inside
-- of the luaunit assertion function. Suspect something to do with Go lua impl.
lu.STRIP_EXTRA_ENTRIES_IN_STACK_TRACE = 1


function dumpTable(table, depth)
  if depth == nil then
    depth = 0
  end
  if (depth > 200) then
    print("Error: Depth > 200 in dumpTable()")
    return
  end
  for k,v in pairs(table) do
    if (type(v) == "table") then
      print(string.rep("  ", depth)..k..":")
      dumpTable(v, depth+1)
    else
      print(string.rep("  ", depth)..k..": ",v)
    end
  end
end

KEYS = {"account_key~account_name", "policy_key"}

cmsgpack = require 'luamsgpack/MessagePack'

PB = loadfile('../../../quotapb/quotapb.lua')()

T0_SEC = 1670834486
T0_MSEC = 0

function DUMP(...)
  local toprint = {}
  for i, v in next, arg do
    if type(v) == "table" then
      v = cjson.encode(v)
    end
    toprint[i] = v
  end
  print(unpack(toprint))
end

redis = {
  DATA = {},
  TTL = {}
}

function redis.call(method, ...)
  local function check(nameTypes)
    if #nameTypes ~= arg.n then
      error(string.format("TEST ERROR: redis:call: %s: wrong arg.n: %d != %d", method, #nameTypes, args.n))
    end
    for i, nameType in ipairs(nameTypes) do
      local t = type(arg[i])
      local name, want = unpack(nameType)
      if t ~= want then
        error(string.format("TEST ERROR: redis:call: %s: bad %s type: %s, wanted %s", method, name, t, want))
      end
    end
    return unpack(arg)
  end

  if method == "HGET" then
    local key, field = check({{"key", "string"}, {"field", "string"}})
    return (redis.DATA[key] or {})[field]
  end

  if method == "HMGET" then
    local key, field, field2, field3 = check({
      {"key", "string"},
      {"field", "string"},
      {"field2", "string"},
      {"field3", "string"},
    })
    local subCache = (redis.DATA[key] or {})
    return {subCache[field], subCache[field2], subCache[field3]}
  end

  if method == "HSET" and arg.n == 3 then
    local key, field, value = check({
      {"key", "string"},
      {"field", "string"}, {"value", "string"},
    })
    local subCache = redis.DATA[key]
    if subCache == nil then
      subCache = {}
      redis.DATA[key] = subCache
    end
    subCache[field] = tostring(value)
    return
  end

  if method == "HSET" and arg.n == 5 then
    local key, field, value, field2, value2 = check({
      {"key", "string"},
      {"field", "string"}, {"value", "string"},
      {"field2", "string"}, {"value2", "string"},
    })
    local subCache = redis.DATA[key]
    if subCache == nil then
      subCache = {}
      redis.DATA[key] = subCache
    end
    subCache[field] = tostring(value)
    subCache[field2] = tostring(value3)
    return
  end

  if method == "HSET" then
    local key, field, value, field2, value2, field3, value3 = check({
      {"key", "string"},
      {"field", "string"}, {"value", "string"},
      {"field2", "string"}, {"value2", "string"},
      {"field3", "string"}, {"value3", "string"},
    })
    local subCache = redis.DATA[key]
    if subCache == nil then
      subCache = {}
      redis.DATA[key] = subCache
    end
    subCache[field] = tostring(value)
    subCache[field2] = tostring(value2)
    subCache[field3] = tostring(value3)
    return
  end

  if method == "GET" then
    local key = check({{"key", "string"}})
    return redis.DATA[key]
  end

  if method == "SET" and arg.n == 4 then
    local key, value, PX, TTL = check({
      {"key", "string"}, {"value", "string"},
      {"PX", "string"}, {"TTL", "number"},
    })
    if PX ~= "PX" then
      error(string.format("TEST ERROR: unexpected option %s != PX", PX))
    end
    redis.DATA[key] = value
    redis.TTL[key] = TTL
    return
  end

  if method == "SET" then
    local key, value = check({{"key", "string"}, {"value", "string"}})
    redis.DATA[key] = value
    redis.TTL[key] = nil
    return
  end

  if method == "PEXPIRE" then
    local key, exp = check({{"key", "string"}, {"exp", "number"}})
    redis.TTL[key] = exp
    return
  end

  if method == "TIME" then
    check({})
    return {T0_SEC, T0_MSEC}
  end

  error(string.format("TEST ERROR: redis.call unsupported method %s", method))
end

function redis.error_reply(msg)
  return { error = "ERR "..msg }
end

function redis.status_reply(obj)
  return { ok = obj }
end

local Utils = loadfile('../../lua/utils.lua')(PB, policyNames)
local Policy = loadfile('../../lua/policy.lua')(PB, Utils)
local Account = loadfile('../../lua/account.lua')(PB, Utils, Policy)

G = {}
G.Utils = Utils
G.Policy = Policy
G.Account = Account

-- This is a hack; we hook the output to reset global state.
-- The alternative would be to add a layer of 'test classes', but for our
-- purposes, this should be OK.
local realStartTest = lu.LuaUnit.outputType.startTest
lu.LuaUnit.outputType.startTest = function( self, testName )
  redis.DATA = {}
  redis.TTL = {}

  Policy.CACHE = {}
  Account.CACHE = {}
  Utils.NOW.seconds = T0_SEC
  Utils.NOW.nanos = T0_MSEC * 1000000

  realStartTest(self, testName)
end
