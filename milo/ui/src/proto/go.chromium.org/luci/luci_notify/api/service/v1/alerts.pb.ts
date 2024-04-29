/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "luci.notify.v1";

export interface BatchGetAlertsRequest {
  /**
   * The resource names of the alerts to get.
   *
   * Currently by convention the keys match the keys in sheriff-o-matic, but
   * this is not a requirement.
   *
   * Format: alerts/{key}
   */
  readonly names: readonly string[];
}

/** The Status of a tree for an interval of time. */
export interface Alert {
  /**
   * The resource name of this alert.
   * Format: alerts/{key}
   */
  readonly name: string;
  /**
   * The buganizer bug ID of the bug associated with this alert.
   * 0 means the alert is not associated with any bug.
   */
  readonly bug: string;
  /**
   * The build number of the builder corresponding to the alert that this alert should be ignored until.
   * In other words, if the latest_failing_build_number (currently in SOM alerts) <= silence_until, this alert should be considered 'silenced'.
   */
  readonly silenceUntil: string;
  /**
   * The Gerrit CL number associated with this alert.
   * 0 means the alert is not associated with any CL.
   */
  readonly gerritCl: string;
  /**
   * The time the alert was last modified.
   *
   * This is automatically set by the server and cannot be modified explicitly
   * through RPC.
   */
  readonly modifyTime:
    | string
    | undefined;
  /**
   * This checksum is computed by the server based on the value of other
   * fields, and may be sent on update and delete requests to ensure the
   * client has an up-to-date value before proceeding.
   * Note that these etags are weak - they are only computed based on mutable
   * fields.  Other fields in the alert may be auto-updated but they will not
   * affect the etag value.
   * The etag field is optional on update requests, if not provided
   * the update will succeed.  If provided, the update will only succeed if
   * the etag is an exact match.
   */
  readonly etag: string;
}

export interface BatchGetAlertsResponse {
  /**
   * Alerts requested.
   * The order matches the order of names in the request.
   */
  readonly alerts: readonly Alert[];
}

export interface UpdateAlertRequest {
  /** The alert to update. */
  readonly alert: Alert | undefined;
}

export interface BatchUpdateAlertsRequest {
  /**
   * The request messages specifying the alerts to update.
   * A maximum of 1000 alerts can be modified in a batch.
   */
  readonly requests: readonly UpdateAlertRequest[];
}

export interface BatchUpdateAlertsResponse {
  /**
   * Alerts updated.
   * The order matches the order of names in the request.
   */
  readonly alerts: readonly Alert[];
}

function createBaseBatchGetAlertsRequest(): BatchGetAlertsRequest {
  return { names: [] };
}

