/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Duration } from "../../../../../google/protobuf/duration.pb";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { Variant } from "./common.pb";
import { TestVerdictPredicate, VariantPredicate } from "./predicate.pb";
import { TestVerdict } from "./test_verdict.pb";

export const protobufPackage = "luci.analysis.v1";

/** A request message for `TestHistory.Query` RPC. */
export interface QueryTestHistoryRequest {
  /**
   * Required. The LUCI Project of the test results.
   * I.e. For a result to be part of the history, it needs to be contained
   * transitively by an invocation in this project.
   */
  readonly project: string;
  /** Required. The test ID to query the history from. */
  readonly testId: string;
  /** Required. A test verdict in the response must satisfy this predicate. */
  readonly predicate:
    | TestVerdictPredicate
    | undefined;
  /**
   * The maximum number of entries to return.
   *
   * The service may return fewer than this value.
   * If unspecified, at most 100 variants will be returned.
   * The maximum value is 1000; values above 1000 will be coerced to 1000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to the next call MUST
   * match the call that provided the page token.
   */
  readonly pageToken: string;
}

/** A response message for `TestHistory.Query` RPC. */
export interface QueryTestHistoryResponse {
  /**
   * The list of test verdicts.
   * Test verdicts will be ordered by `partition_time` DESC, `variant_hash` ASC,
   * `invocation_id` ASC.
   */
  readonly verdicts: readonly TestVerdict[];
  /**
   * This field will be set if there are more results to return.
   * To get the next page of data, send the same request again, but include this
   * token.
   */
  readonly nextPageToken: string;
}

/** A request message for `TestHistory.QueryStats` RPC. */
export interface QueryTestHistoryStatsRequest {
  /**
   * Required. The LUCI Project of the test results.
   * I.e. For a result to be part of the history, it needs to be contained
   * transitively by an invocation in this project.
   */
  readonly project: string;
  /** Required. The test ID to query the history from. */
  readonly testId: string;
  /** Required. A test verdict in the response must satisfy this predicate. */
  readonly predicate:
    | TestVerdictPredicate
    | undefined;
  /**
   * The maximum number of entries to return.
   *
   * The service may return fewer than this value.
   * If unspecified, at most 100 variants will be returned.
   * The maximum value is 1000; values above 1000 will be coerced to 1000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to the next call
   * MUST match the call that provided the page token.
   */
  readonly pageToken: string;
}

/** A response message for `TestHistory.QueryStats` RPC. */
export interface QueryTestHistoryStatsResponse {
  /**
   * The list of test verdict groups. Test verdicts will be grouped and ordered
   * by `partition_date` DESC, `variant_hash` ASC.
   */
  readonly groups: readonly QueryTestHistoryStatsResponse_Group[];
  /**
   * This field will be set if there are more results to return.
   * To get the next page of data, send the same request again, but include this
   * token.
   */
  readonly nextPageToken: string;
}

export interface QueryTestHistoryStatsResponse_Group {
  /**
   * The start time of this group.
   * Test verdicts that are paritioned in the 24 hours following this
   * timestamp are captured in this group.
   */
  readonly partitionTime:
    | string
    | undefined;
  /** The hash of the variant. */
  readonly variantHash: string;
  /** The number of unexpected test verdicts in the group. */
  readonly unexpectedCount: number;
  /** The number of unexpectedly skipped test verdicts in the group. */
  readonly unexpectedlySkippedCount: number;
  /** The number of flaky test verdicts in the group. */
  readonly flakyCount: number;
  /** The number of exonerated test verdicts in the group. */
  readonly exoneratedCount: number;
  /** The number of expected test verdicts in the group. */
  readonly expectedCount: number;
  /** The average duration of passing test results in the group. */
  readonly passedAvgDuration: Duration | undefined;
}

/** A request message for the `QueryVariants` RPC. */
export interface QueryVariantsRequest {
  /** Required. The LUCI project to query the variants from. */
  readonly project: string;
  /** Required. The test ID to query the variants from. */
  readonly testId: string;
  /**
   * Optional. The project-scoped realm to query the variants from.
   * This is the realm without the "<project>:" prefix.
   *
   * When specified, only the test variants found in the matching realm will be
   * returned.
   */
  readonly subRealm: string;
  /**
   * Optional. When specified, only variant matches this predicate will be
   * returned.
   */
  readonly variantPredicate:
    | VariantPredicate
    | undefined;
  /**
   * The maximum number of variants to return.
   *
   * The service may return fewer than this value.
   * If unspecified, at most 100 variants will be returned.
   * The maximum value is 1000; values above 1000 will be coerced to 1000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous `QueryVariants` call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to `QueryVariants` MUST
   * match the call that provided the page token.
   */
  readonly pageToken: string;
}

