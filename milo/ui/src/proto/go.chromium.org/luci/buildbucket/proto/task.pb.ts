/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Struct } from "../../../../google/protobuf/struct.pb";
import { Status, StatusDetails, statusFromJSON, statusToJSON } from "./common.pb";

export const protobufPackage = "buildbucket.v2";

/**
 * A backend task.
 * Next id: 9.
 */
export interface Task {
  readonly id:
    | TaskID
    | undefined;
  /**
   * (optional) Human-clickable link to the status page for this task.
   * This should be populated as part of the Task response in RunTaskResponse.
   * Any update to this via the Task field in BuildTaskUpdate will override the
   * existing link that was provided in RunTaskResponse.
   */
  readonly link: string;
  /** The backend's status for handling this task. */
  readonly status: Status;
  /** The 'status_details' around handling this task. */
  readonly statusDetails:
    | StatusDetails
    | undefined;
  /** Deprecated. Use summary_markdown instead. */
  readonly summaryHtml: string;
  /**
   * Additional backend-specific details about the task.
   *
   * This could be used to indicate things like named-cache status, task
   * startup/end time, etc.
   *
   * This is limited to 10KB (binary PB + gzip(5))
   *
   * This should be populated as part of the Task response in RunTaskResponse.
   * Any update to this via the Task field in BuildTaskUpdate will override the
   * existing details that were provided in RunTaskResponse.
   */
  readonly details:
    | { readonly [key: string]: any }
    | undefined;
  /**
   * A monotonically increasing integer set by the backend to track
   * which task is the most up to date when calling UpdateBuildTask.
   * When the build is first created, this will be set to 0.
   * When RunTask is called and returns a task, this should not be 0 or nil.
   * Each UpdateBuildTask call will check this to ensure the latest task is
   * being stored in datastore.
   */
  readonly updateId: string;
  /** Human-readable commentary around the handling of this task. */
  readonly summaryMarkdown: string;
}

/** A unique identifier for tasks. */
export interface TaskID {
  /** Target backend. e.g. "swarming://chromium-swarm". */
  readonly target: string;
  /**
   * An ID unique to the target used to identify this task. e.g. Swarming task
   * ID.
   */
  readonly id: string;
}

/**
 * A message sent by task backends as part of the payload to a
 * pubsub topic corresponding with that backend. Buildbucket handles these
 * pubsub messages with the UpdateBuildTask cloud task.
 * Backends must use this proto when sending pubsub updates to buildbucket.
 *
 * NOTE: If the task has not been registered with buildbucket yet (by means of
 * RunTask returning or StartBuild doing an initial associaton of the task to
 * the build), then the message will be dropped and lost forever.
 * Use with caution.
 */
export interface BuildTaskUpdate {
  /** A build ID. */
  readonly buildId: string;
  /** Task */
  readonly task: Task | undefined;
}

function createBaseTask(): Task {
  return {
    id: undefined,
    link: "",
    status: 0,
    statusDetails: undefined,
    summaryHtml: "",
    details: undefined,
    updateId: "0",
    summaryMarkdown: "",
  };
}

