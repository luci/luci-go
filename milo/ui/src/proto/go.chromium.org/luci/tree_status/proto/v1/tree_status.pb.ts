/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "luci.tree_status.v1";

/** GeneralState are the possible states for a tree to be in. */
export enum GeneralState {
  /**
   * UNSPECIFIED - GeneralState was not specified.
   * This should not be used, it is the default value for an unset field.
   */
  UNSPECIFIED = 0,
  /** OPEN - The tree is open and accepting new commits. */
  OPEN = 1,
  /** CLOSED - The tree is closed, no new commits are currently being accepted. */
  CLOSED = 2,
  /**
   * THROTTLED - The tree is throttled.  The meaning of this state can vary by project,
   * but generally it is between the open and closed states.
   */
  THROTTLED = 3,
  /**
   * MAINTENANCE - The tree is in maintenance.  Generally CLs will not be accepted while the
   * tree is in this state.
   */
  MAINTENANCE = 4,
}

export function generalStateFromJSON(object: any): GeneralState {
  switch (object) {
    case 0:
    case "GENERAL_STATE_UNSPECIFIED":
      return GeneralState.UNSPECIFIED;
    case 1:
    case "OPEN":
      return GeneralState.OPEN;
    case 2:
    case "CLOSED":
      return GeneralState.CLOSED;
    case 3:
    case "THROTTLED":
      return GeneralState.THROTTLED;
    case 4:
    case "MAINTENANCE":
      return GeneralState.MAINTENANCE;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum GeneralState");
  }
}

export function generalStateToJSON(object: GeneralState): string {
  switch (object) {
    case GeneralState.UNSPECIFIED:
      return "GENERAL_STATE_UNSPECIFIED";
    case GeneralState.OPEN:
      return "OPEN";
    case GeneralState.CLOSED:
      return "CLOSED";
    case GeneralState.THROTTLED:
      return "THROTTLED";
    case GeneralState.MAINTENANCE:
      return "MAINTENANCE";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum GeneralState");
  }
}

export interface GetStatusRequest {
  /**
   * The status value to get.
   *
   * You can use 'latest' as the id to get the latest status for a tree,
   * i.e. set the name to 'trees/{tree}/status/latest'.
   *
   * Format: trees/{tree}/status/{id}
   */
  readonly name: string;
}

/** The Status of a tree for an interval of time. */
export interface Status {
  /**
   * The name of this status.
   * Format: trees/{tree}/status/{id}
   */
  readonly name: string;
  /**
   * The general state of the tree.  Possible values are open, closed, throttled
   * and maintenance.
   */
  readonly generalState: GeneralState;
  /**
   * The message explaining details about the status.  This may contain HTML,
   * it is the responsibility of the caller to sanitize the HTML before display.
   * Maximum length of 1024 bytes.  Must be a valid UTF-8 string in normalized form
   * C without any non-printable runes.
   */
  readonly message: string;
  /**
   * The email address of the user who added this.  May be empty if
   * the reader does not have permission to see personal data.  Will also be
   * set to 'user' after the user data TTL (of 30 days).
   */
  readonly createUser: string;
  /** The time the status update was made. */
  readonly createTime: string | undefined;
}

export interface ListStatusRequest {
  /**
   * The parent tree which the status values belongs to.
   * Format: trees/{tree}/status
   */
  readonly parent: string;
  /**
   * The maximum number of status values to return. The service may return fewer
   * than this value. If unspecified, at most 50 status values will be returned.
   * The maximum value is 1000; values above 1000 will be coerced to 1000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous `ListStatus` call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to `ListStatus` must match
   * the call that provided the page token.
   */
  readonly pageToken: string;
}

export interface ListStatusResponse {
  /** The status values of the tree. */
  readonly status: readonly Status[];
  /**
   * A token, which can be sent as `page_token` to retrieve the next page.
   * If this field is omitted, there are no subsequent pages.
   */
  readonly nextPageToken: string;
}

export interface CreateStatusRequest {
  /**
   * The parent tree which the status values belongs to.
   * Format: trees/{tree}/status
   */
  readonly parent: string;
  /**
   * The status to create.
   * Only the general state and message fields can be provided, the current date
   * will be used for the date and the RPC caller will be used for the username.
   */
  readonly status: Status | undefined;
}

function createBaseGetStatusRequest(): GetStatusRequest {
  return { name: "" };
}

