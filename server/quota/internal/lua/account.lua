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
local PB, Utils, Policy = ...
assert(PB)
assert(Utils)
assert(Policy)

local Account = {
  -- Account key => {
  --   key: (redis key),
  --   pb: PB go.chromium.org.luci.server.quota.Account,
  --   account_status: AccountStatus string,
  -- }
  CACHE = {},
}

-- TODO(iannucci) - make local references for all readonly dot-access stuff?
local math_floor = math.floor
local math_min = math.min
local math_max = math.max

local NOW = Utils.NOW
local redis_call = redis.call

local AccountPB = "go.chromium.org.luci.server.quota.quotapb.Account"

function Account:get(key)
  assert(key, "Account:get called with <nil>")
  assert(key ~= "", "Account:get called with ''")
  local entry = Account.CACHE[key]
  if not entry then
    local raw = redis_call("GET", key)
    entry = {
      key = key,
      -- NOTE: could keep track of whether this Account needs to be written, but
      -- if we are loading it, it means that the user has elected to perform an
      -- Op on this Account. Only in rather pathological cases would this result
      -- in a total no op (no refill, Op e.g. subtracts 0 to the balance). So,
      -- we elect to just always write all loaded Accounts at the end of the
      -- update script.
    }
    if raw then
      local ok, pb = pcall(PB.unmarshal, AccountPB, raw)
      if ok then
        entry.pb = pb
        entry.account_status = "ALREADY_EXISTS"
      else
        -- NOTE: not-ok implies that this account is broken; we treat this the
        -- same as a non-existant account (i.e. we'll make a new one).
        entry.account_status = "RECREATED"
      end
    else
      entry.account_status = "CREATED"
    end
    if not entry.pb then
      entry.pb = PB.new(AccountPB, {
        -- This Account will be (re)created, so it's updated_ts is NOW.
        updated_ts = NOW,
      })
    end
    setmetatable(entry, self)
    self.__index = self
    self.CACHE[key] = entry

    -- The entry was not new; it may have some refill policy to apply.
    --
    -- We set updated_ts here and not in applyRefill because the way refill is
    -- defined; We do the refill, under the existing policy, exactly once when
    -- the Account is loaded. Note that the existing policy could be missing,
    -- infinite, or just a regular refill policy. In ALL of those cases, we want
    -- any future policy application which happens in this invocation to
    -- calculate vs NOW, not vs any previous updated_ts time.
    --
    -- We could move this to applyRefill... but here it is a clearer indicator
    -- that NO applyRefill calls (e.g. in the case of applying an infinite
    -- refill policy via an Op) in this process will use any previous
    -- updated_ts.
    --
    -- You will note that updated_ts is only ever set to NOW in this whole
    -- program, and both of those assignments happen here, in this constructor.
    if entry.account_status == "ALREADY_EXISTS" then
      entry:applyRefill()
      entry.pb.updated_ts = NOW
    end
  end
  return entry
end

local RELATIVE_TO_CURRENT_BALANCE = "CURRENT_BALANCE"
local RELATIVE_TO_DEFAULT = "DEFAULT"
local RELATIVE_TO_LIMIT = "LIMIT"
local RELATIVE_TO_ZERO = "ZERO"

-- isInfiniteRefill determines if the policy has a refill policy, and if so, if
-- it is positively infinite.
--
-- An 'infinite refill policy' is one where the `interval` (i.e. how frequently
-- `units` apply is 0) and `units` is positive. This is because the policy would
-- logically fill the account at a rate of `units / interval` (thus answering
-- the age-old question of what happens when you divide by zero).
--
-- It is not valid to have an interval of 0 with units <= 0. "Negative Infinity"
-- policies are better represented by just setting the policy limit to 0. This
-- function raises an error in such a case.
local isInfiniteRefill = function(policy)
  if not policy then
    return false
  end

  if not policy.refill then
    return false
  end

  if policy.refill.interval ~= 0 then
    return false
  end

  if policy.refill.units <= 0 then
    error("invalid zero-interval refill policy")
  end

  return true
end

local opts = PB.E["go.chromium.org.luci.server.quota.quotapb.Op.Options"]
local IGNORE_POLICY_BOUNDS = opts.IGNORE_POLICY_BOUNDS
local DO_NOT_CAP_PROPOSED = opts.DO_NOT_CAP_PROPOSED
local WITH_POLICY_LIMIT_DELTA = opts.WITH_POLICY_LIMIT_DELTA

-- computeProposed computes the new, proposed, balance value for an account.
--
-- Args:
--   * op -- The Op message
--   * new_account (bool) -- True if this acccount is 'new' (i.e. prior to this
--     Op, the Account did not exist)
--   * current (number) -- The balance of the account (undefined for new
--     accounts).
--   * limit (number or nil) -- The limit of the account's policy, or nil if the
--     account has no policy.
--   * default (number or nil) -- The default for a new account, or nil if the
--     account has no policy.
--
-- Returns the proposed account balance (number) plus a status string (if there
-- was a error). If `op` is malformed, this raises an error.
local computeProposed = function(op, new_account, current, policy)
  local relative_to = op.relative_to
  if relative_to == RELATIVE_TO_ZERO then
    return op.delta
  end

  if policy == nil and new_account then
    -- no policy and this account didn't exist? `current` is undefined, since we
    -- don't know what its default should be.
    return 0, "ERR_POLICY_REQUIRED"
  end

  if relative_to == RELATIVE_TO_CURRENT_BALANCE then
    return current + op.delta
  end

  if policy == nil then
    return 0, "ERR_POLICY_REQUIRED"
  end

  if relative_to == RELATIVE_TO_LIMIT then
    return policy.limit + op.delta
  end

  if relative_to == RELATIVE_TO_DEFAULT then
    return policy.default + op.delta
  end

  error("invalid `relative_to` value: "..op.relative_to)
end

function Account:applyOp(op, result)
  -- this weird construction checks if the bit is set in options.
  local options = op.options
  local ignore_bounds = (options/IGNORE_POLICY_BOUNDS)%2 >= 1
  local no_cap = (options/DO_NOT_CAP_PROPOSED)%2 >= 1
  local with_policy_limit_delta = (options/WITH_POLICY_LIMIT_DELTA)%2 >= 1

  -- If there is a policy_ref attempt to set the policy.
  if op.policy_ref ~= nil then
    local policy_raw = Policy.get(op.policy_ref)
    if not policy_raw then
      result.status = "ERR_UNKNOWN_POLICY"
      return
    end
    self:setPolicy(policy_raw, result, with_policy_limit_delta)
  end
  local pb = self.pb
  local policy = pb.policy

  if ignore_bounds and no_cap then
    error("IGNORE_POLICY_BOUNDS and DO_NOT_CAP_PROPOSED both set")
  end

  local current = pb.balance

  -- step 1; figure out what value they want to set the account to.
  -- NOTE: computeProposed will return an error status if the op wants to compute
  -- something relative to a value we don't have (e.g. the balance for a `new`
  -- Account, or the limit/default for a policy-less Account).
  local proposed, status = computeProposed(op, self.account_status ~= "ALREADY_EXISTS", current, policy)
  if status ~= nil then
    result.status = status
    return
  end

  local limit = nil
  if policy then
    limit = policy.limit
  end
  if not (no_cap or ignore_bounds) then
    -- We haven't been instructed to cap the proposed value by policy.limit, and
    -- we also haven't been instructed to completely ignore the bounds.
    proposed = math_min(proposed, limit)
  end

  -- step 2, figure out how to apply the proposed value.
  if ignore_bounds then
    -- No boundaries matter, use the proposed value as-is.
    pb.balance = proposed
  elseif policy == nil then
    -- You cannot apply a value to an Account lacking a policy, without using
    -- the ignore_bounds flag, so check that here.
    result.status = "ERR_POLICY_REQUIRED"
    return
  elseif proposed >= 0 and proposed <= limit then
    -- We are respecting policy bounds, and this proposed value is "in bounds".
    if isInfiniteRefill(policy) then
      -- setting a value 'in bounds' with an infinite policy means that the
      -- balance automatically replenishes to policy.limit. Note that pb.balance
      -- could, itself, be out of bounds here (i.e. balance could be negative or
      -- over the limit), but the proposal puts us in [0, limit], whereupon the
      -- policy.refill now applies.
      pb.balance = limit
    else
      pb.balance = proposed
    end
  else
    -- We are respecting policy bounds, and this proposed value is "out of bounds"
    -- (either below 0 or above policy.limit).
    --
    -- We allow updates which bring the account balance back towards [0, limit].
    -- That is, we allow a credit to a negative account balance, and we allow
    -- a debit from an overflowed account balance.
    --
    -- However we wouldn't allow making a negative balance MORE negative or an
    -- overflowed balance MORE overflowed.
    --
    -- NOTE: Even with an infinite refill policy, you are not allowed to reduce
    -- the balance below 0 without UNDERFLOW-ing.
    if proposed < 0 and proposed < current then
      result.status = "ERR_UNDERFLOW"
      return
    end
    if proposed > limit and proposed > current then
      result.status = "ERR_OVERFLOW"
      return
    end
    pb.balance = proposed
  end

  -- We applied an op without error; the Account is now "ALREADY_EXISTS" for any
  -- subsequent ops.
  self.account_status = "ALREADY_EXISTS"