/** A response message for the `QueryVariants` RPC. */
export interface QueryVariantsResponse {
  /** A list of variants. Ordered by variant hash. */
  readonly variants: readonly QueryVariantsResponse_VariantInfo[];
  /**
   * A token, which can be sent as `page_token` to retrieve the next page.
   * If this field is omitted, there were no subsequent pages at the time of
   * request.
   */
  readonly nextPageToken: string;
}

/** Contains the variant definition and its hash. */
export interface QueryVariantsResponse_VariantInfo {
  /** The hash of the variant. */
  readonly variantHash: string;
  /** The definition of the variant. */
  readonly variant: Variant | undefined;
}

/** A request message for the `QueryTests` RPC. */
export interface QueryTestsRequest {
  /** Required. The LUCI project to query the tests from. */
  readonly project: string;
  /** Required. Only tests that contain the substring will be returned. */
  readonly testIdSubstring: string;
  /**
   * Optional. The project-scoped realm to query the variants from.
   * This is the realm without the "<project>:" prefix.
   *
   * When specified, only the tests found in the matching realm will be
   * returned.
   */
  readonly subRealm: string;
  /**
   * The maximum number of test IDs to return.
   *
   * The service may return fewer than this value.
   * If unspecified, at most 100 test IDs will be returned.
   * The maximum value is 1000; values above 1000 will be coerced to 1000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous `QueryTests` call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to `QueryTests` MUST
   * match the call that provided the page token.
   */
  readonly pageToken: string;
}

/** A response message for the `QueryTests` RPC. */
export interface QueryTestsResponse {
  /** A list of test Ids. Ordered alphabetically. */
  readonly testIds: readonly string[];
  /**
   * A token, which can be sent as `page_token` to retrieve the next page.
   * If this field is omitted, there were no subsequent pages at the time of
   * request.
   */
  readonly nextPageToken: string;
}

function createBaseQueryTestHistoryRequest(): QueryTestHistoryRequest {
  return { project: "", testId: "", predicate: undefined, pageSize: 0, pageToken: "" };
}

