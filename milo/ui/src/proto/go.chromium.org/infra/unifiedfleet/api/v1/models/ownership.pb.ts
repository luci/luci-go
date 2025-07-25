// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/infra/unifiedfleet/api/v1/models/ownership.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";

export const protobufPackage = "unifiedfleet.api.v1.models";

/** Next ID: 9 */
export interface OwnershipData {
  /**
   * resource group to which this bot belongs to - deprecated
   *
   * @deprecated
   */
  readonly resourceGroup: readonly string[];
  /** security level of the bot ex:trusted, untrusted etc */
  readonly securityLevel: string;
  /**
   * security pool to which this bot belongs to - deprecated to support
   * array of pools
   *
   * @deprecated
   */
  readonly poolName: string;
  /** swarming instance to which this bot will communicate */
  readonly swarmingInstance: string;
  /**
   * custom miba realm for this bot - deprecated
   *
   * @deprecated
   */
  readonly mibaRealm: string;
  /** Customer who uses this bot. */
  readonly customer: string;
  /** builders for this bot, if any */
  readonly builders: readonly string[];
  /** security pool(s) to which this bot belongs to */
  readonly pools: readonly string[];
}

function createBaseOwnershipData(): OwnershipData {
  return {
    resourceGroup: [],
    securityLevel: "",
    poolName: "",
    swarmingInstance: "",
    mibaRealm: "",
    customer: "",
    builders: [],
    pools: [],
  };
}

export const OwnershipData: MessageFns<OwnershipData> = {
  encode(message: OwnershipData, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    for (const v of message.resourceGroup) {
      writer.uint32(10).string(v!);
    }
    if (message.securityLevel !== "") {
      writer.uint32(18).string(message.securityLevel);
    }
    if (message.poolName !== "") {
      writer.uint32(26).string(message.poolName);
    }
    if (message.swarmingInstance !== "") {
      writer.uint32(34).string(message.swarmingInstance);
    }
    if (message.mibaRealm !== "") {
      writer.uint32(42).string(message.mibaRealm);
    }
    if (message.customer !== "") {
      writer.uint32(50).string(message.customer);
    }
    for (const v of message.builders) {
      writer.uint32(58).string(v!);
    }
    for (const v of message.pools) {
      writer.uint32(66).string(v!);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): OwnershipData {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseOwnershipData() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.resourceGroup.push(reader.string());
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.securityLevel = reader.string();
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.poolName = reader.string();
          continue;
        }
        case 4: {
          if (tag !== 34) {
            break;
          }

          message.swarmingInstance = reader.string();
          continue;
        }
        case 5: {
          if (tag !== 42) {
            break;
          }

          message.mibaRealm = reader.string();
          continue;
        }
        case 6: {
          if (tag !== 50) {
            break;
          }

          message.customer = reader.string();
          continue;
        }
        case 7: {
          if (tag !== 58) {
            break;
          }

          message.builders.push(reader.string());
          continue;
        }
        case 8: {
          if (tag !== 66) {
            break;
          }

          message.pools.push(reader.string());
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

  fromJSON(object: any): OwnershipData {
    return {
      resourceGroup: globalThis.Array.isArray(object?.resourceGroup)
        ? object.resourceGroup.map((e: any) => globalThis.String(e))
        : [],
      securityLevel: isSet(object.securityLevel) ? globalThis.String(object.securityLevel) : "",
      poolName: isSet(object.poolName) ? globalThis.String(object.poolName) : "",
      swarmingInstance: isSet(object.swarmingInstance) ? globalThis.String(object.swarmingInstance) : "",
      mibaRealm: isSet(object.mibaRealm) ? globalThis.String(object.mibaRealm) : "",
      customer: isSet(object.customer) ? globalThis.String(object.customer) : "",
      builders: globalThis.Array.isArray(object?.builders) ? object.builders.map((e: any) => globalThis.String(e)) : [],
      pools: globalThis.Array.isArray(object?.pools) ? object.pools.map((e: any) => globalThis.String(e)) : [],
    };
  },

  toJSON(message: OwnershipData): unknown {
    const obj: any = {};
    if (message.resourceGroup?.length) {
      obj.resourceGroup = message.resourceGroup;
    }
    if (message.securityLevel !== "") {
      obj.securityLevel = message.securityLevel;
    }
    if (message.poolName !== "") {
      obj.poolName = message.poolName;
    }
    if (message.swarmingInstance !== "") {
      obj.swarmingInstance = message.swarmingInstance;
    }
    if (message.mibaRealm !== "") {
      obj.mibaRealm = message.mibaRealm;
    }
    if (message.customer !== "") {
      obj.customer = message.customer;
    }
    if (message.builders?.length) {
      obj.builders = message.builders;
    }
    if (message.pools?.length) {
      obj.pools = message.pools;
    }
    return obj;
  },

  create(base?: DeepPartial<OwnershipData>): OwnershipData {
    return OwnershipData.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<OwnershipData>): OwnershipData {
    const message = createBaseOwnershipData() as any;
    message.resourceGroup = object.resourceGroup?.map((e) => e) || [];
    message.securityLevel = object.securityLevel ?? "";
    message.poolName = object.poolName ?? "";
    message.swarmingInstance = object.swarmingInstance ?? "";
    message.mibaRealm = object.mibaRealm ?? "";
    message.customer = object.customer ?? "";
    message.builders = object.builders?.map((e) => e) || [];
    message.pools = object.pools?.map((e) => e) || [];
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
