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

function testMillis()
  lu.assertEquals(Utils.Millis(nil), nil)

  local dur = PB.new("google.protobuf.Duration", {
    seconds = 127,
    nanos = 19304658,
  })
  lu.assertEquals(Utils.Millis(dur), 127019)
end

function testWithRequestIDNoKey()
  local req = PB.new("go.chromium.org.luci.server.quota.quotapb.UpdateAccountsInput")

  local ret = Utils.WithRequestID(req, function()
    return PB.new("google.protobuf.Duration", {
      seconds = 100,
    }), true
  end)
  lu.assertEquals(ret, "\145d")
  lu.assertEquals(redis.DATA, {})
  lu.assertEquals(redis.TTL, {})
end

function testWithRequestIDWithKeyAndHash()
  local req = PB.new("go.chromium.org.luci.server.quota.quotapb.UpdateAccountsInput", {
    request_key = "thing",
    request_key_ttl = PB.new("google.protobuf.Duration", {
      seconds = 127,
      nanos = 19304658,
    }),
    hash_scheme = 1,
    hash = "i am a banana.",
  })

  local ret = Utils.WithRequestID(req, function()
    return PB.new("google.protobuf.Duration", {
      seconds = 100,
    }), true
  end)
  lu.assertEquals(ret, "\145d")

  lu.assertEquals(redis.DATA["thing"], {
    hash="i am a banana.",
    hash_scheme="1",
    response="\145d",
  })
  lu.assertEquals(redis.TTL["thing"], 127019)

  local ret = Utils.WithRequestID(req, function()
    error("no! this should be deduplicated!")
  end)
  lu.assertEquals(ret, "\145d")

  req.hash = "not banana"
  local ret = Utils.WithRequestID(req, function()
    error("no! this should be deduplicated!")
  end)
  lu.assertEquals(ret, {
    error = "ERR REQUEST_HASH",
  })

  req.hash_scheme = 2 -- allowed
  local ret = Utils.WithRequestID(req, function()
    error("no! this should be deduplicated!")
  end)
  lu.assertEquals(ret, "\145d")
end

function testWithRequestIDError()
  local req = PB.new("go.chromium.org.luci.server.quota.quotapb.UpdateAccountsInput", {
    request_key = "thing",
    request_key_ttl = PB.new("google.protobuf.Duration", {
      seconds = 127,
      nanos = 19304658,
    }),
    hash_scheme = 1,
    hash = "i am a banana.",
  })

  local ret = Utils.WithRequestID(req, function()
    return PB.new("google.protobuf.Duration", {
      seconds = 100,
    }), false
  end)
  lu.assertEquals(ret, "\145d")
  lu.assertEquals(redis.DATA, {})
  lu.assertEquals(redis.TTL, {})
end

return lu.LuaUnit.run("-v")
