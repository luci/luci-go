// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v1.176.0
//   protoc               v5.26.1
// source: go.chromium.org/luci/resultdb/sink/proto/v1/sink.proto

/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Empty } from "../../../../../../google/protobuf/empty.pb";
import { FieldMask } from "../../../../../../google/protobuf/field_mask.pb";
import { Struct } from "../../../../../../google/protobuf/struct.pb";
import { Artifact, TestResult } from "./test_result.pb";

export const protobufPackage = "luci.resultsink.v1";

export interface ReportTestResultsRequest {
  /** Test results to report. */
  readonly testResults: readonly TestResult[];
}

export interface ReportTestResultsResponse {
  /**
   * List of unique identifiers that can be used to link to these results
   * or requested via luci.resultdb.v1.ResultDB service.
   */
  readonly testResultNames: readonly string[];
}

export interface ReportInvocationLevelArtifactsRequest {
  /**
   * Invocation-level artifacts to report.
   * The map key is an artifact id.
   */
  readonly artifacts: { [key: string]: Artifact };
}

export interface ReportInvocationLevelArtifactsRequest_ArtifactsEntry {
  readonly key: string;
  readonly value: Artifact | undefined;
}

export interface UpdateInvocationRequest {
  /** Invocation to update. */
  readonly invocation:
    | Invocation
    | undefined;
  /**
   * The list of fields to be updated. See https://google.aip.dev/161.
   *
   * The following paths can be used for extended_properties:
   * * "extended_properties" to target the whole extended_properties,
   * * "extended_properties.some_key" to target one key of extended_properties.
   * See sink/sink_server.go for implementation.
   */
  readonly updateMask: readonly string[] | undefined;
}

/**
 * A local equivalent of the luci.resultdb.v1.Invocation message.
 * The 'name' field is omitted as result sink keeps track of which invocation
 * is being uploaded to.
 */
export interface Invocation {
  /**
   * See 'extended_properties' field of the luci.resultdb.v1.Invocation message
   * for details.
   */
  readonly extendedProperties: { [key: string]: { readonly [key: string]: any } | undefined };
}

export interface Invocation_ExtendedPropertiesEntry {
  readonly key: string;
  readonly value: { readonly [key: string]: any } | undefined;
}

function createBaseReportTestResultsRequest(): ReportTestResultsRequest {
  return { testResults: [] };
}

