/* eslint-disable */
import _m0 from "protobufjs/minimal";

export const protobufPackage = "luci.analysis.v1";

/** A request to list metrics in a given LUCI project. */
export interface ListProjectMetricsRequest {
  /**
   * The parent LUCI Project, which owns the collection of metrics.
   * Format: projects/{project}.
   */
  readonly parent: string;
}

/**
 * Lists the metrics available in a LUCI Project.
 * Designed to follow aip.dev/132.
 */
export interface ListProjectMetricsResponse {
  /** The metrics available in the LUCI Project. */
  readonly metrics: readonly ProjectMetric[];
}

/** A metric with LUCI project-specific configuration attached. */
export interface ProjectMetric {
  /**
   * The resource name of the metric.
   * Format: projects/{project}/metrics/{metric_id}.
   * See aip.dev/122 for more.
   */
  readonly name: string;
  /**
   * The identifier of the metric.
   * Follows the pattern: ^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$.
   */
  readonly metricId: string;
  /**
   * A human readable name for the metric. E.g.
   * "User CLs Failed Presubmit".
   */
  readonly humanReadableName: string;
  /**
   * A human readable description of the metric. Normally
   * this appears in a help popup near the metric.
   */
  readonly description: string;
  /**
   * Whether the metric should be shown by default in
   * the cluster listing and on cluster pages.
   */
  readonly isDefault: boolean;
  /**
   * SortPriority defines the order by which metrics are sorted by default.
   * The metric with the highest sort priority will define the
   * (default) primary sort order, followed by the metric with the
   * second highest sort priority, and so on.
   * Each metric is guaranteed to have a unique sort priority.
   */
  readonly sortPriority: number;
}

function createBaseListProjectMetricsRequest(): ListProjectMetricsRequest {
  return { parent: "" };
}

export const ListProjectMetricsRequest = {
  encode(message: ListProjectMetricsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.parent !== "") {
      writer.uint32(10).string(message.parent);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListProjectMetricsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListProjectMetricsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.parent = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListProjectMetricsRequest {
    return { parent: isSet(object.parent) ? globalThis.String(object.parent) : "" };
  },

  toJSON(message: ListProjectMetricsRequest): unknown {
    const obj: any = {};
    if (message.parent !== "") {
      obj.parent = message.parent;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListProjectMetricsRequest>, I>>(base?: I): ListProjectMetricsRequest {
    return ListProjectMetricsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListProjectMetricsRequest>, I>>(object: I): ListProjectMetricsRequest {
    const message = createBaseListProjectMetricsRequest() as any;
    message.parent = object.parent ?? "";
    return message;
  },
};

function createBaseListProjectMetricsResponse(): ListProjectMetricsResponse {
  return { metrics: [] };
}

export const ListProjectMetricsResponse = {
  encode(message: ListProjectMetricsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.metrics) {
      ProjectMetric.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListProjectMetricsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListProjectMetricsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.metrics.push(ProjectMetric.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListProjectMetricsResponse {
    return {
      metrics: globalThis.Array.isArray(object?.metrics)
        ? object.metrics.map((e: any) => ProjectMetric.fromJSON(e))
        : [],
    };
  },

  toJSON(message: ListProjectMetricsResponse): unknown {
    const obj: any = {};
    if (message.metrics?.length) {
      obj.metrics = message.metrics.map((e) => ProjectMetric.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListProjectMetricsResponse>, I>>(base?: I): ListProjectMetricsResponse {
    return ListProjectMetricsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListProjectMetricsResponse>, I>>(object: I): ListProjectMetricsResponse {
    const message = createBaseListProjectMetricsResponse() as any;
    message.metrics = object.metrics?.map((e) => ProjectMetric.fromPartial(e)) || [];
    return message;
  },
};

function createBaseProjectMetric(): ProjectMetric {
  return { name: "", metricId: "", humanReadableName: "", description: "", isDefault: false, sortPriority: 0 };
}

export const ProjectMetric = {
  encode(message: ProjectMetric, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.metricId !== "") {
      writer.uint32(18).string(message.metricId);
    }
    if (message.humanReadableName !== "") {
      writer.uint32(26).string(message.humanReadableName);
    }
    if (message.description !== "") {
      writer.uint32(34).string(message.description);
    }
    if (message.isDefault === true) {
      writer.uint32(40).bool(message.isDefault);
    }
    if (message.sortPriority !== 0) {
      writer.uint32(48).int32(message.sortPriority);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectMetric {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectMetric() as any;
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
          if (tag !== 18) {
            break;
          }

          message.metricId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.humanReadableName = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.description = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.isDefault = reader.bool();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.sortPriority = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ProjectMetric {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      metricId: isSet(object.metricId) ? globalThis.String(object.metricId) : "",
      humanReadableName: isSet(object.humanReadableName) ? globalThis.String(object.humanReadableName) : "",
      description: isSet(object.description) ? globalThis.String(object.description) : "",
      isDefault: isSet(object.isDefault) ? globalThis.Boolean(object.isDefault) : false,
      sortPriority: isSet(object.sortPriority) ? globalThis.Number(object.sortPriority) : 0,
    };
  },

  toJSON(message: ProjectMetric): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.metricId !== "") {
      obj.metricId = message.metricId;
    }
    if (message.humanReadableName !== "") {
      obj.humanReadableName = message.humanReadableName;
    }
    if (message.description !== "") {
      obj.description = message.description;
    }
    if (message.isDefault === true) {
      obj.isDefault = message.isDefault;
    }
    if (message.sortPriority !== 0) {
      obj.sortPriority = Math.round(message.sortPriority);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectMetric>, I>>(base?: I): ProjectMetric {
    return ProjectMetric.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ProjectMetric>, I>>(object: I): ProjectMetric {
    const message = createBaseProjectMetric() as any;
    message.name = object.name ?? "";
    message.metricId = object.metricId ?? "";
    message.humanReadableName = object.humanReadableName ?? "";
    message.description = object.description ?? "";
    message.isDefault = object.isDefault ?? false;
    message.sortPriority = object.sortPriority ?? 0;
    return message;
  },
};

/** Provides information about metrics in LUCI Analysis. */
export interface Metrics {
  /**
   * ListForProject lists metrics in a given LUCI Project.
   * Designed to follow aip.dev/132.
   */
  listForProject(request: ListProjectMetricsRequest): Promise<ListProjectMetricsResponse>;
}

export const MetricsServiceName = "luci.analysis.v1.Metrics";
export class MetricsClientImpl implements Metrics {
  static readonly DEFAULT_SERVICE = MetricsServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || MetricsServiceName;
    this.rpc = rpc;
    this.listForProject = this.listForProject.bind(this);
  }
  listForProject(request: ListProjectMetricsRequest): Promise<ListProjectMetricsResponse> {
    const data = ListProjectMetricsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ListForProject", data);
    return promise.then((data) => ListProjectMetricsResponse.fromJSON(data));
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