export const QueryTestHistoryRequest = {
  encode(message: QueryTestHistoryRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(18).string(message.testId);
    }
    if (message.predicate !== undefined) {
      TestVerdictPredicate.encode(message.predicate, writer.uint32(26).fork()).ldelim();
    }
    if (message.pageSize !== 0) {
      writer.uint32(32).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(42).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestHistoryRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestHistoryRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.predicate = TestVerdictPredicate.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 5:
          if (tag !== 42) {
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

  fromJSON(object: any): QueryTestHistoryRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      predicate: isSet(object.predicate) ? TestVerdictPredicate.fromJSON(object.predicate) : undefined,
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: QueryTestHistoryRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.predicate !== undefined) {
      obj.predicate = TestVerdictPredicate.toJSON(message.predicate);
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestHistoryRequest>, I>>(base?: I): QueryTestHistoryRequest {
    return QueryTestHistoryRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestHistoryRequest>, I>>(object: I): QueryTestHistoryRequest {
    const message = createBaseQueryTestHistoryRequest() as any;
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.predicate = (object.predicate !== undefined && object.predicate !== null)
      ? TestVerdictPredicate.fromPartial(object.predicate)
      : undefined;
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseQueryTestHistoryResponse(): QueryTestHistoryResponse {
  return { verdicts: [], nextPageToken: "" };
}

export const QueryTestHistoryResponse = {
  encode(message: QueryTestHistoryResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.verdicts) {
      TestVerdict.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestHistoryResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestHistoryResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.verdicts.push(TestVerdict.decode(reader, reader.uint32()));
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

  fromJSON(object: any): QueryTestHistoryResponse {
    return {
      verdicts: globalThis.Array.isArray(object?.verdicts)
        ? object.verdicts.map((e: any) => TestVerdict.fromJSON(e))
        : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: QueryTestHistoryResponse): unknown {
    const obj: any = {};
    if (message.verdicts?.length) {
      obj.verdicts = message.verdicts.map((e) => TestVerdict.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestHistoryResponse>, I>>(base?: I): QueryTestHistoryResponse {
    return QueryTestHistoryResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestHistoryResponse>, I>>(object: I): QueryTestHistoryResponse {
    const message = createBaseQueryTestHistoryResponse() as any;
    message.verdicts = object.verdicts?.map((e) => TestVerdict.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseQueryTestHistoryStatsRequest(): QueryTestHistoryStatsRequest {
  return { project: "", testId: "", predicate: undefined, pageSize: 0, pageToken: "" };
}

export const QueryTestHistoryStatsRequest = {
  encode(message: QueryTestHistoryStatsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(18).string(message.testId);
    }
    if (message.predicate !== undefined) {
      TestVerdictPredicate.encode(message.predicate, writer.uint32(26).fork()).ldelim();
    }
    if (message.pageSize !== 0) {
      writer.uint32(32).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(42).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestHistoryStatsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestHistoryStatsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.predicate = TestVerdictPredicate.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 5:
          if (tag !== 42) {
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

  fromJSON(object: any): QueryTestHistoryStatsRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      predicate: isSet(object.predicate) ? TestVerdictPredicate.fromJSON(object.predicate) : undefined,
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: QueryTestHistoryStatsRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.predicate !== undefined) {
      obj.predicate = TestVerdictPredicate.toJSON(message.predicate);
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestHistoryStatsRequest>, I>>(base?: I): QueryTestHistoryStatsRequest {
    return QueryTestHistoryStatsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestHistoryStatsRequest>, I>>(object: I): QueryTestHistoryStatsRequest {
    const message = createBaseQueryTestHistoryStatsRequest() as any;
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.predicate = (object.predicate !== undefined && object.predicate !== null)
      ? TestVerdictPredicate.fromPartial(object.predicate)
      : undefined;
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseQueryTestHistoryStatsResponse(): QueryTestHistoryStatsResponse {
  return { groups: [], nextPageToken: "" };
}

export const QueryTestHistoryStatsResponse = {
  encode(message: QueryTestHistoryStatsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.groups) {
      QueryTestHistoryStatsResponse_Group.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestHistoryStatsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestHistoryStatsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.groups.push(QueryTestHistoryStatsResponse_Group.decode(reader, reader.uint32()));
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

  fromJSON(object: any): QueryTestHistoryStatsResponse {
    return {
      groups: globalThis.Array.isArray(object?.groups)
        ? object.groups.map((e: any) => QueryTestHistoryStatsResponse_Group.fromJSON(e))
        : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: QueryTestHistoryStatsResponse): unknown {
    const obj: any = {};
    if (message.groups?.length) {
      obj.groups = message.groups.map((e) => QueryTestHistoryStatsResponse_Group.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestHistoryStatsResponse>, I>>(base?: I): QueryTestHistoryStatsResponse {
    return QueryTestHistoryStatsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestHistoryStatsResponse>, I>>(
    object: I,
  ): QueryTestHistoryStatsResponse {
    const message = createBaseQueryTestHistoryStatsResponse() as any;
    message.groups = object.groups?.map((e) => QueryTestHistoryStatsResponse_Group.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseQueryTestHistoryStatsResponse_Group(): QueryTestHistoryStatsResponse_Group {
  return {
    partitionTime: undefined,
    variantHash: "",
    unexpectedCount: 0,
    unexpectedlySkippedCount: 0,
    flakyCount: 0,
    exoneratedCount: 0,
    expectedCount: 0,
    passedAvgDuration: undefined,
  };
}

export const QueryTestHistoryStatsResponse_Group = {
  encode(message: QueryTestHistoryStatsResponse_Group, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.partitionTime !== undefined) {
      Timestamp.encode(toTimestamp(message.partitionTime), writer.uint32(10).fork()).ldelim();
    }
    if (message.variantHash !== "") {
      writer.uint32(18).string(message.variantHash);
    }
    if (message.unexpectedCount !== 0) {
      writer.uint32(24).int32(message.unexpectedCount);
    }
    if (message.unexpectedlySkippedCount !== 0) {
      writer.uint32(32).int32(message.unexpectedlySkippedCount);
    }
    if (message.flakyCount !== 0) {
      writer.uint32(40).int32(message.flakyCount);
    }
    if (message.exoneratedCount !== 0) {
      writer.uint32(48).int32(message.exoneratedCount);
    }
    if (message.expectedCount !== 0) {
      writer.uint32(56).int32(message.expectedCount);
    }
    if (message.passedAvgDuration !== undefined) {
      Duration.encode(message.passedAvgDuration, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestHistoryStatsResponse_Group {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestHistoryStatsResponse_Group() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.partitionTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.variantHash = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.unexpectedCount = reader.int32();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.unexpectedlySkippedCount = reader.int32();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.flakyCount = reader.int32();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.exoneratedCount = reader.int32();
          continue;
        case 7:
          if (tag !== 56) {
            break;
          }

          message.expectedCount = reader.int32();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.passedAvgDuration = Duration.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): QueryTestHistoryStatsResponse_Group {
    return {
      partitionTime: isSet(object.partitionTime) ? globalThis.String(object.partitionTime) : undefined,
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      unexpectedCount: isSet(object.unexpectedCount) ? globalThis.Number(object.unexpectedCount) : 0,
      unexpectedlySkippedCount: isSet(object.unexpectedlySkippedCount)
        ? globalThis.Number(object.unexpectedlySkippedCount)
        : 0,
      flakyCount: isSet(object.flakyCount) ? globalThis.Number(object.flakyCount) : 0,
      exoneratedCount: isSet(object.exoneratedCount) ? globalThis.Number(object.exoneratedCount) : 0,
      expectedCount: isSet(object.expectedCount) ? globalThis.Number(object.expectedCount) : 0,
      passedAvgDuration: isSet(object.passedAvgDuration) ? Duration.fromJSON(object.passedAvgDuration) : undefined,
    };
  },

  toJSON(message: QueryTestHistoryStatsResponse_Group): unknown {
    const obj: any = {};
    if (message.partitionTime !== undefined) {
      obj.partitionTime = message.partitionTime;
    }
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.unexpectedCount !== 0) {
      obj.unexpectedCount = Math.round(message.unexpectedCount);
    }
    if (message.unexpectedlySkippedCount !== 0) {
      obj.unexpectedlySkippedCount = Math.round(message.unexpectedlySkippedCount);
    }
    if (message.flakyCount !== 0) {
      obj.flakyCount = Math.round(message.flakyCount);
    }
    if (message.exoneratedCount !== 0) {
      obj.exoneratedCount = Math.round(message.exoneratedCount);
    }
    if (message.expectedCount !== 0) {
      obj.expectedCount = Math.round(message.expectedCount);
    }
    if (message.passedAvgDuration !== undefined) {
      obj.passedAvgDuration = Duration.toJSON(message.passedAvgDuration);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestHistoryStatsResponse_Group>, I>>(
    base?: I,
  ): QueryTestHistoryStatsResponse_Group {
    return QueryTestHistoryStatsResponse_Group.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestHistoryStatsResponse_Group>, I>>(
    object: I,
  ): QueryTestHistoryStatsResponse_Group {
    const message = createBaseQueryTestHistoryStatsResponse_Group() as any;
    message.partitionTime = object.partitionTime ?? undefined;
    message.variantHash = object.variantHash ?? "";
    message.unexpectedCount = object.unexpectedCount ?? 0;
    message.unexpectedlySkippedCount = object.unexpectedlySkippedCount ?? 0;
    message.flakyCount = object.flakyCount ?? 0;
    message.exoneratedCount = object.exoneratedCount ?? 0;
    message.expectedCount = object.expectedCount ?? 0;
    message.passedAvgDuration = (object.passedAvgDuration !== undefined && object.passedAvgDuration !== null)
      ? Duration.fromPartial(object.passedAvgDuration)
      : undefined;
    return message;
  },
};

function createBaseQueryVariantsRequest(): QueryVariantsRequest {
  return { project: "", testId: "", subRealm: "", variantPredicate: undefined, pageSize: 0, pageToken: "" };
}

export const QueryVariantsRequest = {
  encode(message: QueryVariantsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(18).string(message.testId);
    }
    if (message.subRealm !== "") {
      writer.uint32(26).string(message.subRealm);
    }
    if (message.variantPredicate !== undefined) {
      VariantPredicate.encode(message.variantPredicate, writer.uint32(50).fork()).ldelim();
    }
    if (message.pageSize !== 0) {
      writer.uint32(32).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(42).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryVariantsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryVariantsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.subRealm = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.variantPredicate = VariantPredicate.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 5:
          if (tag !== 42) {
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

  fromJSON(object: any): QueryVariantsRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      subRealm: isSet(object.subRealm) ? globalThis.String(object.subRealm) : "",
      variantPredicate: isSet(object.variantPredicate) ? VariantPredicate.fromJSON(object.variantPredicate) : undefined,
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: QueryVariantsRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.subRealm !== "") {
      obj.subRealm = message.subRealm;
    }
    if (message.variantPredicate !== undefined) {
      obj.variantPredicate = VariantPredicate.toJSON(message.variantPredicate);
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryVariantsRequest>, I>>(base?: I): QueryVariantsRequest {
    return QueryVariantsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryVariantsRequest>, I>>(object: I): QueryVariantsRequest {
    const message = createBaseQueryVariantsRequest() as any;
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.subRealm = object.subRealm ?? "";
    message.variantPredicate = (object.variantPredicate !== undefined && object.variantPredicate !== null)
      ? VariantPredicate.fromPartial(object.variantPredicate)
      : undefined;
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseQueryVariantsResponse(): QueryVariantsResponse {
  return { variants: [], nextPageToken: "" };
}

export const QueryVariantsResponse = {
  encode(message: QueryVariantsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.variants) {
      QueryVariantsResponse_VariantInfo.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryVariantsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryVariantsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.variants.push(QueryVariantsResponse_VariantInfo.decode(reader, reader.uint32()));
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

  fromJSON(object: any): QueryVariantsResponse {
    return {
      variants: globalThis.Array.isArray(object?.variants)
        ? object.variants.map((e: any) => QueryVariantsResponse_VariantInfo.fromJSON(e))
        : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: QueryVariantsResponse): unknown {
    const obj: any = {};
    if (message.variants?.length) {
      obj.variants = message.variants.map((e) => QueryVariantsResponse_VariantInfo.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryVariantsResponse>, I>>(base?: I): QueryVariantsResponse {
    return QueryVariantsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryVariantsResponse>, I>>(object: I): QueryVariantsResponse {
    const message = createBaseQueryVariantsResponse() as any;
    message.variants = object.variants?.map((e) => QueryVariantsResponse_VariantInfo.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseQueryVariantsResponse_VariantInfo(): QueryVariantsResponse_VariantInfo {
  return { variantHash: "", variant: undefined };
}

export const QueryVariantsResponse_VariantInfo = {
  encode(message: QueryVariantsResponse_VariantInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.variantHash !== "") {
      writer.uint32(10).string(message.variantHash);
    }
    if (message.variant !== undefined) {
      Variant.encode(message.variant, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryVariantsResponse_VariantInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryVariantsResponse_VariantInfo() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.variantHash = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.variant = Variant.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): QueryVariantsResponse_VariantInfo {
    return {
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      variant: isSet(object.variant) ? Variant.fromJSON(object.variant) : undefined,
    };
  },

  toJSON(message: QueryVariantsResponse_VariantInfo): unknown {
    const obj: any = {};
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.variant !== undefined) {
      obj.variant = Variant.toJSON(message.variant);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryVariantsResponse_VariantInfo>, I>>(
    base?: I,
  ): QueryVariantsResponse_VariantInfo {
    return QueryVariantsResponse_VariantInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryVariantsResponse_VariantInfo>, I>>(
    object: I,
  ): QueryVariantsResponse_VariantInfo {
    const message = createBaseQueryVariantsResponse_VariantInfo() as any;
    message.variantHash = object.variantHash ?? "";
    message.variant = (object.variant !== undefined && object.variant !== null)
      ? Variant.fromPartial(object.variant)
      : undefined;
    return message;
  },
};

function createBaseQueryTestsRequest(): QueryTestsRequest {
  return { project: "", testIdSubstring: "", subRealm: "", pageSize: 0, pageToken: "" };
}

export const QueryTestsRequest = {
  encode(message: QueryTestsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.testIdSubstring !== "") {
      writer.uint32(18).string(message.testIdSubstring);
    }
    if (message.subRealm !== "") {
      writer.uint32(26).string(message.subRealm);
    }
    if (message.pageSize !== 0) {
      writer.uint32(32).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(42).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.project = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.testIdSubstring = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.subRealm = reader.string();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 5:
          if (tag !== 42) {
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

  fromJSON(object: any): QueryTestsRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testIdSubstring: isSet(object.testIdSubstring) ? globalThis.String(object.testIdSubstring) : "",
      subRealm: isSet(object.subRealm) ? globalThis.String(object.subRealm) : "",
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: QueryTestsRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testIdSubstring !== "") {
      obj.testIdSubstring = message.testIdSubstring;
    }
    if (message.subRealm !== "") {
      obj.subRealm = message.subRealm;
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestsRequest>, I>>(base?: I): QueryTestsRequest {
    return QueryTestsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestsRequest>, I>>(object: I): QueryTestsRequest {
    const message = createBaseQueryTestsRequest() as any;
    message.project = object.project ?? "";
    message.testIdSubstring = object.testIdSubstring ?? "";
    message.subRealm = object.subRealm ?? "";
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseQueryTestsResponse(): QueryTestsResponse {
  return { testIds: [], nextPageToken: "" };
}

export const QueryTestsResponse = {
  encode(message: QueryTestsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testIds) {
      writer.uint32(10).string(v!);
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryTestsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryTestsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testIds.push(reader.string());
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

  fromJSON(object: any): QueryTestsResponse {
    return {
      testIds: globalThis.Array.isArray(object?.testIds) ? object.testIds.map((e: any) => globalThis.String(e)) : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: QueryTestsResponse): unknown {
    const obj: any = {};
    if (message.testIds?.length) {
      obj.testIds = message.testIds;
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryTestsResponse>, I>>(base?: I): QueryTestsResponse {
    return QueryTestsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryTestsResponse>, I>>(object: I): QueryTestsResponse {
    const message = createBaseQueryTestsResponse() as any;
    message.testIds = object.testIds?.map((e) => e) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

/** Provide methods to read test histories. */
export interface TestHistory {
  /**
   * Retrieves test verdicts for a given test ID in a given project and in a
   * given range of time.
   * Accepts a test variant predicate to filter the verdicts.
   */
  Query(request: QueryTestHistoryRequest): Promise<QueryTestHistoryResponse>;
  /**
   * Retrieves a summary of test verdicts for a given test ID in a given project
   * and in a given range of times.
   * Accepts a test variant predicate to filter the verdicts.
   */
  QueryStats(request: QueryTestHistoryStatsRequest): Promise<QueryTestHistoryStatsResponse>;
  /**
   * Retrieves variants for a given test ID in a given project that were
   * recorded in the past 90 days.
   */
  QueryVariants(request: QueryVariantsRequest): Promise<QueryVariantsResponse>;
  /**
   * Finds test IDs that contain the given substring in a given project that
   * were recorded in the past 90 days.
   */
  QueryTests(request: QueryTestsRequest): Promise<QueryTestsResponse>;
}

export const TestHistoryServiceName = "luci.analysis.v1.TestHistory";
export class TestHistoryClientImpl implements TestHistory {
  static readonly DEFAULT_SERVICE = TestHistoryServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || TestHistoryServiceName;
    this.rpc = rpc;
    this.Query = this.Query.bind(this);
    this.QueryStats = this.QueryStats.bind(this);
    this.QueryVariants = this.QueryVariants.bind(this);
    this.QueryTests = this.QueryTests.bind(this);
  }
  Query(request: QueryTestHistoryRequest): Promise<QueryTestHistoryResponse> {
    const data = QueryTestHistoryRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "Query", data);
    return promise.then((data) => QueryTestHistoryResponse.decode(_m0.Reader.create(data)));
  }

  QueryStats(request: QueryTestHistoryStatsRequest): Promise<QueryTestHistoryStatsResponse> {
    const data = QueryTestHistoryStatsRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "QueryStats", data);
    return promise.then((data) => QueryTestHistoryStatsResponse.decode(_m0.Reader.create(data)));
  }

  QueryVariants(request: QueryVariantsRequest): Promise<QueryVariantsResponse> {
    const data = QueryVariantsRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "QueryVariants", data);
    return promise.then((data) => QueryVariantsResponse.decode(_m0.Reader.create(data)));
  }

  QueryTests(request: QueryTestsRequest): Promise<QueryTestsResponse> {
    const data = QueryTestsRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "QueryTests", data);
    return promise.then((data) => QueryTestsResponse.decode(_m0.Reader.create(data)));
  }
}

interface Rpc {
  request(service: string, method: string, data: Uint8Array): Promise<Uint8Array>;
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