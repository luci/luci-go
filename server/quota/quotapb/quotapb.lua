local PB = {}

local next = next
local type = type

PB.E = {
  ["go.chromium.org.luci.server.quota.quotapb.Op.Options"] = {
    ["NO_OPTIONS"] = 0,
    [0] = "NO_OPTIONS",
    ["IGNORE_POLICY_BOUNDS"] = 1,
    [1] = "IGNORE_POLICY_BOUNDS",
    ["DO_NOT_CAP_PROPOSED"] = 2,
    [2] = "DO_NOT_CAP_PROPOSED",
  },

  ["go.chromium.org.luci.server.quota.quotapb.Op.RelativeTo"] = {
    ["CURRENT_BALANCE"] = 0,
    [0] = "CURRENT_BALANCE",
    ["ZERO"] = 1,
    [1] = "ZERO",
    ["DEFAULT"] = 2,
    [2] = "DEFAULT",
    ["LIMIT"] = 3,
    [3] = "LIMIT",
  },

  ["go.chromium.org.luci.server.quota.quotapb.OpError.Status"] = {
    ["UNKNOWN"] = 0,
    [0] = "UNKNOWN",
    ["OVERFLOW"] = 1,
    [1] = "OVERFLOW",
    ["UNDERFLOW"] = 2,
    [2] = "UNDERFLOW",
    ["UNKNOWN_POLICY"] = 3,
    [3] = "UNKNOWN_POLICY",
    ["MISSING_ACCOUNT"] = 4,
    [4] = "MISSING_ACCOUNT",
    ["POLICY_REQUIRED"] = 5,
    [5] = "POLICY_REQUIRED",
  },
}

