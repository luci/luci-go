// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v1.176.0
//   protoc               v5.26.1
// source: infra/libs/skylab/inventory/common.proto

/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";

export const protobufPackage = "chrome.chromeos_infra.skylab.proto.inventory";

/**
 * Copyright 2018 The Chromium Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

/** NEXT TAG: 4 */
export enum Environment {
  ENVIRONMENT_INVALID = 0,
  ENVIRONMENT_PROD = 1,
  ENVIRONMENT_STAGING = 2,
  /** @deprecated */
  ENVIRONMENT_SKYLAB = 3,
}

export function environmentFromJSON(object: any): Environment {
  switch (object) {
    case 0:
    case "ENVIRONMENT_INVALID":
      return Environment.ENVIRONMENT_INVALID;
    case 1:
    case "ENVIRONMENT_PROD":
      return Environment.ENVIRONMENT_PROD;
    case 2:
    case "ENVIRONMENT_STAGING":
      return Environment.ENVIRONMENT_STAGING;
    case 3:
    case "ENVIRONMENT_SKYLAB":
      return Environment.ENVIRONMENT_SKYLAB;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Environment");
  }
}

export function environmentToJSON(object: Environment): string {
  switch (object) {
    case Environment.ENVIRONMENT_INVALID:
      return "ENVIRONMENT_INVALID";
    case Environment.ENVIRONMENT_PROD:
      return "ENVIRONMENT_PROD";
    case Environment.ENVIRONMENT_STAGING:
      return "ENVIRONMENT_STAGING";
    case Environment.ENVIRONMENT_SKYLAB:
      return "ENVIRONMENT_SKYLAB";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Environment");
  }
}

export interface Timestamp {
  readonly seconds?: string | undefined;
  readonly nanos?: number | undefined;
}

function createBaseTimestamp(): Timestamp {
  return { seconds: "0", nanos: 0 };
}

export const Timestamp = {
  encode(message: Timestamp, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.seconds !== undefined && message.seconds !== "0") {
      writer.uint32(8).int64(message.seconds);
    }
    if (message.nanos !== undefined && message.nanos !== 0) {
      writer.uint32(16).int32(message.nanos);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Timestamp {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTimestamp() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.seconds = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.nanos = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Timestamp {
    return {
      seconds: isSet(object.seconds) ? globalThis.String(object.seconds) : "0",
      nanos: isSet(object.nanos) ? globalThis.Number(object.nanos) : 0,
    };
  },

  toJSON(message: Timestamp): unknown {
    const obj: any = {};
    if (message.seconds !== undefined && message.seconds !== "0") {
      obj.seconds = message.seconds;
    }
    if (message.nanos !== undefined && message.nanos !== 0) {
      obj.nanos = Math.round(message.nanos);
    }
    return obj;
  },

  create(base?: DeepPartial<Timestamp>): Timestamp {
    return Timestamp.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<Timestamp>): Timestamp {
    const message = createBaseTimestamp() as any;
    message.seconds = object.seconds ?? "0";
    message.nanos = object.nanos ?? 0;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

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