/* eslint-disable */
import _m0 from "protobufjs/minimal";

export const protobufPackage = "infra.appengine.som.v1";

export interface ListAlertsRequest {
  /**
   * The name of the tree to retrieve alerts for
   * Format: trees/{tree}
   */
  readonly parent: string;
  /**
   * The maximum number of alerts to return.  The service may return fewer than
   * this value.
   * If unspecified, at most 1000 alerts will be returned.
   * The maximum value is 1000; values above 1000 will be coerced to 1000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous `ListAlerts` call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to `ListAlerts` must match
   * the call that provided the page token.
   */
  readonly pageToken: string;
}

export interface ListAlertsResponse {
  /** The requested alerts. */
  readonly alerts: readonly Alert[];
  /**
   * A token, which can be sent as `page_token` to retrieve the next page.
   * If this field is omitted, there are no subsequent pages.
   */
  readonly nextPageToken: string;
}

/** An alert corresponding to a failure of a step in a build. */
export interface Alert {
  /** The opaque, unique key of this alert.  This is also present in the alert_json field. */
  readonly key: string;
  /**
   * A string containing the JSON for the alert that would be returned by the REST API.
   *
   * This is left as JSON rather than proto fields because we intend to reduce usage of
   * this over time, so it is better to preserve backwards compatibility of the existing
   * UI codethan to break out invdividual fields which would require rewriting the code
   * which will soon be replaced anyway.
   */
  readonly alertJson: string;
}

function createBaseListAlertsRequest(): ListAlertsRequest {
  return { parent: "", pageSize: 0, pageToken: "" };
}

export const ListAlertsRequest = {
  encode(message: ListAlertsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
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

  decode(input: _m0.Reader | Uint8Array, length?: number): ListAlertsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListAlertsRequest() as any;
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

  fromJSON(object: any): ListAlertsRequest {
    return {
      parent: isSet(object.parent) ? globalThis.String(object.parent) : "",
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: ListAlertsRequest): unknown {
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

  create<I extends Exact<DeepPartial<ListAlertsRequest>, I>>(base?: I): ListAlertsRequest {
    return ListAlertsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListAlertsRequest>, I>>(object: I): ListAlertsRequest {
    const message = createBaseListAlertsRequest() as any;
    message.parent = object.parent ?? "";
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseListAlertsResponse(): ListAlertsResponse {
  return { alerts: [], nextPageToken: "" };
}

export const ListAlertsResponse = {
  encode(message: ListAlertsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.alerts) {
      Alert.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListAlertsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListAlertsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.alerts.push(Alert.decode(reader, reader.uint32()));
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

  fromJSON(object: any): ListAlertsResponse {
    return {
      alerts: globalThis.Array.isArray(object?.alerts) ? object.alerts.map((e: any) => Alert.fromJSON(e)) : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: ListAlertsResponse): unknown {
    const obj: any = {};
    if (message.alerts?.length) {
      obj.alerts = message.alerts.map((e) => Alert.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListAlertsResponse>, I>>(base?: I): ListAlertsResponse {
    return ListAlertsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListAlertsResponse>, I>>(object: I): ListAlertsResponse {
    const message = createBaseListAlertsResponse() as any;
    message.alerts = object.alerts?.map((e) => Alert.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseAlert(): Alert {
  return { key: "", alertJson: "" };
}

export const Alert = {
  encode(message: Alert, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.alertJson !== "") {
      writer.uint32(18).string(message.alertJson);
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

          message.key = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.alertJson = reader.string();
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
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      alertJson: isSet(object.alertJson) ? globalThis.String(object.alertJson) : "",
    };
  },

  toJSON(message: Alert): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.alertJson !== "") {
      obj.alertJson = message.alertJson;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Alert>, I>>(base?: I): Alert {
    return Alert.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Alert>, I>>(object: I): Alert {
    const message = createBaseAlert() as any;
    message.key = object.key ?? "";
    message.alertJson = object.alertJson ?? "";
    return message;
  },
};

/**
 * Provides methods to retrieve alerts.
 *
 * An alert corresponds to a step failure on a single builder.
 */
export interface Alerts {
  /** Get the list of unresolved alerts for a given tree. */
  ListAlerts(request: ListAlertsRequest): Promise<ListAlertsResponse>;
}

export const AlertsServiceName = "infra.appengine.som.v1.Alerts";
export class AlertsClientImpl implements Alerts {
  static readonly DEFAULT_SERVICE = AlertsServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || AlertsServiceName;
    this.rpc = rpc;
    this.ListAlerts = this.ListAlerts.bind(this);
  }
  ListAlerts(request: ListAlertsRequest): Promise<ListAlertsResponse> {
    const data = ListAlertsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ListAlerts", data);
    return promise.then((data) => ListAlertsResponse.fromJSON(data));
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}