export const Task = {
  encode(message: Task, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== undefined) {
      TaskID.encode(message.id, writer.uint32(10).fork()).ldelim();
    }
    if (message.link !== "") {
      writer.uint32(18).string(message.link);
    }
    if (message.status !== 0) {
      writer.uint32(24).int32(message.status);
    }
    if (message.statusDetails !== undefined) {
      StatusDetails.encode(message.statusDetails, writer.uint32(34).fork()).ldelim();
    }
    if (message.summaryHtml !== "") {
      writer.uint32(42).string(message.summaryHtml);
    }
    if (message.details !== undefined) {
      Struct.encode(Struct.wrap(message.details), writer.uint32(50).fork()).ldelim();
    }
    if (message.updateId !== "0") {
      writer.uint32(56).int64(message.updateId);
    }
    if (message.summaryMarkdown !== "") {
      writer.uint32(66).string(message.summaryMarkdown);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Task {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTask() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.id = TaskID.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.link = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.statusDetails = StatusDetails.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.summaryHtml = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.details = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 56) {
            break;
          }

          message.updateId = longToString(reader.int64() as Long);
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.summaryMarkdown = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Task {
    return {
      id: isSet(object.id) ? TaskID.fromJSON(object.id) : undefined,
      link: isSet(object.link) ? globalThis.String(object.link) : "",
      status: isSet(object.status) ? statusFromJSON(object.status) : 0,
      statusDetails: isSet(object.statusDetails) ? StatusDetails.fromJSON(object.statusDetails) : undefined,
      summaryHtml: isSet(object.summaryHtml) ? globalThis.String(object.summaryHtml) : "",
      details: isObject(object.details) ? object.details : undefined,
      updateId: isSet(object.updateId) ? globalThis.String(object.updateId) : "0",
      summaryMarkdown: isSet(object.summaryMarkdown) ? globalThis.String(object.summaryMarkdown) : "",
    };
  },

  toJSON(message: Task): unknown {
    const obj: any = {};
    if (message.id !== undefined) {
      obj.id = TaskID.toJSON(message.id);
    }
    if (message.link !== "") {
      obj.link = message.link;
    }
    if (message.status !== 0) {
      obj.status = statusToJSON(message.status);
    }
    if (message.statusDetails !== undefined) {
      obj.statusDetails = StatusDetails.toJSON(message.statusDetails);
    }
    if (message.summaryHtml !== "") {
      obj.summaryHtml = message.summaryHtml;
    }
    if (message.details !== undefined) {
      obj.details = message.details;
    }
    if (message.updateId !== "0") {
      obj.updateId = message.updateId;
    }
    if (message.summaryMarkdown !== "") {
      obj.summaryMarkdown = message.summaryMarkdown;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Task>, I>>(base?: I): Task {
    return Task.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Task>, I>>(object: I): Task {
    const message = createBaseTask() as any;
    message.id = (object.id !== undefined && object.id !== null) ? TaskID.fromPartial(object.id) : undefined;
    message.link = object.link ?? "";
    message.status = object.status ?? 0;
    message.statusDetails = (object.statusDetails !== undefined && object.statusDetails !== null)
      ? StatusDetails.fromPartial(object.statusDetails)
      : undefined;
    message.summaryHtml = object.summaryHtml ?? "";
    message.details = object.details ?? undefined;
    message.updateId = object.updateId ?? "0";
    message.summaryMarkdown = object.summaryMarkdown ?? "";
    return message;
  },
};

function createBaseTaskID(): TaskID {
  return { target: "", id: "" };
}

export const TaskID = {
  encode(message: TaskID, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.target !== "") {
      writer.uint32(10).string(message.target);
    }
    if (message.id !== "") {
      writer.uint32(18).string(message.id);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TaskID {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTaskID() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.target = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.id = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TaskID {
    return {
      target: isSet(object.target) ? globalThis.String(object.target) : "",
      id: isSet(object.id) ? globalThis.String(object.id) : "",
    };
  },

  toJSON(message: TaskID): unknown {
    const obj: any = {};
    if (message.target !== "") {
      obj.target = message.target;
    }
    if (message.id !== "") {
      obj.id = message.id;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TaskID>, I>>(base?: I): TaskID {
    return TaskID.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TaskID>, I>>(object: I): TaskID {
    const message = createBaseTaskID() as any;
    message.target = object.target ?? "";
    message.id = object.id ?? "";
    return message;
  },
};

function createBaseBuildTaskUpdate(): BuildTaskUpdate {
  return { buildId: "", task: undefined };
}

export const BuildTaskUpdate = {
  encode(message: BuildTaskUpdate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.buildId !== "") {
      writer.uint32(10).string(message.buildId);
    }
    if (message.task !== undefined) {
      Task.encode(message.task, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildTaskUpdate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildTaskUpdate() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buildId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.task = Task.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildTaskUpdate {
    return {
      buildId: isSet(object.buildId) ? globalThis.String(object.buildId) : "",
      task: isSet(object.task) ? Task.fromJSON(object.task) : undefined,
    };
  },

  toJSON(message: BuildTaskUpdate): unknown {
    const obj: any = {};
    if (message.buildId !== "") {
      obj.buildId = message.buildId;
    }
    if (message.task !== undefined) {
      obj.task = Task.toJSON(message.task);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildTaskUpdate>, I>>(base?: I): BuildTaskUpdate {
    return BuildTaskUpdate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildTaskUpdate>, I>>(object: I): BuildTaskUpdate {
    const message = createBaseBuildTaskUpdate() as any;
    message.buildId = object.buildId ?? "";
    message.task = (object.task !== undefined && object.task !== null) ? Task.fromPartial(object.task) : undefined;
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
