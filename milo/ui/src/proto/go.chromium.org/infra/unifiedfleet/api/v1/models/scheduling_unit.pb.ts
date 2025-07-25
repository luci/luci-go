// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/infra/unifiedfleet/api/v1/models/scheduling_unit.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";
import { Timestamp } from "../../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "unifiedfleet.api.v1.models";

export enum SchedulingUnitType {
  SCHEDULING_UNIT_TYPE_INVALID = 0,
  /** SCHEDULING_UNIT_TYPE_ALL - which means the SchedulingUnit only considers as ready when all of the associated DUT's/MachineLSE's resourceState is ready. */
  SCHEDULING_UNIT_TYPE_ALL = 1,
  /** SCHEDULING_UNIT_TYPE_INDIVIDUAL - which means the SchedulingUnit is considered as ready if at least one of the associated DUT's/MachineLSE's resourceState is ready. */
  SCHEDULING_UNIT_TYPE_INDIVIDUAL = 2,
}

export function schedulingUnitTypeFromJSON(object: any): SchedulingUnitType {
  switch (object) {
    case 0:
    case "SCHEDULING_UNIT_TYPE_INVALID":
      return SchedulingUnitType.SCHEDULING_UNIT_TYPE_INVALID;
    case 1:
    case "SCHEDULING_UNIT_TYPE_ALL":
      return SchedulingUnitType.SCHEDULING_UNIT_TYPE_ALL;
    case 2:
    case "SCHEDULING_UNIT_TYPE_INDIVIDUAL":
      return SchedulingUnitType.SCHEDULING_UNIT_TYPE_INDIVIDUAL;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum SchedulingUnitType");
  }
}

export function schedulingUnitTypeToJSON(object: SchedulingUnitType): string {
  switch (object) {
    case SchedulingUnitType.SCHEDULING_UNIT_TYPE_INVALID:
      return "SCHEDULING_UNIT_TYPE_INVALID";
    case SchedulingUnitType.SCHEDULING_UNIT_TYPE_ALL:
      return "SCHEDULING_UNIT_TYPE_ALL";
    case SchedulingUnitType.SCHEDULING_UNIT_TYPE_INDIVIDUAL:
      return "SCHEDULING_UNIT_TYPE_INDIVIDUAL";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum SchedulingUnitType");
  }
}

/**
 * SchedulingUnit is used for supporting multi-DUT setup in ChromeOS lab.
 *
 * A SchedulingUnit will have one or more DUTs associated with it.
 */
export interface SchedulingUnit {
  readonly name: string;
  /** name of DUT/MachineLSE */
  readonly machineLSEs: readonly string[];
  /** swarming pools to which this SchedulingUnit belongs to. */
  readonly pools: readonly string[];
  /** indicate how dut_state dimension of a scheduling unit should be calculated. */
  readonly type: SchedulingUnitType;
  /** description of the SchedulingUnit. */
  readonly description: string;
  /** record the last update timestamp of this SchedulingUnit (In UTC timezone) */
  readonly updateTime:
    | string
    | undefined;
  /** tags user can attach for easy querying/searching */
  readonly tags: readonly string[];
  /** hostname of designated primary dut. primary dut is optional. */
  readonly primaryDut: string;
  /** expose type of scheduling unit labels. */
  readonly exposeType: SchedulingUnit_ExposeType;
  /** Indicates if the scheduling unit is entirely hosted in a wifecell box. */
  readonly wificell: boolean;
  /** Indicates the scheduling unit is in a cellular testbed with a specific carrier. */
  readonly carrier: string;
}

/** ExposeType determines label dimensions for a scheduling unit */
export enum SchedulingUnit_ExposeType {
  UNKNOWN = 0,
  /** DEFAULT - default expose board and model of all duts and labels that are intersection of all duts. */
  DEFAULT = 1,
  /** DEFAULT_PLUS_PRIMARY - default_plus_primary expose board and model of all duts plus all other labels of primary dut. */
  DEFAULT_PLUS_PRIMARY = 2,
  /** STRICTLY_PRIMARY_ONLY - default_primary_only expose all labels of primary dut execpt for dut_name. */
  STRICTLY_PRIMARY_ONLY = 3,
}

export function schedulingUnit_ExposeTypeFromJSON(object: any): SchedulingUnit_ExposeType {
  switch (object) {
    case 0:
    case "UNKNOWN":
      return SchedulingUnit_ExposeType.UNKNOWN;
    case 1:
    case "DEFAULT":
      return SchedulingUnit_ExposeType.DEFAULT;
    case 2:
    case "DEFAULT_PLUS_PRIMARY":
      return SchedulingUnit_ExposeType.DEFAULT_PLUS_PRIMARY;
    case 3:
    case "STRICTLY_PRIMARY_ONLY":
      return SchedulingUnit_ExposeType.STRICTLY_PRIMARY_ONLY;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum SchedulingUnit_ExposeType");
  }
}

export function schedulingUnit_ExposeTypeToJSON(object: SchedulingUnit_ExposeType): string {
  switch (object) {
    case SchedulingUnit_ExposeType.UNKNOWN:
      return "UNKNOWN";
    case SchedulingUnit_ExposeType.DEFAULT:
      return "DEFAULT";
    case SchedulingUnit_ExposeType.DEFAULT_PLUS_PRIMARY:
      return "DEFAULT_PLUS_PRIMARY";
    case SchedulingUnit_ExposeType.STRICTLY_PRIMARY_ONLY:
      return "STRICTLY_PRIMARY_ONLY";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum SchedulingUnit_ExposeType");
  }
}