end

function Account:applyRefill()
  -- NOTE: in this function we assume that policy has already been validated and
  -- follows all validation rules set in `policy.proto`.
  local policy = self.pb.policy
  if policy == nil then
    return
  end
  local limit = policy.limit

  local refill = policy.refill
  if refill == nil then
    return
  end

  local curBalance = self.pb.balance

  if isInfiniteRefill(policy) then
    self.pb.balance = math_max(curBalance, limit)
    return
  end

  local units = refill.units
  local interval = refill.interval

  -- we subtract offset from all timestamps for this calculation; this has the
  -- effect of shifting midnight forwards by pushing all the timestamps
  -- backwards.
  --
  -- NOTE - Because interval and offset are defined as UTC midnight aligned
  -- seconds, we can completely ignore the nanos field in both updated_ts and
  -- NOW, since they would be obliterated by the % interval logic below anyway.
  local offset = refill.offset
  local updated_unix = self.pb.updated_ts.seconds - offset
  local now_unix = NOW.seconds - offset

  -- Intervals:  A     B     C     D
  -- Timestamps:    U          N
  --
  -- first_event_unix == B
  -- last_event_unix == C
  --
  -- num_events == 2

  -- find first refill event after updated_ts
  local first_event_unix = (updated_unix - (updated_unix % interval)) + interval
  -- find last refill event which happened before NOW
  local last_event_unix = (now_unix - (now_unix % interval))

  if last_event_unix < first_event_unix then
    return
  end

  local num_events = ((last_event_unix - first_event_unix) / interval) + 1
  -- we should always have an integer number of events
  assert(math_floor(num_events) == num_events)

  local delta = num_events * units
  if delta > 0 and curBalance < limit then
    self.pb.balance = math_min(curBalance+delta, limit)
  elseif delta < 0 and curBalance > 0 then
    self.pb.balance = math_max(curBalance+delta, 0)
  end
