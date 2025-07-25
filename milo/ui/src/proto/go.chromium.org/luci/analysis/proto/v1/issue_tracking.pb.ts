// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/luci/analysis/proto/v1/issue_tracking.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";

export const protobufPackage = "luci.analysis.v1";

/**
 * This enum represents the Buganizer priorities.
 * It is equivalent to the one in Buganizer API.
 */
export enum BuganizerPriority {
  /** BUGANIZER_PRIORITY_UNSPECIFIED - Priority unspecified; do not use this value. */
  BUGANIZER_PRIORITY_UNSPECIFIED = 0,
  /** P0 - P0, Highest priority. */
  P0 = 1,
  P1 = 2,
  P2 = 3,
  P3 = 4,
  P4 = 5,
}

export function buganizerPriorityFromJSON(object: any): BuganizerPriority {
  switch (object) {
    case 0:
    case "BUGANIZER_PRIORITY_UNSPECIFIED":
      return BuganizerPriority.BUGANIZER_PRIORITY_UNSPECIFIED;
    case 1:
    case "P0":
      return BuganizerPriority.P0;
    case 2:
    case "P1":
      return BuganizerPriority.P1;
    case 3:
    case "P2":
      return BuganizerPriority.P2;
    case 4:
    case "P3":
      return BuganizerPriority.P3;
    case 5:
    case "P4":
      return BuganizerPriority.P4;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuganizerPriority");
  }
}

export function buganizerPriorityToJSON(object: BuganizerPriority): string {
  switch (object) {
    case BuganizerPriority.BUGANIZER_PRIORITY_UNSPECIFIED:
      return "BUGANIZER_PRIORITY_UNSPECIFIED";
    case BuganizerPriority.P0:
      return "P0";
    case BuganizerPriority.P1:
      return "P1";
    case BuganizerPriority.P2:
      return "P2";
    case BuganizerPriority.P3:
      return "P3";
    case BuganizerPriority.P4:
      return "P4";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuganizerPriority");
  }
}

/**
 * Represents a component in an issue tracker. A component is
 * a container for issues.
 */
export interface BugComponent {
  /** The Google Issue Tracker component. */
  readonly issueTracker?:
    | IssueTrackerComponent
    | undefined;
  /** The monorail component. */
  readonly monorail?: MonorailComponent | undefined;
}

/**
 * A component in Google Issue Tracker, sometimes known as Buganizer,
 * available at https://issuetracker.google.com.
 */
export interface IssueTrackerComponent {
  /** The Google Issue Tracker component ID. */
  readonly componentId: string;
}

/**
 * A component in monorail issue tracker, available at
 * https://bugs.chromium.org.
 */
export interface MonorailComponent {
  /** The monorail project name. */
  readonly project: string;
  /** The monorail component value. E.g. "Blink>Accessibility". */
  readonly value: string;
}

function createBaseBugComponent(): BugComponent {
  return { issueTracker: undefined, monorail: undefined };
}

export const BugComponent: MessageFns<BugComponent> = {
  encode(message: BugComponent, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.issueTracker !== undefined) {
      IssueTrackerComponent.encode(message.issueTracker, writer.uint32(10).fork()).join();
    }
    if (message.monorail !== undefined) {
      MonorailComponent.encode(message.monorail, writer.uint32(18).fork()).join();
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): BugComponent {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugComponent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.issueTracker = IssueTrackerComponent.decode(reader, reader.uint32());
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.monorail = MonorailComponent.decode(reader, reader.uint32());
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

  fromJSON(object: any): BugComponent {
    return {
      issueTracker: isSet(object.issueTracker) ? IssueTrackerComponent.fromJSON(object.issueTracker) : undefined,
      monorail: isSet(object.monorail) ? MonorailComponent.fromJSON(object.monorail) : undefined,
    };
  },

  toJSON(message: BugComponent): unknown {
    const obj: any = {};
    if (message.issueTracker !== undefined) {
      obj.issueTracker = IssueTrackerComponent.toJSON(message.issueTracker);
    }
    if (message.monorail !== undefined) {
      obj.monorail = MonorailComponent.toJSON(message.monorail);
    }
    return obj;
  },

  create(base?: DeepPartial<BugComponent>): BugComponent {
    return BugComponent.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<BugComponent>): BugComponent {
    const message = createBaseBugComponent() as any;
    message.issueTracker = (object.issueTracker !== undefined && object.issueTracker !== null)
      ? IssueTrackerComponent.fromPartial(object.issueTracker)
      : undefined;
    message.monorail = (object.monorail !== undefined && object.monorail !== null)
      ? MonorailComponent.fromPartial(object.monorail)
      : undefined;
    return message;
  },
};

function createBaseIssueTrackerComponent(): IssueTrackerComponent {
  return { componentId: "0" };
}

export const IssueTrackerComponent: MessageFns<IssueTrackerComponent> = {
  encode(message: IssueTrackerComponent, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.componentId !== "0") {
      writer.uint32(8).int64(message.componentId);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): IssueTrackerComponent {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseIssueTrackerComponent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 8) {
            break;
          }

          message.componentId = reader.int64().toString();
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

  fromJSON(object: any): IssueTrackerComponent {
    return { componentId: isSet(object.componentId) ? globalThis.String(object.componentId) : "0" };
  },

  toJSON(message: IssueTrackerComponent): unknown {
    const obj: any = {};
    if (message.componentId !== "0") {
      obj.componentId = message.componentId;
    }
    return obj;
  },

  create(base?: DeepPartial<IssueTrackerComponent>): IssueTrackerComponent {
    return IssueTrackerComponent.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<IssueTrackerComponent>): IssueTrackerComponent {
    const message = createBaseIssueTrackerComponent() as any;
    message.componentId = object.componentId ?? "0";
    return message;
  },
};

function createBaseMonorailComponent(): MonorailComponent {
  return { project: "", value: "" };
}

export const MonorailComponent: MessageFns<MonorailComponent> = {
  encode(message: MonorailComponent, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): MonorailComponent {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMonorailComponent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.value = reader.string();
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

  fromJSON(object: any): MonorailComponent {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: MonorailComponent): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create(base?: DeepPartial<MonorailComponent>): MonorailComponent {
    return MonorailComponent.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<MonorailComponent>): MonorailComponent {
    const message = createBaseMonorailComponent() as any;
    message.project = object.project ?? "";
    message.value = object.value ?? "";
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