PB.M = {
  ["go.chromium.org.luci.server.quota.quotapb.Account"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["balance"] -- 1: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field balance: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field balance: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field balance: underflows lua min integer")
        end
        acc[1] = val
      end

      val = obj["updated_ts"] -- 2: google.protobuf.Timestamp
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field updated_ts: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field updated_ts: missing type")
        end
        if val["$type"] ~= "google.protobuf.Timestamp" then
          error("field updated_ts: expected message type 'google.protobuf.Timestamp', but got "..val["$type"])
        end
        val = PB.M["google.protobuf.Timestamp"].marshal(val)
        acc[2] = val
      end

      val = obj["policy_change_ts"] -- 3: google.protobuf.Timestamp
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field policy_change_ts: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field policy_change_ts: missing type")
        end
        if val["$type"] ~= "google.protobuf.Timestamp" then
          error("field policy_change_ts: expected message type 'google.protobuf.Timestamp', but got "..val["$type"])
        end
        val = PB.M["google.protobuf.Timestamp"].marshal(val)
        acc[3] = val
      end

      val = obj["policy_ref"] -- 4: go.chromium.org.luci.server.quota.quotapb.PolicyRef
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field policy_ref: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field policy_ref: missing type")
        end
        if val["$type"] ~= "go.chromium.org.luci.server.quota.quotapb.PolicyRef" then
          error("field policy_ref: expected message type 'go.chromium.org.luci.server.quota.quotapb.PolicyRef', but got "..val["$type"])
        end
        val = PB.M["go.chromium.org.luci.server.quota.quotapb.PolicyRef"].marshal(val)
        acc[4] = val
      end

      val = obj["policy"] -- 5: go.chromium.org.luci.server.quota.quotapb.Policy
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field policy: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field policy: missing type")
        end
        if val["$type"] ~= "go.chromium.org.luci.server.quota.quotapb.Policy" then
          error("field policy: expected message type 'go.chromium.org.luci.server.quota.quotapb.Policy', but got "..val["$type"])
        end
        val = PB.M["go.chromium.org.luci.server.quota.quotapb.Policy"].marshal(val)
        acc[5] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.Account",
        ["balance"] = 0,
        ["updated_ts"] = nil,
        ["policy_change_ts"] = nil,
        ["policy_ref"] = nil,
        ["policy"] = nil,
      }
      local dec = {
        [1] = function(val) -- balance: int64
          local T = type(val)
          if T ~= "number" then
            error("field balance: expected number, but got "..T)
          end
          ret["balance"] = val
        end,
        [2] = function(val) -- updated_ts: google.protobuf.Timestamp
          local T = type(val)
          if T ~= "table" then
            error("field updated_ts: expected table, but got "..T)
          end
          ret["updated_ts"] = PB.M["google.protobuf.Timestamp"].unmarshal(val)
        end,
        [3] = function(val) -- policy_change_ts: google.protobuf.Timestamp
          local T = type(val)
          if T ~= "table" then
            error("field policy_change_ts: expected table, but got "..T)
          end
          ret["policy_change_ts"] = PB.M["google.protobuf.Timestamp"].unmarshal(val)
        end,
        [4] = function(val) -- policy_ref: go.chromium.org.luci.server.quota.quotapb.PolicyRef
          local T = type(val)
          if T ~= "table" then
            error("field policy_ref: expected table, but got "..T)
          end
          ret["policy_ref"] = PB.M["go.chromium.org.luci.server.quota.quotapb.PolicyRef"].unmarshal(val)
        end,
        [5] = function(val) -- policy: go.chromium.org.luci.server.quota.quotapb.Policy
          local T = type(val)
          if T ~= "table" then
            error("field policy: expected table, but got "..T)
          end
          ret["policy"] = PB.M["go.chromium.org.luci.server.quota.quotapb.Policy"].unmarshal(val)
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["balance"] = true,
      ["updated_ts"] = true,
      ["policy_change_ts"] = true,
      ["policy_ref"] = true,
      ["policy"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.ApplyOpsResponse"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["balances"] -- 1: repeated int64
      if next(val) ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field balances: expected list[int64], but got "..T)
        end
        local maxIdx = 0
        local length = 0
        for k, v in next, val do
          if type(k) ~= "number" then
            error("field balances: expected list[int64], but got table")
          end
          local T = type(v)
          if T ~= "number" then
            error("field balances["..(i-1).."]: expected number, but got "..T)
          end
          if v > 9007199254740991 then
            error("field balances["..(i-1).."]: overflows lua max integer")
          end
          if v < -9007199254740991 then
            error("field balances["..(i-1).."]: underflows lua min integer")
          end
          if k > maxIdx then
            maxIdx = k
          end
          length = length + 1
        end
        if length ~= maxIdx then
          error("field balances: expected list[int64], but got table")
        end
        acc[1] = val
      end

      val = obj["originally_set"] -- 2: google.protobuf.Timestamp
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field originally_set: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field originally_set: missing type")
        end
        if val["$type"] ~= "google.protobuf.Timestamp" then
          error("field originally_set: expected message type 'google.protobuf.Timestamp', but got "..val["$type"])
        end
        val = PB.M["google.protobuf.Timestamp"].marshal(val)
        acc[2] = val
      end

      val = obj["errors"] -- 3: map<uint32, go.chromium.org.luci.server.quota.quotapb.OpError>
      if next(val) ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field errors: expected map<uint32, message>, but got "..T)
        end
        local i = 0
        for k, v in next, val do
          local T = type(k)
          if T ~= "number" then
            error("field errors["..i.."th entry]: expected number, but got "..T)
          end
          if k < 0 then
            error("field errors["..i.."th entry]: negative")
          end
          if k > 4294967295 then
            error("field errors["..i.."th entry]: overflows max uint32")
          end
          local T = type(v)
          if T ~= "table" then
            error("field errors["..k.."]: expected table, but got "..T)
          end
          if not v["$type"] then
            error("field errors["..k.."]: missing type")
          end
          if v["$type"] ~= "go.chromium.org.luci.server.quota.quotapb.OpError" then
            error("field errors["..k.."]: expected message type 'go.chromium.org.luci.server.quota.quotapb.OpError', but got "..v["$type"])
          end
          val[k] = PB.M["go.chromium.org.luci.server.quota.quotapb.OpError"].marshal(v)
          i = i + 1
        end
        acc[3] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.ApplyOpsResponse",
        ["balances"] = {},
        ["originally_set"] = nil,
        ["errors"] = {},
      }
      local dec = {
        [1] = function(val) -- balances: repeated int64
          local T = type(val)
          if T ~= "table" then
            error("field balances: expected list[int64], but got "..T)
          end
          local max = 0
          local count = 0
          for i, v in next, val do
            if type(i) ~= "number" then
              error("field balances: expected list[int64], but got table")
            end
            if i > max then
              max = i
            end
            count = count + 1
            local T = type(v)
            if T ~= "number" then
              error("field balances["..(i-1).."]: expected number, but got "..T)
            end
            val[i] = val
          end
          if max ~= count then
            error("field balances: expected list[int64], but got table")
          end
          ret["balances"] = val
        end,
        [2] = function(val) -- originally_set: google.protobuf.Timestamp
          local T = type(val)
          if T ~= "table" then
            error("field originally_set: expected table, but got "..T)
          end
          ret["originally_set"] = PB.M["google.protobuf.Timestamp"].unmarshal(val)
        end,
        [3] = function(val) -- errors: map<uint32, go.chromium.org.luci.server.quota.quotapb.OpError>
          local T = type(val)
          if T ~= "table" then
            error("field errors: expected map<uint32, message>, but got "..T)
          end
          local i = 0
          for k, v in next, val do
            local T = type(k)
            if T ~= "number" then
              error("field errors["..i.."th entry]: expected number, but got "..T)
            end
            k = val
            local T = type(v)
            if T ~= "table" then
              error("field errors["..k.."]: expected table, but got "..T)
            end
            val[k] = PB.M["go.chromium.org.luci.server.quota.quotapb.OpError"].unmarshal(v)
            i = i + 1
          end
          ret["errors"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["balances"] = true,
      ["originally_set"] = true,
      ["errors"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.OpError"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["status"] -- 1: enum go.chromium.org.luci.server.quota.quotapb.OpError.Status
      if val ~= 0 and val ~= "UNKNOWN" then
        local T = type(val)
        local origval = val
        if T == "string" then
          val = PB.E["go.chromium.org.luci.server.quota.quotapb.OpError.Status"][val]
          if val == nil then
            error("field status: bad string enum value "..origval)
          end
        elseif T == "number" then
          if PB.E["go.chromium.org.luci.server.quota.quotapb.OpError.Status"][val] == nil then
            error("field status: bad numeric enum value "..origval)
          end
        else
          error("field status: expected number or string, but got "..T)
        end
        acc[1] = val
      end

      val = obj["balance"] -- 2: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field balance: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field balance: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field balance: underflows lua min integer")
        end
        acc[2] = val
      end

      val = obj["info"] -- 3: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field info: expected string, but got "..T)
        end
        acc[3] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.OpError",
        ["status"] = "UNKNOWN",
        ["balance"] = 0,
        ["info"] = "",
      }
      local dec = {
        [1] = function(val) -- status: enum go.chromium.org.luci.server.quota.quotapb.OpError.Status
          local T = type(val)
          if T ~= "number" then
            error("field status: expected numeric enum, but got "..T)
          end
          local origval = val
          local newval = PB.E["go.chromium.org.luci.server.quota.quotapb.OpError.Status"][val]
          if newval == nil then
            error("field status: bad enum value "..origval)
          end
          ret["status"] = newval
        end,
        [2] = function(val) -- balance: int64
          local T = type(val)
          if T ~= "number" then
            error("field balance: expected number, but got "..T)
          end
          ret["balance"] = val
        end,
        [3] = function(val) -- info: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field info: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field info: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field info: expected string, but got "..T)
          end
          ret["info"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["status"] = true,
      ["balance"] = true,
      ["info"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.Policy"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["default"] -- 1: uint64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field default: expected number, but got "..T)
        end
        if val < 0 then
          error("field default: negative")
        end
        if val > 9007199254740991 then
          error("field default: overflows lua max integer")
        end
        acc[1] = val
      end

      val = obj["limit"] -- 2: uint64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field limit: expected number, but got "..T)
        end
        if val < 0 then
          error("field limit: negative")
        end
        if val > 9007199254740991 then
          error("field limit: overflows lua max integer")
        end
        acc[2] = val
      end

      val = obj["refill"] -- 3: go.chromium.org.luci.server.quota.quotapb.Policy.Refill
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field refill: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field refill: missing type")
        end
        if val["$type"] ~= "go.chromium.org.luci.server.quota.quotapb.Policy.Refill" then
          error("field refill: expected message type 'go.chromium.org.luci.server.quota.quotapb.Policy.Refill', but got "..val["$type"])
        end
        val = PB.M["go.chromium.org.luci.server.quota.quotapb.Policy.Refill"].marshal(val)
        acc[3] = val
      end

      val = obj["options"] -- 4: int32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field options: expected number, but got "..T)
        end
        if val > 2147483647 then
          error("field options: overflows int32")
        end
        if val < -2147483648 then
          error("field options: underflows int32")
        end
        acc[4] = val
      end

      val = obj["lifetime"] -- 5: google.protobuf.Duration
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field lifetime: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field lifetime: missing type")
        end
        if val["$type"] ~= "google.protobuf.Duration" then
          error("field lifetime: expected message type 'google.protobuf.Duration', but got "..val["$type"])
        end
        val = PB.M["google.protobuf.Duration"].marshal(val)
        acc[5] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.Policy",
        ["default"] = 0,
        ["limit"] = 0,
        ["refill"] = nil,
        ["options"] = 0,
        ["lifetime"] = nil,
      }
      local dec = {
        [1] = function(val) -- default: uint64
          local T = type(val)
          if T ~= "number" then
            error("field default: expected number, but got "..T)
          end
          ret["default"] = val
        end,
        [2] = function(val) -- limit: uint64
          local T = type(val)
          if T ~= "number" then
            error("field limit: expected number, but got "..T)
          end
          ret["limit"] = val
        end,
        [3] = function(val) -- refill: go.chromium.org.luci.server.quota.quotapb.Policy.Refill
          local T = type(val)
          if T ~= "table" then
            error("field refill: expected table, but got "..T)
          end
          ret["refill"] = PB.M["go.chromium.org.luci.server.quota.quotapb.Policy.Refill"].unmarshal(val)
        end,
        [4] = function(val) -- options: int32
          local T = type(val)
          if T ~= "number" then
            error("field options: expected number, but got "..T)
          end
          ret["options"] = val
        end,
        [5] = function(val) -- lifetime: google.protobuf.Duration
          local T = type(val)
          if T ~= "table" then
            error("field lifetime: expected table, but got "..T)
          end
          ret["lifetime"] = PB.M["google.protobuf.Duration"].unmarshal(val)
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["default"] = true,
      ["limit"] = true,
      ["refill"] = true,
      ["options"] = true,
      ["lifetime"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.Policy.Refill"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["units"] -- 1: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field units: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field units: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field units: underflows lua min integer")
        end
        acc[1] = val
      end

      val = obj["interval"] -- 2: uint32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field interval: expected number, but got "..T)
        end
        if val < 0 then
          error("field interval: negative")
        end
        if val > 4294967295 then
          error("field interval: overflows max uint32")
        end
        acc[2] = val
      end

      val = obj["offset"] -- 3: uint32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field offset: expected number, but got "..T)
        end
        if val < 0 then
          error("field offset: negative")
        end
        if val > 4294967295 then
          error("field offset: overflows max uint32")
        end
        acc[3] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.Policy.Refill",
        ["units"] = 0,
        ["interval"] = 0,
        ["offset"] = 0,
      }
      local dec = {
        [1] = function(val) -- units: int64
          local T = type(val)
          if T ~= "number" then
            error("field units: expected number, but got "..T)
          end
          ret["units"] = val
        end,
        [2] = function(val) -- interval: uint32
          local T = type(val)
          if T ~= "number" then
            error("field interval: expected number, but got "..T)
          end
          ret["interval"] = val
        end,
        [3] = function(val) -- offset: uint32
          local T = type(val)
          if T ~= "number" then
            error("field offset: expected number, but got "..T)
          end
          ret["offset"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["units"] = true,
      ["interval"] = true,
      ["offset"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.PolicyRef"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["config"] -- 1: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field config: expected string, but got "..T)
        end
        acc[1] = val
      end

      val = obj["key"] -- 2: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field key: expected string, but got "..T)
        end
        acc[2] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.PolicyRef",
        ["config"] = "",
        ["key"] = "",
      }
      local dec = {
        [1] = function(val) -- config: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field config: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field config: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field config: expected string, but got "..T)
          end
          ret["config"] = val
        end,
        [2] = function(val) -- key: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field key: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field key: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field key: expected string, but got "..T)
          end
          ret["key"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["config"] = true,
      ["key"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.RawOp"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["account_ref"] -- 1: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field account_ref: expected string, but got "..T)
        end
        acc[1] = val
      end

      val = obj["policy_ref"] -- 2: go.chromium.org.luci.server.quota.quotapb.PolicyRef
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field policy_ref: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field policy_ref: missing type")
        end
        if val["$type"] ~= "go.chromium.org.luci.server.quota.quotapb.PolicyRef" then
          error("field policy_ref: expected message type 'go.chromium.org.luci.server.quota.quotapb.PolicyRef', but got "..val["$type"])
        end
        val = PB.M["go.chromium.org.luci.server.quota.quotapb.PolicyRef"].marshal(val)
        acc[2] = val
      end

      val = obj["relative_to"] -- 3: enum go.chromium.org.luci.server.quota.quotapb.Op.RelativeTo
      if val ~= 0 and val ~= "CURRENT_BALANCE" then
        local T = type(val)
        local origval = val
        if T == "string" then
          val = PB.E["go.chromium.org.luci.server.quota.quotapb.Op.RelativeTo"][val]
          if val == nil then
            error("field relative_to: bad string enum value "..origval)
          end
        elseif T == "number" then
          if PB.E["go.chromium.org.luci.server.quota.quotapb.Op.RelativeTo"][val] == nil then
            error("field relative_to: bad numeric enum value "..origval)
          end
        else
          error("field relative_to: expected number or string, but got "..T)
        end
        acc[3] = val
      end

      val = obj["delta"] -- 4: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field delta: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field delta: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field delta: underflows lua min integer")
        end
        acc[4] = val
      end

      val = obj["options"] -- 5: uint32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field options: expected number, but got "..T)
        end
        if val < 0 then
          error("field options: negative")
        end
        if val > 4294967295 then
          error("field options: overflows max uint32")
        end
        acc[5] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.RawOp",
        ["account_ref"] = "",
        ["policy_ref"] = nil,
        ["relative_to"] = "CURRENT_BALANCE",
        ["delta"] = 0,
        ["options"] = 0,
      }
      local dec = {
        [1] = function(val) -- account_ref: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field account_ref: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field account_ref: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field account_ref: expected string, but got "..T)
          end
          ret["account_ref"] = val
        end,
        [2] = function(val) -- policy_ref: go.chromium.org.luci.server.quota.quotapb.PolicyRef
          local T = type(val)
          if T ~= "table" then
            error("field policy_ref: expected table, but got "..T)
          end
          ret["policy_ref"] = PB.M["go.chromium.org.luci.server.quota.quotapb.PolicyRef"].unmarshal(val)
        end,
        [3] = function(val) -- relative_to: enum go.chromium.org.luci.server.quota.quotapb.Op.RelativeTo
          local T = type(val)
          if T ~= "number" then
            error("field relative_to: expected numeric enum, but got "..T)
          end
          local origval = val
          local newval = PB.E["go.chromium.org.luci.server.quota.quotapb.Op.RelativeTo"][val]
          if newval == nil then
            error("field relative_to: bad enum value "..origval)
          end
          ret["relative_to"] = newval
        end,
        [4] = function(val) -- delta: int64
          local T = type(val)
          if T ~= "number" then
            error("field delta: expected number, but got "..T)
          end
          ret["delta"] = val
        end,
        [5] = function(val) -- options: uint32
          local T = type(val)
          if T ~= "number" then
            error("field options: expected number, but got "..T)
          end
          ret["options"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["account_ref"] = true,
      ["policy_ref"] = true,
      ["relative_to"] = true,
      ["delta"] = true,
      ["options"] = true,
    },
  },

  ["go.chromium.org.luci.server.quota.quotapb.UpdateAccountsInput"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["request_key"] -- 1: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field request_key: expected string, but got "..T)
        end
        acc[1] = val
      end

      val = obj["request_key_ttl"] -- 2: google.protobuf.Duration
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field request_key_ttl: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field request_key_ttl: missing type")
        end
        if val["$type"] ~= "google.protobuf.Duration" then
          error("field request_key_ttl: expected message type 'google.protobuf.Duration', but got "..val["$type"])
        end
        val = PB.M["google.protobuf.Duration"].marshal(val)
        acc[2] = val
      end

      val = obj["hash_scheme"] -- 3: uint32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field hash_scheme: expected number, but got "..T)
        end
        if val < 0 then
          error("field hash_scheme: negative")
        end
        if val > 4294967295 then
          error("field hash_scheme: overflows max uint32")
        end
        acc[3] = val
      end

      val = obj["hash"] -- 4: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field hash: expected string, but got "..T)
        end
        acc[4] = val
      end

      val = obj["ops"] -- 5: repeated go.chromium.org.luci.server.quota.quotapb.RawOp
      if next(val) ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field ops: expected list[message], but got "..T)
        end
        local maxIdx = 0
        local length = 0
        for k, v in next, val do
          if type(k) ~= "number" then
            error("field ops: expected list[message], but got table")
          end
          local T = type(v)
          if T ~= "table" then
            error("field ops["..(i-1).."]: expected table, but got "..T)
          end
          if not v["$type"] then
            error("field ops["..(i-1).."]: missing type")
          end
          if v["$type"] ~= "go.chromium.org.luci.server.quota.quotapb.RawOp" then
            error("field ops["..(i-1).."]: expected message type 'go.chromium.org.luci.server.quota.quotapb.RawOp', but got "..v["$type"])
          end
          val[i] = PB.M["go.chromium.org.luci.server.quota.quotapb.RawOp"].marshal(v)
          if k > maxIdx then
            maxIdx = k
          end
          length = length + 1
        end
        if length ~= maxIdx then
          error("field ops: expected list[message], but got table")
        end
        acc[5] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "go.chromium.org.luci.server.quota.quotapb.UpdateAccountsInput",
        ["request_key"] = "",
        ["request_key_ttl"] = nil,
        ["hash_scheme"] = 0,
        ["hash"] = "",
        ["ops"] = {},
      }
      local dec = {
        [1] = function(val) -- request_key: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field request_key: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field request_key: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field request_key: expected string, but got "..T)
          end
          ret["request_key"] = val
        end,
        [2] = function(val) -- request_key_ttl: google.protobuf.Duration
          local T = type(val)
          if T ~= "table" then
            error("field request_key_ttl: expected table, but got "..T)
          end
          ret["request_key_ttl"] = PB.M["google.protobuf.Duration"].unmarshal(val)
        end,
        [3] = function(val) -- hash_scheme: uint32
          local T = type(val)
          if T ~= "number" then
            error("field hash_scheme: expected number, but got "..T)
          end
          ret["hash_scheme"] = val
        end,
        [4] = function(val) -- hash: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field hash: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field hash: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field hash: expected string, but got "..T)
          end
          ret["hash"] = val
        end,
        [5] = function(val) -- ops: repeated go.chromium.org.luci.server.quota.quotapb.RawOp
          local T = type(val)
          if T ~= "table" then
            error("field ops: expected list[message], but got "..T)
          end
          local max = 0
          local count = 0
          for i, v in next, val do
            if type(i) ~= "number" then
              error("field ops: expected list[message], but got table")
            end
            if i > max then
              max = i
            end
            count = count + 1
            local T = type(v)
            if T ~= "table" then
              error("field ops["..(i-1).."]: expected table, but got "..T)
            end
            val[i] = PB.M["go.chromium.org.luci.server.quota.quotapb.RawOp"].unmarshal(v)
          end
          if max ~= count then
            error("field ops: expected list[message], but got table")
          end
          ret["ops"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["request_key"] = true,
      ["request_key_ttl"] = true,
      ["hash_scheme"] = true,
      ["hash"] = true,
      ["ops"] = true,
    },
  },

  ["google.protobuf.Duration"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["seconds"] -- 1: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field seconds: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field seconds: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field seconds: underflows lua min integer")
        end
        acc[1] = val
      end

      val = obj["nanos"] -- 2: int32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field nanos: expected number, but got "..T)
        end
        if val > 2147483647 then
          error("field nanos: overflows int32")
        end
        if val < -2147483648 then
          error("field nanos: underflows int32")
        end
        acc[2] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "google.protobuf.Duration",
        ["seconds"] = 0,
        ["nanos"] = 0,
      }
      local dec = {
        [1] = function(val) -- seconds: int64
          local T = type(val)
          if T ~= "number" then
            error("field seconds: expected number, but got "..T)
          end
          ret["seconds"] = val
        end,
        [2] = function(val) -- nanos: int32
          local T = type(val)
          if T ~= "number" then
            error("field nanos: expected number, but got "..T)
          end
          ret["nanos"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["seconds"] = true,
      ["nanos"] = true,
    },
  },

  ["google.protobuf.Timestamp"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["seconds"] -- 1: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field seconds: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field seconds: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field seconds: underflows lua min integer")
        end
        acc[1] = val
      end

      val = obj["nanos"] -- 2: int32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field nanos: expected number, but got "..T)
        end
        if val > 2147483647 then
          error("field nanos: overflows int32")
        end
        if val < -2147483648 then
          error("field nanos: underflows int32")
        end
        acc[2] = val
      end

      local unknown = obj["$unknown"]
      if unknown ~= nil then
        for k, v in next, unknown do acc[k] = v end
      end
      return acc
    end,

    unmarshal = function(raw)
      local defaults = {}
      local ret =  {
        ["$unknown"] = {},
        ["$type"] = "google.protobuf.Timestamp",
        ["seconds"] = 0,
        ["nanos"] = 0,
      }
      local dec = {
        [1] = function(val) -- seconds: int64
          local T = type(val)
          if T ~= "number" then
            error("field seconds: expected number, but got "..T)
          end
          ret["seconds"] = val
        end,
        [2] = function(val) -- nanos: int32
          local T = type(val)
          if T ~= "number" then
            error("field nanos: expected number, but got "..T)
          end
          ret["nanos"] = val
        end,
      }
      for k, v in next, raw do
        local fn = dec[k]
        if fn ~= nil then
          fn(v)
        else
          ret["$unknown"][k] = v
        end
      end
      return ret
    end,
    keys = {
      ["seconds"] = true,
      ["nanos"] = true,
    },
  },
}

if cmsgpack == nil then
  local cmsgpack = ...
  assert(cmsgpack)
end
local cmsgpack_pack = cmsgpack.pack
local cmsgpack_unpack = cmsgpack.unpack
function PB.setInternTable(t)
  if PB.internUnmarshalTable then
    error("cannot set intern table twice")
  end
  if type(t) ~= "table" then
    error("must call PB.setInternTable with a table (got "..type(t)..")")
  end
  PB.internUnmarshalTable = {}
  for i, v in ipairs(t) do
    PB.internUnmarshalTable[i-1] = v
  end
end
function PB.marshal(obj)
  local T = obj["$type"]
  if T == nil then
    error("obj is missing '$type' field")
  end
  local codec = PB.M[T]
  if codec == nil then
    error("unknown proto message type: "..T)
  end
  return cmsgpack_pack(codec.marshal(obj))
end
function PB.unmarshal(messageName, msgpackpb)
  local codec = PB.M[messageName]
  if codec == nil then
    error("unknown proto message type: "..messageName)
  end
  return codec.unmarshal(cmsgpack_unpack(msgpackpb))
end
function PB.new(messageName, defaults)
  local codec = PB.M[messageName]
  if codec == nil then
    error("unknown proto message type: "..messageName)
  end
  local ret = codec.unmarshal({})
  if defaults ~= nil then
    for k, v in next, defaults do
      if k[0] ~= "$" then
        if not codec.keys[k] then
          error("invalid property name: "..k)
        end
        ret[k] = v
      end
    end
  end
  return ret
end
return PB