end

local policyRefEq = function(a, b)
  if a == nil and b == nil then
    return true
  end
  if a == nil and b ~= nil then
    return false
  end
  if a ~= nil and b == nil then
    return false
  end
  return a.config == b.config and a.key == b.key
end

function Account:setPolicy(policy, result, with_policy_limit_delta)
  if policy then
    -- sets Policy on this Account, updating its policy entry and replenishing it.
    if not policyRefEq(self.pb.policy_ref, policy.policy_ref) then
      -- When with_policy_limit_delta is set for an account with an existing
      -- policy_ref, and there is a policy update, add the delta of the new
      -- policy limit and the old policy limit to the current account balance.
      if with_policy_limit_delta and self.pb.policy_ref ~= nil then
        local delta = policy.pb.limit - self.pb.policy.limit
        self.pb.balance = self.pb.balance + delta

        -- Update previous_balance_adjusted to the updated balance.
        result.previous_balance_adjusted = self.pb.balance
      end

      self.pb.policy = policy.pb
      self.pb.policy_ref = policy.policy_ref
      self.pb.policy_change_ts = NOW
    end

    -- this account didn't exist before; set the initial value.
    if self.account_status ~= "ALREADY_EXISTS" then
      -- if multiple ops apply to this Account, treat the account as existing
      -- from this point on.
      self.pb.balance = policy.pb.default
    end

    self:applyRefill() -- in case this policy is infinite, this will set the balance to limit.
  else
    -- explicitly unset the policy; no further action is needed.
    self.pb.policy = nil
    self.pb.policy_ref = nil
    self.pb.policy_change_ts = NOW
  end
end

function Account:write()
  local lifetime_ms = nil
  if self.pb.policy then
    lifetime_ms = Utils.Millis(self.pb.policy.lifetime)
  end

  local raw = PB.marshal(self.pb)
  if lifetime_ms then
    redis_call('SET', self.key, raw, 'PX', lifetime_ms)
  else
    redis_call('SET', self.key, raw)
  end
end

function Account.ApplyOps(oplist)
  local ret = PB.new("go.chromium.org.luci.server.quota.quotapb.ApplyOpsResponse")
  local allOK = true
  for i, op in ipairs(oplist) do
    local account = Account:get(op.account_ref)

    local result = PB.new("go.chromium.org.luci.server.quota.quotapb.OpResult", {
      status = "SUCCESS",  -- by default; applyOp can overwrite this.
      account_status = account.account_status,
      previous_balance = account.pb.balance,
      -- This will be updated if WITH_POLICY_LIMIT_DELTA is set, the account
      -- has an existing policy_ref, and the op introduces a policy change.
      previous_balance_adjusted = account.pb.balance,
    })
    ret.results[i] = result

    -- applyOp should never raise an error, so any caught here are unknown.
    local ok, err = pcall(account.applyOp, account, op, result)
    if not ok then
      result.status = "ERR_UNKNOWN"
      result.status_msg = tostring(err)
    end

    if result.status == "SUCCESS" then
      result.new_balance = account.pb.balance
    else
      allOK = false
    end
  end

  if allOK then
    for key, account in pairs(Account.CACHE) do
      account:write()
    end
    ret.originally_set = NOW
  end

  return ret, allOK
end

return Account