function createBaseSchedulingUnit(): SchedulingUnit {
  return {
    name: "",
    machineLSEs: [],
    pools: [],
    type: 0,
    description: "",
    updateTime: undefined,
    tags: [],
    primaryDut: "",
    exposeType: 0,
    wificell: false,
    carrier: "",
  };
}

export const SchedulingUnit: MessageFns<SchedulingUnit> = {
  encode(message: SchedulingUnit, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    for (const v of message.machineLSEs) {
      writer.uint32(18).string(v!);
    }
    for (const v of message.pools) {
      writer.uint32(26).string(v!);
    }
    if (message.type !== 0) {
      writer.uint32(32).int32(message.type);
    }
    if (message.description !== "") {
      writer.uint32(42).string(message.description);
    }
    if (message.updateTime !== undefined) {
      Timestamp.encode(toTimestamp(message.updateTime), writer.uint32(50).fork()).join();
    }
    for (const v of message.tags) {
      writer.uint32(58).string(v!);
    }
    if (message.primaryDut !== "") {
      writer.uint32(66).string(message.primaryDut);
    }
    if (message.exposeType !== 0) {
      writer.uint32(72).int32(message.exposeType);
    }
    if (message.wificell !== false) {
      writer.uint32(80).bool(message.wificell);
    }
    if (message.carrier !== "") {
      writer.uint32(90).string(message.carrier);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): SchedulingUnit {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSchedulingUnit() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.machineLSEs.push(reader.string());
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.pools.push(reader.string());
          continue;
        }
        case 4: {
          if (tag !== 32) {
            break;
          }

          message.type = reader.int32() as any;
          continue;
        }
        case 5: {
          if (tag !== 42) {
            break;
          }

          message.description = reader.string();
          continue;
        }
        case 6: {
          if (tag !== 50) {
            break;
          }

          message.updateTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        }
        case 7: {
          if (tag !== 58) {
            break;
          }

          message.tags.push(reader.string());
          continue;
        }
        case 8: {
          if (tag !== 66) {
            break;
          }

          message.primaryDut = reader.string();
          continue;
        }
        case 9: {
          if (tag !== 72) {
            break;
          }

          message.exposeType = reader.int32() as any;
          continue;
        }
        case 10: {
          if (tag !== 80) {
            break;
          }

          message.wificell = reader.bool();
          continue;
        }
        case 11: {
          if (tag !== 90) {
            break;
          }

          message.carrier = reader.string();
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

  fromJSON(object: any): SchedulingUnit {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      machineLSEs: globalThis.Array.isArray(object?.machineLSEs)
        ? object.machineLSEs.map((e: any) => globalThis.String(e))
        : [],
      pools: globalThis.Array.isArray(object?.pools) ? object.pools.map((e: any) => globalThis.String(e)) : [],
      type: isSet(object.type) ? schedulingUnitTypeFromJSON(object.type) : 0,
      description: isSet(object.description) ? globalThis.String(object.description) : "",
      updateTime: isSet(object.updateTime) ? globalThis.String(object.updateTime) : undefined,
      tags: globalThis.Array.isArray(object?.tags) ? object.tags.map((e: any) => globalThis.String(e)) : [],
      primaryDut: isSet(object.primaryDut) ? globalThis.String(object.primaryDut) : "",
      exposeType: isSet(object.exposeType) ? schedulingUnit_ExposeTypeFromJSON(object.exposeType) : 0,
      wificell: isSet(object.wificell) ? globalThis.Boolean(object.wificell) : false,
      carrier: isSet(object.carrier) ? globalThis.String(object.carrier) : "",
    };
  },

  toJSON(message: SchedulingUnit): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.machineLSEs?.length) {
      obj.machineLSEs = message.machineLSEs;
    }
    if (message.pools?.length) {
      obj.pools = message.pools;
    }
    if (message.type !== 0) {
      obj.type = schedulingUnitTypeToJSON(message.type);
    }
    if (message.description !== "") {
      obj.description = message.description;
    }
    if (message.updateTime !== undefined) {
      obj.updateTime = message.updateTime;
    }
    if (message.tags?.length) {
      obj.tags = message.tags;
    }
    if (message.primaryDut !== "") {
      obj.primaryDut = message.primaryDut;
    }
    if (message.exposeType !== 0) {
      obj.exposeType = schedulingUnit_ExposeTypeToJSON(message.exposeType);
    }
    if (message.wificell !== false) {
      obj.wificell = message.wificell;
    }
    if (message.carrier !== "") {
      obj.carrier = message.carrier;
    }
    return obj;
  },

  create(base?: DeepPartial<SchedulingUnit>): SchedulingUnit {
    return SchedulingUnit.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<SchedulingUnit>): SchedulingUnit {
    const message = createBaseSchedulingUnit() as any;
    message.name = object.name ?? "";
    message.machineLSEs = object.machineLSEs?.map((e) => e) || [];
    message.pools = object.pools?.map((e) => e) || [];
    message.type = object.type ?? 0;
    message.description = object.description ?? "";
    message.updateTime = object.updateTime ?? undefined;
    message.tags = object.tags?.map((e) => e) || [];
    message.primaryDut = object.primaryDut ?? "";
    message.exposeType = object.exposeType ?? 0;
    message.wificell = object.wificell ?? false;
    message.carrier = object.carrier ?? "";
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

function toTimestamp(dateStr: string): Timestamp {
  const date = new globalThis.Date(dateStr);
  const seconds = Math.trunc(date.getTime() / 1_000).toString();
  const nanos = (date.getTime() % 1_000) * 1_000_000;
  return { seconds, nanos };
}

function fromTimestamp(t: Timestamp): string {
  let millis = (globalThis.Number(t.seconds) || 0) * 1_000;
  millis += (t.nanos || 0) / 1_000_000;
  return new globalThis.Date(millis).toISOString();
}

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
