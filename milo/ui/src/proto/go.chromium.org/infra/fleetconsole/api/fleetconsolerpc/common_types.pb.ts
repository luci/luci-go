// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";

export const protobufPackage = "fleetconsole";

export interface DateOnly {
  readonly year: number;
  readonly month: number;
  readonly day: number;
}

function createBaseDateOnly(): DateOnly {
  return { year: 0, month: 0, day: 0 };
}

export const DateOnly: MessageFns<DateOnly> = {
  encode(message: DateOnly, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.year !== 0) {
      writer.uint32(8).int32(message.year);
    }
    if (message.month !== 0) {
      writer.uint32(16).int32(message.month);
    }
    if (message.day !== 0) {
      writer.uint32(24).int32(message.day);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): DateOnly {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDateOnly() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 8) {
            break;
          }

          message.year = reader.int32();
          continue;
        }
        case 2: {
          if (tag !== 16) {
            break;
          }

          message.month = reader.int32();
          continue;
        }
        case 3: {
          if (tag !== 24) {
            break;
          }

          message.day = reader.int32();
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): DateOnly {
    return {
      year: isSet(object.year) ? globalThis.Number(object.year) : 0,
      month: isSet(object.month) ? globalThis.Number(object.month) : 0,
      day: isSet(object.day) ? globalThis.Number(object.day) : 0,
    };
  },

  toJSON(message: DateOnly): unknown {
    const obj: any = {};
    if (message.year !== 0) {
      obj.year = Math.round(message.year);
    }
    if (message.month !== 0) {
      obj.month = Math.round(message.month);
    }
    if (message.day !== 0) {
      obj.day = Math.round(message.day);
    }
    return obj;
  },

  create(base?: DeepPartial<DateOnly>): DateOnly {
    return DateOnly.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<DateOnly>): DateOnly {
    const message = createBaseDateOnly() as any;
    message.year = object.year ?? 0;
    message.month = object.month ?? 0;
    message.day = object.day ?? 0;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}

export interface MessageFns<T> {
  encode(message: T, writer?: BinaryWriter): BinaryWriter;
  decode(input: BinaryReader | Uint8Array, length?: number): T;
  fromJSON(object: any): T;
  toJSON(message: T): unknown;
  create(base?: DeepPartial<T>): T;
  fromPartial(object: DeepPartial<T>): T;
}
