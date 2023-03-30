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

-- expect these to be passed in
local PB = ...
assert(PB)

local redis = redis

local Utils = {}

-- Get the current time; We treat the script execution as atomic, so all
-- accounts will be updated at exactly the same timestamp.
--
-- Note that time returns seconds and MICRO seconds (not milliseconds) within
-- that second.
local nowRaw = redis.call('TIME')
Utils.NOW = PB.new('google.protobuf.Timestamp', {
  seconds = tonumber(nowRaw[1]),
  nanos = tonumber(nowRaw[2]) * 1000,
})

local math_floor = math.floor

function Utils.Millis(duration)
  if duration == nil then
    return nil
  end
  return math_floor(duration.seconds * 1000 + duration.nanos / 1000000)
end

function Utils.WithRequestID(req, callback)
  local req_id_key = req.request_key

  if req_id_key ~= "" then
    local hash_scheme, hash, response = unpack(redis.call(
       "HMGET", req_id_key, "hash_scheme", "hash", "response"))
    if hash_scheme ~= nil then
      local my_scheme = tonumber(hash_scheme)
      if response then
        if req.hash_scheme ~= my_scheme then
          -- hash scheme mismatch; just return the response
          return response
        elseif req.hash_scheme == my_scheme and req.hash == hash then
          -- hash match; return response.
          return response
        else
          return redis.error_reply('REQUEST_HASH')
        end
      end
    end
  end

  -- either there was no request_key, or this is the first call.
  local ret, allOK = callback()

  local response = PB.marshal(ret)

  if allOK and req_id_key ~= "" then
    redis.call(
      "HSET", req_id_key,
      "hash_scheme", tostring(req.hash_scheme),
      "hash", req.hash,
      "response", response
    )
    local ttl = Utils.Millis(req.request_key_ttl)
    if ttl == nil then
      -- 2 hours * 60 minutes * 60 seconds * 1000 milliseconds.
      ttl = 2 * 60 * 60 * 1000
    end
    if ttl > 0 then
      redis.call("PEXPIRE", req_id_key, ttl)
    end
  end

  return response
end

return Utils
