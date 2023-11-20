/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";

export const protobufPackage = "luci.bisection.v1";

/** Information about a bug associated with a failure. */
export interface BugInfo {
  readonly monorailBugInfo?: MonorailBugInfo | undefined;
  readonly buganizerBugInfo?: BuganizerBugInfo | undefined;
}

export interface MonorailBugInfo {
  /** The project of the bug, e.g. "chromium". */
  readonly project: string;
  /** Monorail bug ID. */
  readonly bugId: number;
}

export interface BuganizerBugInfo {
  /** Buganizer bug ID. */
  readonly bugId: string;
}

function createBaseBugInfo(): BugInfo {
  return { monorailBugInfo: undefined, buganizerBugInfo: undefined };
}

export const BugInfo = {
  encode(message: BugInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.monorailBugInfo !== undefined) {
      MonorailBugInfo.encode(message.monorailBugInfo, writer.uint32(10).fork()).ldelim();
    }
    if (message.buganizerBugInfo !== undefined) {
      BuganizerBugInfo.encode(message.buganizerBugInfo, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugInfo() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.monorailBugInfo = MonorailBugInfo.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.buganizerBugInfo = BuganizerBugInfo.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugInfo {
    return {
      monorailBugInfo: isSet(object.monorailBugInfo) ? MonorailBugInfo.fromJSON(object.monorailBugInfo) : undefined,
      buganizerBugInfo: isSet(object.buganizerBugInfo) ? BuganizerBugInfo.fromJSON(object.buganizerBugInfo) : undefined,
    };
  },

  toJSON(message: BugInfo): unknown {
    const obj: any = {};
    if (message.monorailBugInfo !== undefined) {
      obj.monorailBugInfo = MonorailBugInfo.toJSON(message.monorailBugInfo);
    }
    if (message.buganizerBugInfo !== undefined) {
      obj.buganizerBugInfo = BuganizerBugInfo.toJSON(message.buganizerBugInfo);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugInfo>, I>>(base?: I): BugInfo {
    return BugInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugInfo>, I>>(object: I): BugInfo {
    const message = createBaseBugInfo() as any;
    message.monorailBugInfo = (object.monorailBugInfo !== undefined && object.monorailBugInfo !== null)
      ? MonorailBugInfo.fromPartial(object.monorailBugInfo)
      : undefined;
    message.buganizerBugInfo = (object.buganizerBugInfo !== undefined && object.buganizerBugInfo !== null)
      ? BuganizerBugInfo.fromPartial(object.buganizerBugInfo)
      : undefined;
    return message;
  },
};

function createBaseMonorailBugInfo(): MonorailBugInfo {
  return { project: "", bugId: 0 };
}

export const MonorailBugInfo = {
  encode(message: MonorailBugInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.bugId !== 0) {
      writer.uint32(16).int32(message.bugId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MonorailBugInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMonorailBugInfo() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.bugId = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): MonorailBugInfo {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      bugId: isSet(object.bugId) ? globalThis.Number(object.bugId) : 0,
    };
  },

  toJSON(message: MonorailBugInfo): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.bugId !== 0) {
      obj.bugId = Math.round(message.bugId);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<MonorailBugInfo>, I>>(base?: I): MonorailBugInfo {
    return MonorailBugInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<MonorailBugInfo>, I>>(object: I): MonorailBugInfo {
    const message = createBaseMonorailBugInfo() as any;
    message.project = object.project ?? "";
    message.bugId = object.bugId ?? 0;
    return message;
  },
};

function createBaseBuganizerBugInfo(): BuganizerBugInfo {
  return { bugId: "0" };
}

export const BuganizerBugInfo = {
  encode(message: BuganizerBugInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bugId !== "0") {
      writer.uint32(8).int64(message.bugId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuganizerBugInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuganizerBugInfo() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.bugId = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuganizerBugInfo {
    return { bugId: isSet(object.bugId) ? globalThis.String(object.bugId) : "0" };
  },

  toJSON(message: BuganizerBugInfo): unknown {
    const obj: any = {};
    if (message.bugId !== "0") {
      obj.bugId = message.bugId;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuganizerBugInfo>, I>>(base?: I): BuganizerBugInfo {
    return BuganizerBugInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuganizerBugInfo>, I>>(object: I): BuganizerBugInfo {
    const message = createBaseBuganizerBugInfo() as any;
    message.bugId = object.bugId ?? "0";
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

function longToString(long: Long) {
  return long.toString();
}

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