export const GetStatusRequest = {
  encode(message: GetStatusRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetStatusRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetStatusRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GetStatusRequest {
    return { name: isSet(object.name) ? globalThis.String(object.name) : "" };
  },

  toJSON(message: GetStatusRequest): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetStatusRequest>, I>>(base?: I): GetStatusRequest {
    return GetStatusRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetStatusRequest>, I>>(object: I): GetStatusRequest {
    const message = createBaseGetStatusRequest() as any;
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseStatus(): Status {
  return { name: "", generalState: 0, message: "", createUser: "", createTime: undefined };
}

export const Status = {
  encode(message: Status, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.generalState !== 0) {
      writer.uint32(16).int32(message.generalState);
    }
    if (message.message !== "") {
      writer.uint32(26).string(message.message);
    }
    if (message.createUser !== "") {
      writer.uint32(34).string(message.createUser);
    }
    if (message.createTime !== undefined) {
      Timestamp.encode(toTimestamp(message.createTime), writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Status {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStatus() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.generalState = reader.int32() as any;
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.message = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.createUser = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.createTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Status {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      generalState: isSet(object.generalState) ? generalStateFromJSON(object.generalState) : 0,
      message: isSet(object.message) ? globalThis.String(object.message) : "",
      createUser: isSet(object.createUser) ? globalThis.String(object.createUser) : "",
      createTime: isSet(object.createTime) ? globalThis.String(object.createTime) : undefined,
    };
  },

  toJSON(message: Status): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.generalState !== 0) {
      obj.generalState = generalStateToJSON(message.generalState);
    }
    if (message.message !== "") {
      obj.message = message.message;
    }
    if (message.createUser !== "") {
      obj.createUser = message.createUser;
    }
    if (message.createTime !== undefined) {
      obj.createTime = message.createTime;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Status>, I>>(base?: I): Status {
    return Status.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Status>, I>>(object: I): Status {
    const message = createBaseStatus() as any;
    message.name = object.name ?? "";
    message.generalState = object.generalState ?? 0;
    message.message = object.message ?? "";
    message.createUser = object.createUser ?? "";
    message.createTime = object.createTime ?? undefined;
    return message;
  },
};

function createBaseListStatusRequest(): ListStatusRequest {
  return { parent: "", pageSize: 0, pageToken: "" };
}

export const ListStatusRequest = {
  encode(message: ListStatusRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.parent !== "") {
      writer.uint32(10).string(message.parent);
    }
    if (message.pageSize !== 0) {
      writer.uint32(16).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(26).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListStatusRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListStatusRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.parent = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.pageToken = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListStatusRequest {
    return {
      parent: isSet(object.parent) ? globalThis.String(object.parent) : "",
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: ListStatusRequest): unknown {
    const obj: any = {};
    if (message.parent !== "") {
      obj.parent = message.parent;
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListStatusRequest>, I>>(base?: I): ListStatusRequest {
    return ListStatusRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListStatusRequest>, I>>(object: I): ListStatusRequest {
    const message = createBaseListStatusRequest() as any;
    message.parent = object.parent ?? "";
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseListStatusResponse(): ListStatusResponse {
  return { status: [], nextPageToken: "" };
}

export const ListStatusResponse = {
  encode(message: ListStatusResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.status) {
      Status.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListStatusResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListStatusResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.status.push(Status.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.nextPageToken = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListStatusResponse {
    return {
      status: globalThis.Array.isArray(object?.status) ? object.status.map((e: any) => Status.fromJSON(e)) : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: ListStatusResponse): unknown {
    const obj: any = {};
    if (message.status?.length) {
      obj.status = message.status.map((e) => Status.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListStatusResponse>, I>>(base?: I): ListStatusResponse {
    return ListStatusResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListStatusResponse>, I>>(object: I): ListStatusResponse {
    const message = createBaseListStatusResponse() as any;
    message.status = object.status?.map((e) => Status.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseCreateStatusRequest(): CreateStatusRequest {
  return { parent: "", status: undefined };
}

export const CreateStatusRequest = {
  encode(message: CreateStatusRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.parent !== "") {
      writer.uint32(10).string(message.parent);
    }
    if (message.status !== undefined) {
      Status.encode(message.status, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CreateStatusRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCreateStatusRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.parent = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.status = Status.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): CreateStatusRequest {
    return {
      parent: isSet(object.parent) ? globalThis.String(object.parent) : "",
      status: isSet(object.status) ? Status.fromJSON(object.status) : undefined,
    };
  },

  toJSON(message: CreateStatusRequest): unknown {
    const obj: any = {};
    if (message.parent !== "") {
      obj.parent = message.parent;
    }
    if (message.status !== undefined) {
      obj.status = Status.toJSON(message.status);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<CreateStatusRequest>, I>>(base?: I): CreateStatusRequest {
    return CreateStatusRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<CreateStatusRequest>, I>>(object: I): CreateStatusRequest {
    const message = createBaseCreateStatusRequest() as any;
    message.parent = object.parent ?? "";
    message.status = (object.status !== undefined && object.status !== null)
      ? Status.fromPartial(object.status)
      : undefined;
    return message;
  },
};

export interface TreeStatus {
  /** List all status values for a tree in reverse chronological order. */
  ListStatus(request: ListStatusRequest): Promise<ListStatusResponse>;
  /**
   * Get a status for a tree.
   * Use the resource alias 'latest' to get just the current status.
   */
  GetStatus(request: GetStatusRequest): Promise<Status>;
  /** Create a new status update for the tree. */
  CreateStatus(request: CreateStatusRequest): Promise<Status>;
}

export const TreeStatusServiceName = "luci.tree_status.v1.TreeStatus";
export class TreeStatusClientImpl implements TreeStatus {
  static readonly DEFAULT_SERVICE = TreeStatusServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || TreeStatusServiceName;
    this.rpc = rpc;
    this.ListStatus = this.ListStatus.bind(this);
    this.GetStatus = this.GetStatus.bind(this);
    this.CreateStatus = this.CreateStatus.bind(this);
  }
  ListStatus(request: ListStatusRequest): Promise<ListStatusResponse> {
    const data = ListStatusRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ListStatus", data);
    return promise.then((data) => ListStatusResponse.fromJSON(data));
  }

  GetStatus(request: GetStatusRequest): Promise<Status> {
    const data = GetStatusRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetStatus", data);
    return promise.then((data) => Status.fromJSON(data));
  }

  CreateStatus(request: CreateStatusRequest): Promise<Status> {
    const data = CreateStatusRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "CreateStatus", data);
    return promise.then((data) => Status.fromJSON(data));
  }
}

interface Rpc {
  request(service: string, method: string, data: unknown): Promise<unknown>;
}

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

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