local PB = {}

local next = next
local type = type

PB.E = {
  ["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.VALUE"] = {
    ["ZERO"] = 0,
    [0] = "ZERO",
    ["ONE"] = 1,
    [1] = "ONE",
    ["TWO"] = 2,
    [2] = "TWO",
  },
}

PB.M = {
  ["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"] = {
    marshal = function(obj)
      local acc, val, T = {}, nil, nil, nil

      val = obj["boolval"] -- 2: bool
      if val ~= false then
        local T = type(val)
        if T ~= "boolean" then
          error("field boolval: expected boolean, but got "..T)
        end
        acc[2] = val
      end

      val = obj["intval"] -- 3: int64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field intval: expected number, but got "..T)
        end
        if val > 9007199254740991 then
          error("field intval: overflows lua max integer")
        end
        if val < -9007199254740991 then
          error("field intval: underflows lua min integer")
        end
        acc[3] = val
      end

      val = obj["uintval"] -- 4: uint64
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field uintval: expected number, but got "..T)
        end
        if val < 0 then
          error("field uintval: negative")
        end
        if val > 9007199254740991 then
          error("field uintval: overflows lua max integer")
        end
        acc[4] = val
      end

      val = obj["short_intval"] -- 5: int32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field short_intval: expected number, but got "..T)
        end
        if val > 2147483647 then
          error("field short_intval: overflows int32")
        end
        if val < -2147483648 then
          error("field short_intval: underflows int32")
        end
        acc[5] = val
      end

      val = obj["short_uintval"] -- 6: uint32
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field short_uintval: expected number, but got "..T)
        end
        if val < 0 then
          error("field short_uintval: negative")
        end
        if val > 4294967295 then
          error("field short_uintval: overflows max uint32")
        end
        acc[6] = val
      end

      val = obj["strval"] -- 7: string
      if val ~= "" then
        local T = type(val)
        if T ~= "string" then
          error("field strval: expected string, but got "..T)
        end
        acc[7] = val
      end

      val = obj["floatval"] -- 8: double
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field floatval: expected number, but got "..T)
        end
        acc[8] = val
      end

      val = obj["short_floatval"] -- 9: float
      if val ~= 0 then
        local T = type(val)
        if T ~= "number" then
          error("field short_floatval: expected number, but got "..T)
        end
        acc[9] = val
      end

      val = obj["value"] -- 10: enum go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.VALUE
      if val ~= 0 and val ~= "ZERO" then
        local T = type(val)
        local origval = val
        if T == "string" then
          val = PB.E["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.VALUE"][val]
          if val == nil then
            error("field value: bad string enum value "..origval)
          end
        elseif T == "number" then
          if PB.E["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.VALUE"][val] == nil then
            error("field value: bad numeric enum value "..origval)
          end
        else
          error("field value: expected number or string, but got "..T)
        end
        acc[10] = val
      end

      val = obj["mapfield"] -- 11: map<string, go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage>
      if next(val) ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field mapfield: expected map<string, message>, but got "..T)
        end
        local i = 0
        for k, v in next, val do
          local T = type(k)
          if T ~= "string" then
            error("field mapfield["..i.."th entry]: expected string, but got "..T)
          end
          local T = type(v)
          if T ~= "table" then
            error("field mapfield["..k.."]: expected table, but got "..T)
          end
          if not v["$type"] then
            error("field mapfield["..k.."]: missing type")
          end
          if v["$type"] ~= "go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage" then
            error("field mapfield["..k.."]: expected message type 'go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage', but got "..v["$type"])
          end
          val[k] = PB.M["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"].marshal(v)
          i = i + 1
        end
        acc[11] = val
      end

      val = obj["duration"] -- 12: google.protobuf.Duration
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field duration: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field duration: missing type")
        end
        if val["$type"] ~= "google.protobuf.Duration" then
          error("field duration: expected message type 'google.protobuf.Duration', but got "..val["$type"])
        end
        val = PB.M["google.protobuf.Duration"].marshal(val)
        acc[12] = val
      end

      val = obj["strings"] -- 13: repeated string
      if next(val) ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field strings: expected list[string], but got "..T)
        end
        local maxIdx = 0
        local length = 0
        for i, v in next, val do
          if type(i) ~= "number" then
            error("field strings: expected list[string], but got table")
          end
          local T = type(v)
          if T ~= "string" then
            error("field strings["..(i-1).."]: expected string, but got "..T)
          end
          if i > maxIdx then
            maxIdx = i
          end
          length = length + 1
        end
        if length ~= maxIdx then
          error("field strings: expected list[string], but got table")
        end
        acc[13] = val
      end

      val = obj["single_recurse"] -- 14: go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage
      if val ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field single_recurse: expected table, but got "..T)
        end
        if not val["$type"] then
          error("field single_recurse: missing type")
        end
        if val["$type"] ~= "go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage" then
          error("field single_recurse: expected message type 'go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage', but got "..val["$type"])
        end
        val = PB.M["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"].marshal(val)
        acc[14] = val
      end

      val = obj["multi_recursion"] -- 15: repeated go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage
      if next(val) ~= nil then
        local T = type(val)
        if T ~= "table" then
          error("field multi_recursion: expected list[message], but got "..T)
        end
        local maxIdx = 0
        local length = 0
        for i, v in next, val do
          if type(i) ~= "number" then
            error("field multi_recursion: expected list[message], but got table")
          end
          local T = type(v)
          if T ~= "table" then
            error("field multi_recursion["..(i-1).."]: expected table, but got "..T)
          end
          if not v["$type"] then
            error("field multi_recursion["..(i-1).."]: missing type")
          end
          if v["$type"] ~= "go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage" then
            error("field multi_recursion["..(i-1).."]: expected message type 'go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage', but got "..v["$type"])
          end
          val[i] = PB.M["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"].marshal(v)
          if i > maxIdx then
            maxIdx = i
          end
          length = length + 1
        end
        if length ~= maxIdx then
          error("field multi_recursion: expected list[message], but got table")
        end
        acc[15] = val
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
        ["$type"] = "go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage",
        ["boolval"] = false,
        ["intval"] = 0,
        ["uintval"] = 0,
        ["short_intval"] = 0,
        ["short_uintval"] = 0,
        ["strval"] = "",
        ["floatval"] = 0,
        ["short_floatval"] = 0,
        ["value"] = "ZERO",
        ["mapfield"] = {},
        ["duration"] = nil,
        ["strings"] = {},
        ["single_recurse"] = nil,
        ["multi_recursion"] = {},
      }
      local dec = {
        [2] = function(val) -- boolval: bool
          local T = type(val)
          if T ~= "boolean" then
            error("field boolval: expected boolean, but got "..T)
          end
          ret["boolval"] = val
        end,
        [3] = function(val) -- intval: int64
          local T = type(val)
          if T ~= "number" then
            error("field intval: expected number, but got "..T)
          end
          ret["intval"] = val
        end,
        [4] = function(val) -- uintval: uint64
          local T = type(val)
          if T ~= "number" then
            error("field uintval: expected number, but got "..T)
          end
          ret["uintval"] = val
        end,
        [5] = function(val) -- short_intval: int32
          local T = type(val)
          if T ~= "number" then
            error("field short_intval: expected number, but got "..T)
          end
          ret["short_intval"] = val
        end,
        [6] = function(val) -- short_uintval: uint32
          local T = type(val)
          if T ~= "number" then
            error("field short_uintval: expected number, but got "..T)
          end
          ret["short_uintval"] = val
        end,
        [7] = function(val) -- strval: string
          local T = type(val)
          if T == "number" then
            if not PB.internUnmarshalTable then
              error("field strval: failed to look up interned string: intern table not set")
            end
            local origval = val
            local newval = PB.internUnmarshalTable[val]
            if newval == nil then
              error("field strval: failed to look up interned string: "..origval)
            end
            val = newval
            T = type(val)
          end
          if T ~= "string" then
            error("field strval: expected string, but got "..T)
          end
          ret["strval"] = val
        end,
        [8] = function(val) -- floatval: double
          local T = type(val)
          if T ~= "number" then
            error("field floatval: expected number, but got "..T)
          end
          ret["floatval"] = val
        end,
        [9] = function(val) -- short_floatval: float
          local T = type(val)
          if T ~= "number" then
            error("field short_floatval: expected number, but got "..T)
          end
          ret["short_floatval"] = val
        end,
        [10] = function(val) -- value: enum go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.VALUE
          local T = type(val)
          if T ~= "number" then
            error("field value: expected numeric enum, but got "..T)
          end
          local origval = val
          local newval = PB.E["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.VALUE"][val]
          if newval == nil then
            error("field value: bad enum value "..origval)
          end
          ret["value"] = newval
        end,
        [11] = function(val) -- mapfield: map<string, go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage>
          local T = type(val)
          if T ~= "table" then
            error("field mapfield: expected map<string, message>, but got "..T)
          end
          local i = 0
          for k, v in next, val do
            local T = type(k)
            if T == "number" then
              if not PB.internUnmarshalTable then
                error("field mapfield["..i.."th entry]: failed to look up interned string: intern table not set")
              end
              local origval = k
              local newval = PB.internUnmarshalTable[k]
              if newval == nil then
                error("field mapfield["..i.."th entry]: failed to look up interned string: "..origval)
              end
              val = newval
              T = type(val)
            end
            if T ~= "string" then
              error("field mapfield["..i.."th entry]: expected string, but got "..T)
            end
            k = val
            local T = type(v)
            if T ~= "table" then
              error("field mapfield["..k.."]: expected table, but got "..T)
            end
            val[k] = PB.M["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"].unmarshal(v)
            i = i + 1
          end
          ret["mapfield"] = val
        end,
        [12] = function(val) -- duration: google.protobuf.Duration
          local T = type(val)
          if T ~= "table" then
            error("field duration: expected table, but got "..T)
          end
          ret["duration"] = PB.M["google.protobuf.Duration"].unmarshal(val)
        end,
        [13] = function(val) -- strings: repeated string
          local T = type(val)
          if T ~= "table" then
            error("field strings: expected list[string], but got "..T)
          end
          local max = 0
          local count = 0
          for i, v in next, val do
            if type(i) ~= "number" then
              error("field strings: expected list[string], but got table")
            end
            if i > max then
              max = i
            end
            count = count + 1
            local T = type(v)
            if T == "number" then
              if not PB.internUnmarshalTable then
                error("field strings["..(i-1).."]: failed to look up interned string: intern table not set")
              end
              local origval = v
              local newval = PB.internUnmarshalTable[v]
              if newval == nil then
                error("field strings["..(i-1).."]: failed to look up interned string: "..origval)
              end
              val = newval
              T = type(val)
            end
            if T ~= "string" then
              error("field strings["..(i-1).."]: expected string, but got "..T)
            end
            val[i] = val
          end
          if max ~= count then
            error("field strings: expected list[string], but got table")
          end
          ret["strings"] = val
        end,
        [14] = function(val) -- single_recurse: go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage
          local T = type(val)
          if T ~= "table" then
            error("field single_recurse: expected table, but got "..T)
          end
          ret["single_recurse"] = PB.M["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"].unmarshal(val)
        end,
        [15] = function(val) -- multi_recursion: repeated go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage
          local T = type(val)
          if T ~= "table" then
            error("field multi_recursion: expected list[message], but got "..T)
          end
          local max = 0
          local count = 0
          for i, v in next, val do
            if type(i) ~= "number" then
              error("field multi_recursion: expected list[message], but got table")
            end
            if i > max then
              max = i
            end
            count = count + 1
            local T = type(v)
            if T ~= "table" then
              error("field multi_recursion["..(i-1).."]: expected table, but got "..T)
            end
            val[i] = PB.M["go.chromium.org.luci.common.proto.msgpackpb.luagen.examplepb.TestMessage"].unmarshal(v)
          end
          if max ~= count then
            error("field multi_recursion: expected list[message], but got table")
          end
          ret["multi_recursion"] = val
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
      ["boolval"] = true,
      ["intval"] = true,
      ["uintval"] = true,
      ["short_intval"] = true,
      ["short_uintval"] = true,
      ["strval"] = true,
      ["floatval"] = true,
      ["short_floatval"] = true,
      ["value"] = true,
      ["mapfield"] = true,
      ["duration"] = true,
      ["strings"] = true,
      ["single_recurse"] = true,
      ["multi_recursion"] = true,
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