export const BatchGetAlertsRequest = {
  encode(message: BatchGetAlertsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.names) {
      writer.uint32(10).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetAlertsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetAlertsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.names.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchGetAlertsRequest {
    return { names: globalThis.Array.isArray(object?.names) ? object.names.map((e: any) => globalThis.String(e)) : [] };
  },

  toJSON(message: BatchGetAlertsRequest): unknown {
    const obj: any = {};
    if (message.names?.length) {
      obj.names = message.names;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetAlertsRequest>, I>>(base?: I): BatchGetAlertsRequest {
    return BatchGetAlertsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetAlertsRequest>, I>>(object: I): BatchGetAlertsRequest {
    const message = createBaseBatchGetAlertsRequest() as any;
    message.names = object.names?.map((e) => e) || [];
    return message;
  },
};

function createBaseAlert(): Alert {
  return { name: "", bug: "0", silenceUntil: "0", gerritCl: "0", modifyTime: undefined, etag: "" };
}

export const Alert = {
  encode(message: Alert, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.bug !== "0") {
      writer.uint32(24).int64(message.bug);
    }
    if (message.silenceUntil !== "0") {
      writer.uint32(32).int64(message.silenceUntil);
    }
    if (message.gerritCl !== "0") {
      writer.uint32(56).int64(message.gerritCl);
    }
    if (message.modifyTime !== undefined) {
      Timestamp.encode(toTimestamp(message.modifyTime), writer.uint32(42).fork()).ldelim();
    }
    if (message.etag !== "") {
      writer.uint32(50).string(message.etag);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Alert {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseAlert() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.bug = longToString(reader.int64() as Long);
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.silenceUntil = longToString(reader.int64() as Long);
          continue;
        case 7:
          if (tag !== 56) {
            break;
          }

          message.gerritCl = longToString(reader.int64() as Long);
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.modifyTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.etag = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Alert {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      bug: isSet(object.bug) ? globalThis.String(object.bug) : "0",
      silenceUntil: isSet(object.silenceUntil) ? globalThis.String(object.silenceUntil) : "0",
      gerritCl: isSet(object.gerritCl) ? globalThis.String(object.gerritCl) : "0",
      modifyTime: isSet(object.modifyTime) ? globalThis.String(object.modifyTime) : undefined,
      etag: isSet(object.etag) ? globalThis.String(object.etag) : "",
    };
  },

  toJSON(message: Alert): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.bug !== "0") {
      obj.bug = message.bug;
    }
    if (message.silenceUntil !== "0") {
      obj.silenceUntil = message.silenceUntil;
    }
    if (message.gerritCl !== "0") {
      obj.gerritCl = message.gerritCl;
    }
    if (message.modifyTime !== undefined) {
      obj.modifyTime = message.modifyTime;
    }
    if (message.etag !== "") {
      obj.etag = message.etag;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Alert>, I>>(base?: I): Alert {
    return Alert.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Alert>, I>>(object: I): Alert {
    const message = createBaseAlert() as any;
    message.name = object.name ?? "";
    message.bug = object.bug ?? "0";
    message.silenceUntil = object.silenceUntil ?? "0";
    message.gerritCl = object.gerritCl ?? "0";
    message.modifyTime = object.modifyTime ?? undefined;
    message.etag = object.etag ?? "";
    return message;
  },
};

function createBaseBatchGetAlertsResponse(): BatchGetAlertsResponse {
  return { alerts: [] };
}

export const BatchGetAlertsResponse = {
  encode(message: BatchGetAlertsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.alerts) {
      Alert.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetAlertsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetAlertsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.alerts.push(Alert.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchGetAlertsResponse {
    return { alerts: globalThis.Array.isArray(object?.alerts) ? object.alerts.map((e: any) => Alert.fromJSON(e)) : [] };
  },

  toJSON(message: BatchGetAlertsResponse): unknown {
    const obj: any = {};
    if (message.alerts?.length) {
      obj.alerts = message.alerts.map((e) => Alert.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetAlertsResponse>, I>>(base?: I): BatchGetAlertsResponse {
    return BatchGetAlertsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetAlertsResponse>, I>>(object: I): BatchGetAlertsResponse {
    const message = createBaseBatchGetAlertsResponse() as any;
    message.alerts = object.alerts?.map((e) => Alert.fromPartial(e)) || [];
    return message;
  },
};

function createBaseUpdateAlertRequest(): UpdateAlertRequest {
  return { alert: undefined };
}

export const UpdateAlertRequest = {
  encode(message: UpdateAlertRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.alert !== undefined) {
      Alert.encode(message.alert, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UpdateAlertRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUpdateAlertRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.alert = Alert.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): UpdateAlertRequest {
    return { alert: isSet(object.alert) ? Alert.fromJSON(object.alert) : undefined };
  },

  toJSON(message: UpdateAlertRequest): unknown {
    const obj: any = {};
    if (message.alert !== undefined) {
      obj.alert = Alert.toJSON(message.alert);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<UpdateAlertRequest>, I>>(base?: I): UpdateAlertRequest {
    return UpdateAlertRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<UpdateAlertRequest>, I>>(object: I): UpdateAlertRequest {
    const message = createBaseUpdateAlertRequest() as any;
    message.alert = (object.alert !== undefined && object.alert !== null) ? Alert.fromPartial(object.alert) : undefined;
    return message;
  },
};

function createBaseBatchUpdateAlertsRequest(): BatchUpdateAlertsRequest {
  return { requests: [] };
}

export const BatchUpdateAlertsRequest = {
  encode(message: BatchUpdateAlertsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.requests) {
      UpdateAlertRequest.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchUpdateAlertsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchUpdateAlertsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.requests.push(UpdateAlertRequest.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchUpdateAlertsRequest {
    return {
      requests: globalThis.Array.isArray(object?.requests)
        ? object.requests.map((e: any) => UpdateAlertRequest.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BatchUpdateAlertsRequest): unknown {
    const obj: any = {};
    if (message.requests?.length) {
      obj.requests = message.requests.map((e) => UpdateAlertRequest.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchUpdateAlertsRequest>, I>>(base?: I): BatchUpdateAlertsRequest {
    return BatchUpdateAlertsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchUpdateAlertsRequest>, I>>(object: I): BatchUpdateAlertsRequest {
    const message = createBaseBatchUpdateAlertsRequest() as any;
    message.requests = object.requests?.map((e) => UpdateAlertRequest.fromPartial(e)) || [];
    return message;
  },
};

function createBaseBatchUpdateAlertsResponse(): BatchUpdateAlertsResponse {
  return { alerts: [] };
}

export const BatchUpdateAlertsResponse = {
  encode(message: BatchUpdateAlertsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.alerts) {
      Alert.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchUpdateAlertsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchUpdateAlertsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.alerts.push(Alert.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchUpdateAlertsResponse {
    return { alerts: globalThis.Array.isArray(object?.alerts) ? object.alerts.map((e: any) => Alert.fromJSON(e)) : [] };
  },

  toJSON(message: BatchUpdateAlertsResponse): unknown {
    const obj: any = {};
    if (message.alerts?.length) {
      obj.alerts = message.alerts.map((e) => Alert.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchUpdateAlertsResponse>, I>>(base?: I): BatchUpdateAlertsResponse {
    return BatchUpdateAlertsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchUpdateAlertsResponse>, I>>(object: I): BatchUpdateAlertsResponse {
    const message = createBaseBatchUpdateAlertsResponse() as any;
    message.alerts = object.alerts?.map((e) => Alert.fromPartial(e)) || [];
    return message;
  },
};

/**
 * Service Alerts exposes alerts used in on-call monitoring tools.
 * For now it only tracks mutable fields, with alerts being generated
 * by Sheriff-o-Matic, but eventually the alerts available through this
 * service will incorporate all of the needed information.
 */
export interface Alerts {
  /**
   * BatchGetAlerts allows getting a number of alerts by resource name.
   * If no alert exists by the given name an empty alert will be returned.
   */
  BatchGetAlerts(request: BatchGetAlertsRequest): Promise<BatchGetAlertsResponse>;
  /** BatchUpdateAlerts allows updating the mutable data on a batch of alerts. */
  BatchUpdateAlerts(request: BatchUpdateAlertsRequest): Promise<BatchUpdateAlertsResponse>;
}

export const AlertsServiceName = "luci.notify.v1.Alerts";
export class AlertsClientImpl implements Alerts {
  static readonly DEFAULT_SERVICE = AlertsServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || AlertsServiceName;
    this.rpc = rpc;
    this.BatchGetAlerts = this.BatchGetAlerts.bind(this);
    this.BatchUpdateAlerts = this.BatchUpdateAlerts.bind(this);
  }
  BatchGetAlerts(request: BatchGetAlertsRequest): Promise<BatchGetAlertsResponse> {
    const data = BatchGetAlertsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "BatchGetAlerts", data);
    return promise.then((data) => BatchGetAlertsResponse.fromJSON(data));
  }

  BatchUpdateAlerts(request: BatchUpdateAlertsRequest): Promise<BatchUpdateAlertsResponse> {
    const data = BatchUpdateAlertsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "BatchUpdateAlerts", data);
    return promise.then((data) => BatchUpdateAlertsResponse.fromJSON(data));
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