export const ReportTestResultsRequest = {
  encode(message: ReportTestResultsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testResults) {
      TestResult.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReportTestResultsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReportTestResultsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testResults.push(TestResult.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ReportTestResultsRequest {
    return {
      testResults: globalThis.Array.isArray(object?.testResults)
        ? object.testResults.map((e: any) => TestResult.fromJSON(e))
        : [],
    };
  },

  toJSON(message: ReportTestResultsRequest): unknown {
    const obj: any = {};
    if (message.testResults?.length) {
      obj.testResults = message.testResults.map((e) => TestResult.toJSON(e));
    }
    return obj;
  },

  create(base?: DeepPartial<ReportTestResultsRequest>): ReportTestResultsRequest {
    return ReportTestResultsRequest.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ReportTestResultsRequest>): ReportTestResultsRequest {
    const message = createBaseReportTestResultsRequest() as any;
    message.testResults = object.testResults?.map((e) => TestResult.fromPartial(e)) || [];
    return message;
  },
};

function createBaseReportTestResultsResponse(): ReportTestResultsResponse {
  return { testResultNames: [] };
}

export const ReportTestResultsResponse = {
  encode(message: ReportTestResultsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testResultNames) {
      writer.uint32(10).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReportTestResultsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReportTestResultsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testResultNames.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ReportTestResultsResponse {
    return {
      testResultNames: globalThis.Array.isArray(object?.testResultNames)
        ? object.testResultNames.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: ReportTestResultsResponse): unknown {
    const obj: any = {};
    if (message.testResultNames?.length) {
      obj.testResultNames = message.testResultNames;
    }
    return obj;
  },

  create(base?: DeepPartial<ReportTestResultsResponse>): ReportTestResultsResponse {
    return ReportTestResultsResponse.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ReportTestResultsResponse>): ReportTestResultsResponse {
    const message = createBaseReportTestResultsResponse() as any;
    message.testResultNames = object.testResultNames?.map((e) => e) || [];
    return message;
  },
};

function createBaseReportInvocationLevelArtifactsRequest(): ReportInvocationLevelArtifactsRequest {
  return { artifacts: {} };
}

export const ReportInvocationLevelArtifactsRequest = {
  encode(message: ReportInvocationLevelArtifactsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.artifacts).forEach(([key, value]) => {
      ReportInvocationLevelArtifactsRequest_ArtifactsEntry.encode({ key: key as any, value }, writer.uint32(10).fork())
        .ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReportInvocationLevelArtifactsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReportInvocationLevelArtifactsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          const entry1 = ReportInvocationLevelArtifactsRequest_ArtifactsEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.artifacts[entry1.key] = entry1.value;
          }
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ReportInvocationLevelArtifactsRequest {
    return {
      artifacts: isObject(object.artifacts)
        ? Object.entries(object.artifacts).reduce<{ [key: string]: Artifact }>((acc, [key, value]) => {
          acc[key] = Artifact.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: ReportInvocationLevelArtifactsRequest): unknown {
    const obj: any = {};
    if (message.artifacts) {
      const entries = Object.entries(message.artifacts);
      if (entries.length > 0) {
        obj.artifacts = {};
        entries.forEach(([k, v]) => {
          obj.artifacts[k] = Artifact.toJSON(v);
        });
      }
    }
    return obj;
  },

  create(base?: DeepPartial<ReportInvocationLevelArtifactsRequest>): ReportInvocationLevelArtifactsRequest {
    return ReportInvocationLevelArtifactsRequest.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<ReportInvocationLevelArtifactsRequest>): ReportInvocationLevelArtifactsRequest {
    const message = createBaseReportInvocationLevelArtifactsRequest() as any;
    message.artifacts = Object.entries(object.artifacts ?? {}).reduce<{ [key: string]: Artifact }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = Artifact.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBaseReportInvocationLevelArtifactsRequest_ArtifactsEntry(): ReportInvocationLevelArtifactsRequest_ArtifactsEntry {
  return { key: "", value: undefined };
}

export const ReportInvocationLevelArtifactsRequest_ArtifactsEntry = {
  encode(
    message: ReportInvocationLevelArtifactsRequest_ArtifactsEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      Artifact.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ReportInvocationLevelArtifactsRequest_ArtifactsEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseReportInvocationLevelArtifactsRequest_ArtifactsEntry() as any;
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

          message.value = Artifact.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ReportInvocationLevelArtifactsRequest_ArtifactsEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? Artifact.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: ReportInvocationLevelArtifactsRequest_ArtifactsEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = Artifact.toJSON(message.value);
    }
    return obj;
  },

  create(
    base?: DeepPartial<ReportInvocationLevelArtifactsRequest_ArtifactsEntry>,
  ): ReportInvocationLevelArtifactsRequest_ArtifactsEntry {
    return ReportInvocationLevelArtifactsRequest_ArtifactsEntry.fromPartial(base ?? {});
  },
  fromPartial(
    object: DeepPartial<ReportInvocationLevelArtifactsRequest_ArtifactsEntry>,
  ): ReportInvocationLevelArtifactsRequest_ArtifactsEntry {
    const message = createBaseReportInvocationLevelArtifactsRequest_ArtifactsEntry() as any;
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? Artifact.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseUpdateInvocationRequest(): UpdateInvocationRequest {
  return { invocation: undefined, updateMask: undefined };
}

export const UpdateInvocationRequest = {
  encode(message: UpdateInvocationRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.invocation !== undefined) {
      Invocation.encode(message.invocation, writer.uint32(10).fork()).ldelim();
    }
    if (message.updateMask !== undefined) {
      FieldMask.encode(FieldMask.wrap(message.updateMask), writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UpdateInvocationRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUpdateInvocationRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.invocation = Invocation.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.updateMask = FieldMask.unwrap(FieldMask.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): UpdateInvocationRequest {
    return {
      invocation: isSet(object.invocation) ? Invocation.fromJSON(object.invocation) : undefined,
      updateMask: isSet(object.updateMask) ? FieldMask.unwrap(FieldMask.fromJSON(object.updateMask)) : undefined,
    };
  },

  toJSON(message: UpdateInvocationRequest): unknown {
    const obj: any = {};
    if (message.invocation !== undefined) {
      obj.invocation = Invocation.toJSON(message.invocation);
    }
    if (message.updateMask !== undefined) {
      obj.updateMask = FieldMask.toJSON(FieldMask.wrap(message.updateMask));
    }
    return obj;
  },

  create(base?: DeepPartial<UpdateInvocationRequest>): UpdateInvocationRequest {
    return UpdateInvocationRequest.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<UpdateInvocationRequest>): UpdateInvocationRequest {
    const message = createBaseUpdateInvocationRequest() as any;
    message.invocation = (object.invocation !== undefined && object.invocation !== null)
      ? Invocation.fromPartial(object.invocation)
      : undefined;
    message.updateMask = object.updateMask ?? undefined;
    return message;
  },
};

function createBaseInvocation(): Invocation {
  return { extendedProperties: {} };
}

export const Invocation = {
  encode(message: Invocation, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.extendedProperties).forEach(([key, value]) => {
      if (value !== undefined) {
        Invocation_ExtendedPropertiesEntry.encode({ key: key as any, value }, writer.uint32(10).fork()).ldelim();
      }
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Invocation {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInvocation() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          const entry1 = Invocation_ExtendedPropertiesEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.extendedProperties[entry1.key] = entry1.value;
          }
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Invocation {
    return {
      extendedProperties: isObject(object.extendedProperties)
        ? Object.entries(object.extendedProperties).reduce<
          { [key: string]: { readonly [key: string]: any } | undefined }
        >((acc, [key, value]) => {
          acc[key] = value as { readonly [key: string]: any } | undefined;
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: Invocation): unknown {
    const obj: any = {};
    if (message.extendedProperties) {
      const entries = Object.entries(message.extendedProperties);
      if (entries.length > 0) {
        obj.extendedProperties = {};
        entries.forEach(([k, v]) => {
          obj.extendedProperties[k] = v;
        });
      }
    }
    return obj;
  },

  create(base?: DeepPartial<Invocation>): Invocation {
    return Invocation.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<Invocation>): Invocation {
    const message = createBaseInvocation() as any;
    message.extendedProperties = Object.entries(object.extendedProperties ?? {}).reduce<
      { [key: string]: { readonly [key: string]: any } | undefined }
    >((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = value;
      }
      return acc;
    }, {});
    return message;
  },
};

function createBaseInvocation_ExtendedPropertiesEntry(): Invocation_ExtendedPropertiesEntry {
  return { key: "", value: undefined };
}

export const Invocation_ExtendedPropertiesEntry = {
  encode(message: Invocation_ExtendedPropertiesEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      Struct.encode(Struct.wrap(message.value), writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Invocation_ExtendedPropertiesEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInvocation_ExtendedPropertiesEntry() as any;
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

          message.value = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Invocation_ExtendedPropertiesEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isObject(object.value) ? object.value : undefined,
    };
  },

  toJSON(message: Invocation_ExtendedPropertiesEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = message.value;
    }
    return obj;
  },

  create(base?: DeepPartial<Invocation_ExtendedPropertiesEntry>): Invocation_ExtendedPropertiesEntry {
    return Invocation_ExtendedPropertiesEntry.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<Invocation_ExtendedPropertiesEntry>): Invocation_ExtendedPropertiesEntry {
    const message = createBaseInvocation_ExtendedPropertiesEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? undefined;
    return message;
  },
};

/**
 * Service to report test results.
 *
 * Note that clients need to add the auth token in the HTTP header when invoking
 * the RPCs of this service, or Unauthenticated error will be returned.
 * i.e., Authorization: ResultSink <auth-token>
 *
 * The auth token is available via resultdb.resultsink.auth_token LUCI_CONTEXT
 * value. For more information, visit
 * https://github.com/luci/luci-py/blob/master/client/LUCI_CONTEXT.md
 */
export interface Sink {
  /** Reports test results. */
  ReportTestResults(request: ReportTestResultsRequest): Promise<ReportTestResultsResponse>;
  /**
   * Reports invocation-level artifacts.
   * To upload result-level artifact, use ReportTestResults instead.
   */
  ReportInvocationLevelArtifacts(request: ReportInvocationLevelArtifactsRequest): Promise<Empty>;
  /** Update an invocation */
  UpdateInvocation(request: UpdateInvocationRequest): Promise<Invocation>;
}

export const SinkServiceName = "luci.resultsink.v1.Sink";
export class SinkClientImpl implements Sink {
  static readonly DEFAULT_SERVICE = SinkServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || SinkServiceName;
    this.rpc = rpc;
    this.ReportTestResults = this.ReportTestResults.bind(this);
    this.ReportInvocationLevelArtifacts = this.ReportInvocationLevelArtifacts.bind(this);
    this.UpdateInvocation = this.UpdateInvocation.bind(this);
  }
  ReportTestResults(request: ReportTestResultsRequest): Promise<ReportTestResultsResponse> {
    const data = ReportTestResultsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ReportTestResults", data);
    return promise.then((data) => ReportTestResultsResponse.fromJSON(data));
  }

  ReportInvocationLevelArtifacts(request: ReportInvocationLevelArtifactsRequest): Promise<Empty> {
    const data = ReportInvocationLevelArtifactsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ReportInvocationLevelArtifacts", data);
    return promise.then((data) => Empty.fromJSON(data));
  }

  UpdateInvocation(request: UpdateInvocationRequest): Promise<Invocation> {
    const data = UpdateInvocationRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "UpdateInvocation", data);
    return promise.then((data) => Invocation.fromJSON(data));
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}