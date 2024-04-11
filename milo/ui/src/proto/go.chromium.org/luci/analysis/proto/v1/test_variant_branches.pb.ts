/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Any } from "../../../../../google/protobuf/any.pb";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { Commit } from "../../../common/proto/git/commit.pb";
import { Variant } from "./common.pb";
import { SourceRef } from "./sources.pb";
import { TestVerdict } from "./test_verdict.pb";

export const protobufPackage = "luci.analysis.v1";

/** A request message for `TestVariantBranches.Get` RPC. */
export interface GetRawTestVariantBranchRequest {
  /**
   * The name of the test variant branch.
   * It MUST be of the form projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}
   * where:
   * PROJECT is the LUCI Project of the test variant branch analysis.
   * URL_ESCAPED_TEST_ID is the test ID, escaped with
   * https://golang.org/pkg/net/url/#PathEscape. See also https://aip.dev/122.
   * VARIANT_HASH is the variant hash of the test variant analysis (16 lower-case-character hex string).
   * REF_HASH is the identity of the branch of the analysis. It is a 16 lower-case-character hex string.
   */
  readonly name: string;
}

/** Represents changepoint analysis raw data for a particular (project, test, variant, ref) in spanner. */
export interface TestVariantBranchRaw {
  /**
   * The name of the test variant branch.
   * Of the form projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}
   * where:
   * PROJECT is the LUCI Project of the test variant branch analysis.
   * URL_ESCAPED_TEST_ID is the test ID, escaped with
   * https://golang.org/pkg/net/url/#PathEscape. See also https://aip.dev/122.
   * VARIANT_HASH is the variant hash of the test variant analysis (16 lower-case-character hex string).
   * REF_HASH is the identity of the branch of the analysis. It is a 16 lower-case-character hex string.
   */
  readonly name: string;
  /** The LUCI Project. E.g. "chromium". */
  readonly project: string;
  /** The identity of the test. */
  readonly testId: string;
  /**
   * Hash of the variant, as 16 lowercase hexadecimal characters.
   * E.g. "96c68dc946ab4068".
   */
  readonly variantHash: string;
  /** Hash of the source branch, as 16 lowercase hexadecimal characters. */
  readonly refHash: string;
  /**
   * Describes one specific way of running the test, e.g. a specific bucket,
   * builder and a test suite.
   */
  readonly variant:
    | Variant
    | undefined;
  /** The branch in source control. */
  readonly ref:
    | SourceRef
    | undefined;
  /**
   * The finalized segments in the output buffer.
   *
   * Do not depend on this field. The internal protocol buffer stored in
   * Spanner is returned here for debug purposes only. We use
   * google.protobuf.Any to avoid revealing its type and having clients
   * possibly depend on it.
   *
   * If any tool needs to read this data, a wire proto (that is different
   * from the storage proto) needs to be defined and this field replaced
   * by a field of that wire type.
   */
  readonly finalizedSegments:
    | Any
    | undefined;
  /**
   * The finalizing segment in the output buffer.
   *
   * Do not depend on this field. The internal protocol buffer stored in
   * Spanner is returned here for debug purposes only. We use
   * google.protobuf.Any to avoid revealing its type and having clients
   * possibly depend on it.
   *
   * If any tool needs to read this data, a wire proto (that is different
   * from the storage proto) needs to be defined and this field replaced
   * by a field of that wire type.
   */
  readonly finalizingSegment:
    | Any
    | undefined;
  /**
   * Statistics about verdicts in the output buffer.
   *
   * Do not depend on this field. The internal protocol buffer stored in
   * Spanner is returned here for debug purposes only. We use
   * google.protobuf.Any to avoid revealing its type and having clients
   * possibly depend on it.
   *
   * If any tool needs to read this data, a wire proto (that is different
   * from the storage proto) needs to be defined and this field replaced
   * by a field of that wire type.
   */
  readonly statistics:
    | Any
    | undefined;
  /** The hot input buffer. */
  readonly hotBuffer:
    | InputBuffer
    | undefined;
  /** The cold input buffer. */
  readonly coldBuffer: InputBuffer | undefined;
}

/**
 * InputBuffer contains the verdict history of the test variant branch.
 * It is used for both the hot buffer and the cold buffer.
 */
export interface InputBuffer {
  /** The number of test verdicts in the input buffer. */
  readonly length: string;
  /**
   * Verdicts, sorted by commit position (oldest first), and
   * then result time (oldest first).
   */
  readonly verdicts: readonly PositionVerdict[];
}

/** PositionVerdict represents a test verdict at a commit position. */
export interface PositionVerdict {
  /** The commit position for the verdict. */
  readonly commitPosition: string;
  /** The time that this verdict is produced, truncated to the nearest hour. */
  readonly hour:
    | string
    | undefined;
  /** Whether the verdict is exonerated or not. */
  readonly isExonerated: boolean;
  readonly runs: readonly PositionVerdict_Run[];
}

export interface PositionVerdict_Run {
  /** Number of expectedly passed results in the run. */
  readonly expectedPassCount: string;
  /** Number of expectedly failed results in the run. */
  readonly expectedFailCount: string;
  /** Number of expectedly crashed results in the run. */
  readonly expectedCrashCount: string;
  /** Number of expectedly aborted results in the run. */
  readonly expectedAbortCount: string;
  /** Number of unexpectedly passed results in the run. */
  readonly unexpectedPassCount: string;
  /** Number of unexpectedly failed results in the run. */
  readonly unexpectedFailCount: string;
  /** Number of unexpectedly crashed results in the run. */
  readonly unexpectedCrashCount: string;
  /** Number of unexpectedly aborted results in the run. */
  readonly unexpectedAbortCount: string;
  /** Whether this run is a duplicate run. */
  readonly isDuplicate: boolean;
}

/** A request message for `TestVariantBranches.BatchGet` RPC. */
export interface BatchGetTestVariantBranchRequest {
  /**
   * The name of the test variant branch.
   * It MUST be of the form projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}
   * where:
   * PROJECT is the LUCI Project of the test variant branch analysis.
   * URL_ESCAPED_TEST_ID is the test ID, escaped with
   * https://golang.org/pkg/net/url/#PathEscape. See also https://aip.dev/122.
   * VARIANT_HASH is the variant hash of the test variant analysis (16 lower-case-character hex string).
   * REF_HASH is the identity of the branch of the analysis. It is a 16 lower-case-character hex string.
   * Maximum of 100 can be retrieved, otherwise this RPC will return error.
   */
  readonly names: readonly string[];
}

export interface BatchGetTestVariantBranchResponse {
  /**
   * The return list will have the same length and order as request names list.
   * If a record is not found, the corresponding element will be set to nil.
   */
  readonly testVariantBranches: readonly TestVariantBranch[];
}

/** Represents changepoint analysis for a particular (project, test, variant, ref). */
export interface TestVariantBranch {
  /**
   * The name of the test variant branch.
   * Of the form projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}
   * where:
   * PROJECT is the LUCI Project of the test variant branch analysis.
   * URL_ESCAPED_TEST_ID is the test ID, escaped with
   * https://golang.org/pkg/net/url/#PathEscape. See also https://aip.dev/122.
   * VARIANT_HASH is the variant hash of the test variant analysis (16 lower-case-character hex string).
   * REF_HASH is the identity of the branch of the analysis. It is a 16 lower-case-character hex string.
   */
  readonly name: string;
  /** The LUCI Project. E.g. "chromium". */
  readonly project: string;
  /** The identity of the test. */
  readonly testId: string;
  /**
   * Hash of the variant, as 16 lowercase hexadecimal characters.
   * E.g. "96c68dc946ab4068".
   */
  readonly variantHash: string;
  /** Hash of the source branch, as 16 lowercase hexadecimal characters. */
  readonly refHash: string;
  /**
   * key:value pairs to specify the way of running a particular test.
   * e.g. a specific bucket, builder and a test suite.
   */
  readonly variant:
    | Variant
    | undefined;
  /** The branch in source control. */
  readonly ref:
    | SourceRef
    | undefined;
  /**
   * The test history represented as a set of [start commit position,
   * end commit position] segments, where segments have statistically
   * different failure and/or flake rates. The segments are ordered so that
   * the most recent segment appears first.
   * If a client is only interested in the current failure/flake rate, they
   * can just query the first segment.
   */
  readonly segments: readonly Segment[];
}

/**
 * Represents a period in history where the test had a consistent failure and
 * flake rate. Segments are separated by changepoints. Each segment captures
 * information about the changepoint which started it.
 * Same structure with bigquery proto here, but make a separate copy to allow it to evolve over time.
 * https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/bq/test_variant_branch_row.proto;l=80
 */
export interface Segment {
  /**
   * If set, means the segment commenced with a changepoint.
   * If unset, means the segment began with the beginning of recorded
   * history for the segment. (All recorded history for a test variant branch
   * is deleted after 90 days of no results, so this means there were
   * no results for at least 90 days before the segment.)
   */
  readonly hasStartChangepoint: boolean;
  /**
   * The nominal commit position at which the segment starts (inclusive).
   * Guaranteed to be strictly greater than the end_position of the
   * chronologically previous segment (if any).
   * If this segment has a starting changepoint, this is the nominal position
   * of the changepoint (when the new test behaviour started).
   * If this segment does not have a starting changepoint, this is the
   * simply the first commit position in the known history of the test.
   */
  readonly startPosition: string;
  /**
   * The lower bound of the starting changepoint position in a 99% two-tailed
   * confidence interval. Inclusive.
   * Only set if has_start_changepoint is set.
   */
  readonly startPositionLowerBound99th: string;
  /**
   * The upper bound of the starting changepoint position in a 99% two-tailed
   * confidence interval. Inclusive.
   * Only set if has_start_changepoint is set.
   * When has_start_changepoint is set, the following invariant holds:
   * previous_segment.start_position <= start_position_lower_bound_99th <= start_position <= start_position_upper_bound_99th
   * where previous_segment refers to the chronologically previous segment.
   */
  readonly startPositionUpperBound99th: string;
  /**
   * The earliest hour a test verdict at the indicated start_position
   * was recorded. Gives an approximate upper bound on the timestamp the
   * changepoint occurred, for systems which need to filter by date.
   */
  readonly startHour:
    | string
    | undefined;
  /**
   * The nominal commit position at which the segment ends (inclusive).
   * This is either the last recorded commit position in the test history
   * (for this test variant branch), or the position of the last verdict
   * seen before the next detected changepoint.
   */
  readonly endPosition: string;
  /**
   * The earliest hour a test verdict at the indicated end_position
   * was recorded. Gives an approximate lower bound on the  timestamp
   * the changepoint occurred, for systems which need to filter by date.
   */
  readonly endHour:
    | string
    | undefined;
  /** Total number of test results/runs/verdicts in the segment. */
  readonly counts: Segment_Counts | undefined;
}

/**
 * Counts of verdicts over a time period. Includes only
 * test verdicts for submitted code changes. This is defined as:
 * (1) where the code under test was already submitted when the test ran
 *       (e.g. postsubmit builders)
 * (2) where the code under test was not submitted at the time the test ran,
 *     but was submitted immediately after (e.g. because the tests ran as part
 *     of a tryjob, the presubmit run the tryjob was triggered by succeeded,
 *     and submitted code as a result).
 *     Currently, when test results lead to CL submission via recycled CQ runs,
 *     they are not counted.
 * Statistics for test results and test runs can be added here when needed.
 */
export interface Segment_Counts {
  /** The number of verdicts with only unexpected test results. */
  readonly unexpectedVerdicts: number;
  /** The number of verdicts with a mix of expected and unexpected test results. */
  readonly flakyVerdicts: number;
  /** The total number of verdicts. */
  readonly totalVerdicts: number;
}

export interface QuerySourcePositionsRequest {
  /** The LUCI project. */
  readonly project: string;
  /** The identifier of a test. */
  readonly testId: string;
  /** The hash of the variant. */
  readonly variantHash: string;
  /** Hash of the source branch, as 16 lowercase hexadecimal characters. */
  readonly refHash: string;
  /**
   * The source position where to start listing from, in descending order (newest commit to older commits).
   * This start source position will be the largest source position in the response.
   */
  readonly startSourcePosition: string;
  /**
   * The maximum number of commits to return.
   *
   * The service may return fewer than this value.
   * If unspecified, at most 100 commits will be returned.
   * The maximum value is 1,000; values above 1,000 will be coerced to 1,000.
   */
  readonly pageSize: number;
  /**
   * A page token, received from a previous `QuerySourcePositions` call.
   * Provide this to retrieve the subsequent page.
   *
   * When paginating, all other parameters provided to `QuerySourcePositions` MUST
   * match the call that provided the page token.
   */
  readonly pageToken: string;
}

export interface QuerySourcePositionsResponse {
  /** Source positions in descending order, start from the commit at start_source_position. */
  readonly sourcePositions: readonly SourcePosition[];
  /** A page token for next QuerySourcePositionsRequest to fetch the next page of commits. */
  readonly nextPageToken: string;
}

/** SourcePosition contains the commit and the test verdicts at a source position for a test variant branch. */
export interface SourcePosition {
  /** Source position. */
  readonly position: string;
  /** The git commit. */
  readonly commit:
    | Commit
    | undefined;
  /**
   * Test verdicts at this source position.
   * Test verdicts will be ordered by `partition_time` DESC.
   * At most 20 verdicts will be returned here.
   * Most of time, a test variant at the same source position has less than 20 verdicts.
   */
  readonly verdicts: readonly TestVerdict[];
}

function createBaseGetRawTestVariantBranchRequest(): GetRawTestVariantBranchRequest {
  return { name: "" };
}

export const GetRawTestVariantBranchRequest = {
  encode(message: GetRawTestVariantBranchRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetRawTestVariantBranchRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetRawTestVariantBranchRequest() as any;
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

  fromJSON(object: any): GetRawTestVariantBranchRequest {
    return { name: isSet(object.name) ? globalThis.String(object.name) : "" };
  },

  toJSON(message: GetRawTestVariantBranchRequest): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetRawTestVariantBranchRequest>, I>>(base?: I): GetRawTestVariantBranchRequest {
    return GetRawTestVariantBranchRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetRawTestVariantBranchRequest>, I>>(
    object: I,
  ): GetRawTestVariantBranchRequest {
    const message = createBaseGetRawTestVariantBranchRequest() as any;
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseTestVariantBranchRaw(): TestVariantBranchRaw {
  return {
    name: "",
    project: "",
    testId: "",
    variantHash: "",
    refHash: "",
    variant: undefined,
    ref: undefined,
    finalizedSegments: undefined,
    finalizingSegment: undefined,
    statistics: undefined,
    hotBuffer: undefined,
    coldBuffer: undefined,
  };
}

export const TestVariantBranchRaw = {
  encode(message: TestVariantBranchRaw, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(26).string(message.testId);
    }
    if (message.variantHash !== "") {
      writer.uint32(34).string(message.variantHash);
    }
    if (message.refHash !== "") {
      writer.uint32(42).string(message.refHash);
    }
    if (message.variant !== undefined) {
      Variant.encode(message.variant, writer.uint32(50).fork()).ldelim();
    }
    if (message.ref !== undefined) {
      SourceRef.encode(message.ref, writer.uint32(58).fork()).ldelim();
    }
    if (message.finalizedSegments !== undefined) {
      Any.encode(message.finalizedSegments, writer.uint32(66).fork()).ldelim();
    }
    if (message.finalizingSegment !== undefined) {
      Any.encode(message.finalizingSegment, writer.uint32(74).fork()).ldelim();
    }
    if (message.statistics !== undefined) {
      Any.encode(message.statistics, writer.uint32(98).fork()).ldelim();
    }
    if (message.hotBuffer !== undefined) {
      InputBuffer.encode(message.hotBuffer, writer.uint32(82).fork()).ldelim();
    }
    if (message.coldBuffer !== undefined) {
      InputBuffer.encode(message.coldBuffer, writer.uint32(90).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestVariantBranchRaw {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestVariantBranchRaw() as any;
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

          message.project = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.variantHash = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.refHash = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.variant = Variant.decode(reader, reader.uint32());
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.ref = SourceRef.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.finalizedSegments = Any.decode(reader, reader.uint32());
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.finalizingSegment = Any.decode(reader, reader.uint32());
          continue;
        case 12:
          if (tag !== 98) {
            break;
          }

          message.statistics = Any.decode(reader, reader.uint32());
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.hotBuffer = InputBuffer.decode(reader, reader.uint32());
          continue;
        case 11:
          if (tag !== 90) {
            break;
          }

          message.coldBuffer = InputBuffer.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestVariantBranchRaw {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      refHash: isSet(object.refHash) ? globalThis.String(object.refHash) : "",
      variant: isSet(object.variant) ? Variant.fromJSON(object.variant) : undefined,
      ref: isSet(object.ref) ? SourceRef.fromJSON(object.ref) : undefined,
      finalizedSegments: isSet(object.finalizedSegments) ? Any.fromJSON(object.finalizedSegments) : undefined,
      finalizingSegment: isSet(object.finalizingSegment) ? Any.fromJSON(object.finalizingSegment) : undefined,
      statistics: isSet(object.statistics) ? Any.fromJSON(object.statistics) : undefined,
      hotBuffer: isSet(object.hotBuffer) ? InputBuffer.fromJSON(object.hotBuffer) : undefined,
      coldBuffer: isSet(object.coldBuffer) ? InputBuffer.fromJSON(object.coldBuffer) : undefined,
    };
  },

  toJSON(message: TestVariantBranchRaw): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.refHash !== "") {
      obj.refHash = message.refHash;
    }
    if (message.variant !== undefined) {
      obj.variant = Variant.toJSON(message.variant);
    }
    if (message.ref !== undefined) {
      obj.ref = SourceRef.toJSON(message.ref);
    }
    if (message.finalizedSegments !== undefined) {
      obj.finalizedSegments = Any.toJSON(message.finalizedSegments);
    }
    if (message.finalizingSegment !== undefined) {
      obj.finalizingSegment = Any.toJSON(message.finalizingSegment);
    }
    if (message.statistics !== undefined) {
      obj.statistics = Any.toJSON(message.statistics);
    }
    if (message.hotBuffer !== undefined) {
      obj.hotBuffer = InputBuffer.toJSON(message.hotBuffer);
    }
    if (message.coldBuffer !== undefined) {
      obj.coldBuffer = InputBuffer.toJSON(message.coldBuffer);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestVariantBranchRaw>, I>>(base?: I): TestVariantBranchRaw {
    return TestVariantBranchRaw.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestVariantBranchRaw>, I>>(object: I): TestVariantBranchRaw {
    const message = createBaseTestVariantBranchRaw() as any;
    message.name = object.name ?? "";
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.variantHash = object.variantHash ?? "";
    message.refHash = object.refHash ?? "";
    message.variant = (object.variant !== undefined && object.variant !== null)
      ? Variant.fromPartial(object.variant)
      : undefined;
    message.ref = (object.ref !== undefined && object.ref !== null) ? SourceRef.fromPartial(object.ref) : undefined;
    message.finalizedSegments = (object.finalizedSegments !== undefined && object.finalizedSegments !== null)
      ? Any.fromPartial(object.finalizedSegments)
      : undefined;
    message.finalizingSegment = (object.finalizingSegment !== undefined && object.finalizingSegment !== null)
      ? Any.fromPartial(object.finalizingSegment)
      : undefined;
    message.statistics = (object.statistics !== undefined && object.statistics !== null)
      ? Any.fromPartial(object.statistics)
      : undefined;
    message.hotBuffer = (object.hotBuffer !== undefined && object.hotBuffer !== null)
      ? InputBuffer.fromPartial(object.hotBuffer)
      : undefined;
    message.coldBuffer = (object.coldBuffer !== undefined && object.coldBuffer !== null)
      ? InputBuffer.fromPartial(object.coldBuffer)
      : undefined;
    return message;
  },
};

function createBaseInputBuffer(): InputBuffer {
  return { length: "0", verdicts: [] };
}

export const InputBuffer = {
  encode(message: InputBuffer, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.length !== "0") {
      writer.uint32(8).int64(message.length);
    }
    for (const v of message.verdicts) {
      PositionVerdict.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputBuffer {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputBuffer() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.length = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.verdicts.push(PositionVerdict.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InputBuffer {
    return {
      length: isSet(object.length) ? globalThis.String(object.length) : "0",
      verdicts: globalThis.Array.isArray(object?.verdicts)
        ? object.verdicts.map((e: any) => PositionVerdict.fromJSON(e))
        : [],
    };
  },

  toJSON(message: InputBuffer): unknown {
    const obj: any = {};
    if (message.length !== "0") {
      obj.length = message.length;
    }
    if (message.verdicts?.length) {
      obj.verdicts = message.verdicts.map((e) => PositionVerdict.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputBuffer>, I>>(base?: I): InputBuffer {
    return InputBuffer.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputBuffer>, I>>(object: I): InputBuffer {
    const message = createBaseInputBuffer() as any;
    message.length = object.length ?? "0";
    message.verdicts = object.verdicts?.map((e) => PositionVerdict.fromPartial(e)) || [];
    return message;
  },
};

function createBasePositionVerdict(): PositionVerdict {
  return { commitPosition: "0", hour: undefined, isExonerated: false, runs: [] };
}

export const PositionVerdict = {
  encode(message: PositionVerdict, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.commitPosition !== "0") {
      writer.uint32(8).int64(message.commitPosition);
    }
    if (message.hour !== undefined) {
      Timestamp.encode(toTimestamp(message.hour), writer.uint32(18).fork()).ldelim();
    }
    if (message.isExonerated === true) {
      writer.uint32(24).bool(message.isExonerated);
    }
    for (const v of message.runs) {
      PositionVerdict_Run.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PositionVerdict {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePositionVerdict() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.commitPosition = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.hour = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.isExonerated = reader.bool();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.runs.push(PositionVerdict_Run.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): PositionVerdict {
    return {
      commitPosition: isSet(object.commitPosition) ? globalThis.String(object.commitPosition) : "0",
      hour: isSet(object.hour) ? globalThis.String(object.hour) : undefined,
      isExonerated: isSet(object.isExonerated) ? globalThis.Boolean(object.isExonerated) : false,
      runs: globalThis.Array.isArray(object?.runs) ? object.runs.map((e: any) => PositionVerdict_Run.fromJSON(e)) : [],
    };
  },

  toJSON(message: PositionVerdict): unknown {
    const obj: any = {};
    if (message.commitPosition !== "0") {
      obj.commitPosition = message.commitPosition;
    }
    if (message.hour !== undefined) {
      obj.hour = message.hour;
    }
    if (message.isExonerated === true) {
      obj.isExonerated = message.isExonerated;
    }
    if (message.runs?.length) {
      obj.runs = message.runs.map((e) => PositionVerdict_Run.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PositionVerdict>, I>>(base?: I): PositionVerdict {
    return PositionVerdict.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<PositionVerdict>, I>>(object: I): PositionVerdict {
    const message = createBasePositionVerdict() as any;
    message.commitPosition = object.commitPosition ?? "0";
    message.hour = object.hour ?? undefined;
    message.isExonerated = object.isExonerated ?? false;
    message.runs = object.runs?.map((e) => PositionVerdict_Run.fromPartial(e)) || [];
    return message;
  },
};

function createBasePositionVerdict_Run(): PositionVerdict_Run {
  return {
    expectedPassCount: "0",
    expectedFailCount: "0",
    expectedCrashCount: "0",
    expectedAbortCount: "0",
    unexpectedPassCount: "0",
    unexpectedFailCount: "0",
    unexpectedCrashCount: "0",
    unexpectedAbortCount: "0",
    isDuplicate: false,
  };
}

export const PositionVerdict_Run = {
  encode(message: PositionVerdict_Run, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.expectedPassCount !== "0") {
      writer.uint32(8).int64(message.expectedPassCount);
    }
    if (message.expectedFailCount !== "0") {
      writer.uint32(16).int64(message.expectedFailCount);
    }
    if (message.expectedCrashCount !== "0") {
      writer.uint32(24).int64(message.expectedCrashCount);
    }
    if (message.expectedAbortCount !== "0") {
      writer.uint32(32).int64(message.expectedAbortCount);
    }
    if (message.unexpectedPassCount !== "0") {
      writer.uint32(40).int64(message.unexpectedPassCount);
    }
    if (message.unexpectedFailCount !== "0") {
      writer.uint32(48).int64(message.unexpectedFailCount);
    }
    if (message.unexpectedCrashCount !== "0") {
      writer.uint32(56).int64(message.unexpectedCrashCount);
    }
    if (message.unexpectedAbortCount !== "0") {
      writer.uint32(64).int64(message.unexpectedAbortCount);
    }
    if (message.isDuplicate === true) {
      writer.uint32(72).bool(message.isDuplicate);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PositionVerdict_Run {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePositionVerdict_Run() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.expectedPassCount = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.expectedFailCount = longToString(reader.int64() as Long);
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.expectedCrashCount = longToString(reader.int64() as Long);
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.expectedAbortCount = longToString(reader.int64() as Long);
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.unexpectedPassCount = longToString(reader.int64() as Long);
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.unexpectedFailCount = longToString(reader.int64() as Long);
          continue;
        case 7:
          if (tag !== 56) {
            break;
          }

          message.unexpectedCrashCount = longToString(reader.int64() as Long);
          continue;
        case 8:
          if (tag !== 64) {
            break;
          }

          message.unexpectedAbortCount = longToString(reader.int64() as Long);
          continue;
        case 9:
          if (tag !== 72) {
            break;
          }

          message.isDuplicate = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): PositionVerdict_Run {
    return {
      expectedPassCount: isSet(object.expectedPassCount) ? globalThis.String(object.expectedPassCount) : "0",
      expectedFailCount: isSet(object.expectedFailCount) ? globalThis.String(object.expectedFailCount) : "0",
      expectedCrashCount: isSet(object.expectedCrashCount) ? globalThis.String(object.expectedCrashCount) : "0",
      expectedAbortCount: isSet(object.expectedAbortCount) ? globalThis.String(object.expectedAbortCount) : "0",
      unexpectedPassCount: isSet(object.unexpectedPassCount) ? globalThis.String(object.unexpectedPassCount) : "0",
      unexpectedFailCount: isSet(object.unexpectedFailCount) ? globalThis.String(object.unexpectedFailCount) : "0",
      unexpectedCrashCount: isSet(object.unexpectedCrashCount) ? globalThis.String(object.unexpectedCrashCount) : "0",
      unexpectedAbortCount: isSet(object.unexpectedAbortCount) ? globalThis.String(object.unexpectedAbortCount) : "0",
      isDuplicate: isSet(object.isDuplicate) ? globalThis.Boolean(object.isDuplicate) : false,
    };
  },

  toJSON(message: PositionVerdict_Run): unknown {
    const obj: any = {};
    if (message.expectedPassCount !== "0") {
      obj.expectedPassCount = message.expectedPassCount;
    }
    if (message.expectedFailCount !== "0") {
      obj.expectedFailCount = message.expectedFailCount;
    }
    if (message.expectedCrashCount !== "0") {
      obj.expectedCrashCount = message.expectedCrashCount;
    }
    if (message.expectedAbortCount !== "0") {
      obj.expectedAbortCount = message.expectedAbortCount;
    }
    if (message.unexpectedPassCount !== "0") {
      obj.unexpectedPassCount = message.unexpectedPassCount;
    }
    if (message.unexpectedFailCount !== "0") {
      obj.unexpectedFailCount = message.unexpectedFailCount;
    }
    if (message.unexpectedCrashCount !== "0") {
      obj.unexpectedCrashCount = message.unexpectedCrashCount;
    }
    if (message.unexpectedAbortCount !== "0") {
      obj.unexpectedAbortCount = message.unexpectedAbortCount;
    }
    if (message.isDuplicate === true) {
      obj.isDuplicate = message.isDuplicate;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PositionVerdict_Run>, I>>(base?: I): PositionVerdict_Run {
    return PositionVerdict_Run.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<PositionVerdict_Run>, I>>(object: I): PositionVerdict_Run {
    const message = createBasePositionVerdict_Run() as any;
    message.expectedPassCount = object.expectedPassCount ?? "0";
    message.expectedFailCount = object.expectedFailCount ?? "0";
    message.expectedCrashCount = object.expectedCrashCount ?? "0";
    message.expectedAbortCount = object.expectedAbortCount ?? "0";
    message.unexpectedPassCount = object.unexpectedPassCount ?? "0";
    message.unexpectedFailCount = object.unexpectedFailCount ?? "0";
    message.unexpectedCrashCount = object.unexpectedCrashCount ?? "0";
    message.unexpectedAbortCount = object.unexpectedAbortCount ?? "0";
    message.isDuplicate = object.isDuplicate ?? false;
    return message;
  },
};

function createBaseBatchGetTestVariantBranchRequest(): BatchGetTestVariantBranchRequest {
  return { names: [] };
}

export const BatchGetTestVariantBranchRequest = {
  encode(message: BatchGetTestVariantBranchRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.names) {
      writer.uint32(10).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetTestVariantBranchRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetTestVariantBranchRequest() as any;
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

  fromJSON(object: any): BatchGetTestVariantBranchRequest {
    return { names: globalThis.Array.isArray(object?.names) ? object.names.map((e: any) => globalThis.String(e)) : [] };
  },

  toJSON(message: BatchGetTestVariantBranchRequest): unknown {
    const obj: any = {};
    if (message.names?.length) {
      obj.names = message.names;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetTestVariantBranchRequest>, I>>(
    base?: I,
  ): BatchGetTestVariantBranchRequest {
    return BatchGetTestVariantBranchRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetTestVariantBranchRequest>, I>>(
    object: I,
  ): BatchGetTestVariantBranchRequest {
    const message = createBaseBatchGetTestVariantBranchRequest() as any;
    message.names = object.names?.map((e) => e) || [];
    return message;
  },
};

function createBaseBatchGetTestVariantBranchResponse(): BatchGetTestVariantBranchResponse {
  return { testVariantBranches: [] };
}

export const BatchGetTestVariantBranchResponse = {
  encode(message: BatchGetTestVariantBranchResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testVariantBranches) {
      TestVariantBranch.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetTestVariantBranchResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetTestVariantBranchResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testVariantBranches.push(TestVariantBranch.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchGetTestVariantBranchResponse {
    return {
      testVariantBranches: globalThis.Array.isArray(object?.testVariantBranches)
        ? object.testVariantBranches.map((e: any) => TestVariantBranch.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BatchGetTestVariantBranchResponse): unknown {
    const obj: any = {};
    if (message.testVariantBranches?.length) {
      obj.testVariantBranches = message.testVariantBranches.map((e) => TestVariantBranch.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetTestVariantBranchResponse>, I>>(
    base?: I,
  ): BatchGetTestVariantBranchResponse {
    return BatchGetTestVariantBranchResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetTestVariantBranchResponse>, I>>(
    object: I,
  ): BatchGetTestVariantBranchResponse {
    const message = createBaseBatchGetTestVariantBranchResponse() as any;
    message.testVariantBranches = object.testVariantBranches?.map((e) => TestVariantBranch.fromPartial(e)) || [];
    return message;
  },
};

function createBaseTestVariantBranch(): TestVariantBranch {
  return {
    name: "",
    project: "",
    testId: "",
    variantHash: "",
    refHash: "",
    variant: undefined,
    ref: undefined,
    segments: [],
  };
}

export const TestVariantBranch = {
  encode(message: TestVariantBranch, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(26).string(message.testId);
    }
    if (message.variantHash !== "") {
      writer.uint32(34).string(message.variantHash);
    }
    if (message.refHash !== "") {
      writer.uint32(42).string(message.refHash);
    }
    if (message.variant !== undefined) {
      Variant.encode(message.variant, writer.uint32(50).fork()).ldelim();
    }
    if (message.ref !== undefined) {
      SourceRef.encode(message.ref, writer.uint32(58).fork()).ldelim();
    }
    for (const v of message.segments) {
      Segment.encode(v!, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestVariantBranch {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestVariantBranch() as any;
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

          message.project = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.variantHash = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.refHash = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.variant = Variant.decode(reader, reader.uint32());
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.ref = SourceRef.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.segments.push(Segment.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestVariantBranch {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      refHash: isSet(object.refHash) ? globalThis.String(object.refHash) : "",
      variant: isSet(object.variant) ? Variant.fromJSON(object.variant) : undefined,
      ref: isSet(object.ref) ? SourceRef.fromJSON(object.ref) : undefined,
      segments: globalThis.Array.isArray(object?.segments) ? object.segments.map((e: any) => Segment.fromJSON(e)) : [],
    };
  },

  toJSON(message: TestVariantBranch): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.refHash !== "") {
      obj.refHash = message.refHash;
    }
    if (message.variant !== undefined) {
      obj.variant = Variant.toJSON(message.variant);
    }
    if (message.ref !== undefined) {
      obj.ref = SourceRef.toJSON(message.ref);
    }
    if (message.segments?.length) {
      obj.segments = message.segments.map((e) => Segment.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestVariantBranch>, I>>(base?: I): TestVariantBranch {
    return TestVariantBranch.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestVariantBranch>, I>>(object: I): TestVariantBranch {
    const message = createBaseTestVariantBranch() as any;
    message.name = object.name ?? "";
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.variantHash = object.variantHash ?? "";
    message.refHash = object.refHash ?? "";
    message.variant = (object.variant !== undefined && object.variant !== null)
      ? Variant.fromPartial(object.variant)
      : undefined;
    message.ref = (object.ref !== undefined && object.ref !== null) ? SourceRef.fromPartial(object.ref) : undefined;
    message.segments = object.segments?.map((e) => Segment.fromPartial(e)) || [];
    return message;
  },
};

function createBaseSegment(): Segment {
  return {
    hasStartChangepoint: false,
    startPosition: "0",
    startPositionLowerBound99th: "0",
    startPositionUpperBound99th: "0",
    startHour: undefined,
    endPosition: "0",
    endHour: undefined,
    counts: undefined,
  };
}

export const Segment = {
  encode(message: Segment, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hasStartChangepoint === true) {
      writer.uint32(8).bool(message.hasStartChangepoint);
    }
    if (message.startPosition !== "0") {
      writer.uint32(16).int64(message.startPosition);
    }
    if (message.startPositionLowerBound99th !== "0") {
      writer.uint32(24).int64(message.startPositionLowerBound99th);
    }
    if (message.startPositionUpperBound99th !== "0") {
      writer.uint32(32).int64(message.startPositionUpperBound99th);
    }
    if (message.startHour !== undefined) {
      Timestamp.encode(toTimestamp(message.startHour), writer.uint32(42).fork()).ldelim();
    }
    if (message.endPosition !== "0") {
      writer.uint32(48).int64(message.endPosition);
    }
    if (message.endHour !== undefined) {
      Timestamp.encode(toTimestamp(message.endHour), writer.uint32(58).fork()).ldelim();
    }
    if (message.counts !== undefined) {
      Segment_Counts.encode(message.counts, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Segment {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSegment() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.hasStartChangepoint = reader.bool();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.startPosition = longToString(reader.int64() as Long);
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.startPositionLowerBound99th = longToString(reader.int64() as Long);
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.startPositionUpperBound99th = longToString(reader.int64() as Long);
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.startHour = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.endPosition = longToString(reader.int64() as Long);
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.endHour = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.counts = Segment_Counts.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Segment {
    return {
      hasStartChangepoint: isSet(object.hasStartChangepoint) ? globalThis.Boolean(object.hasStartChangepoint) : false,
      startPosition: isSet(object.startPosition) ? globalThis.String(object.startPosition) : "0",
      startPositionLowerBound99th: isSet(object.startPositionLowerBound99th)
        ? globalThis.String(object.startPositionLowerBound99th)
        : "0",
      startPositionUpperBound99th: isSet(object.startPositionUpperBound99th)
        ? globalThis.String(object.startPositionUpperBound99th)
        : "0",
      startHour: isSet(object.startHour) ? globalThis.String(object.startHour) : undefined,
      endPosition: isSet(object.endPosition) ? globalThis.String(object.endPosition) : "0",
      endHour: isSet(object.endHour) ? globalThis.String(object.endHour) : undefined,
      counts: isSet(object.counts) ? Segment_Counts.fromJSON(object.counts) : undefined,
    };
  },

  toJSON(message: Segment): unknown {
    const obj: any = {};
    if (message.hasStartChangepoint === true) {
      obj.hasStartChangepoint = message.hasStartChangepoint;
    }
    if (message.startPosition !== "0") {
      obj.startPosition = message.startPosition;
    }
    if (message.startPositionLowerBound99th !== "0") {
      obj.startPositionLowerBound99th = message.startPositionLowerBound99th;
    }
    if (message.startPositionUpperBound99th !== "0") {
      obj.startPositionUpperBound99th = message.startPositionUpperBound99th;
    }
    if (message.startHour !== undefined) {
      obj.startHour = message.startHour;
    }
    if (message.endPosition !== "0") {
      obj.endPosition = message.endPosition;
    }
    if (message.endHour !== undefined) {
      obj.endHour = message.endHour;
    }
    if (message.counts !== undefined) {
      obj.counts = Segment_Counts.toJSON(message.counts);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Segment>, I>>(base?: I): Segment {
    return Segment.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Segment>, I>>(object: I): Segment {
    const message = createBaseSegment() as any;
    message.hasStartChangepoint = object.hasStartChangepoint ?? false;
    message.startPosition = object.startPosition ?? "0";
    message.startPositionLowerBound99th = object.startPositionLowerBound99th ?? "0";
    message.startPositionUpperBound99th = object.startPositionUpperBound99th ?? "0";
    message.startHour = object.startHour ?? undefined;
    message.endPosition = object.endPosition ?? "0";
    message.endHour = object.endHour ?? undefined;
    message.counts = (object.counts !== undefined && object.counts !== null)
      ? Segment_Counts.fromPartial(object.counts)
      : undefined;
    return message;
  },
};

function createBaseSegment_Counts(): Segment_Counts {
  return { unexpectedVerdicts: 0, flakyVerdicts: 0, totalVerdicts: 0 };
}

export const Segment_Counts = {
  encode(message: Segment_Counts, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.unexpectedVerdicts !== 0) {
      writer.uint32(8).int32(message.unexpectedVerdicts);
    }
    if (message.flakyVerdicts !== 0) {
      writer.uint32(16).int32(message.flakyVerdicts);
    }
    if (message.totalVerdicts !== 0) {
      writer.uint32(24).int32(message.totalVerdicts);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Segment_Counts {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSegment_Counts() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.unexpectedVerdicts = reader.int32();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.flakyVerdicts = reader.int32();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.totalVerdicts = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Segment_Counts {
    return {
      unexpectedVerdicts: isSet(object.unexpectedVerdicts) ? globalThis.Number(object.unexpectedVerdicts) : 0,
      flakyVerdicts: isSet(object.flakyVerdicts) ? globalThis.Number(object.flakyVerdicts) : 0,
      totalVerdicts: isSet(object.totalVerdicts) ? globalThis.Number(object.totalVerdicts) : 0,
    };
  },

  toJSON(message: Segment_Counts): unknown {
    const obj: any = {};
    if (message.unexpectedVerdicts !== 0) {
      obj.unexpectedVerdicts = Math.round(message.unexpectedVerdicts);
    }
    if (message.flakyVerdicts !== 0) {
      obj.flakyVerdicts = Math.round(message.flakyVerdicts);
    }
    if (message.totalVerdicts !== 0) {
      obj.totalVerdicts = Math.round(message.totalVerdicts);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Segment_Counts>, I>>(base?: I): Segment_Counts {
    return Segment_Counts.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Segment_Counts>, I>>(object: I): Segment_Counts {
    const message = createBaseSegment_Counts() as any;
    message.unexpectedVerdicts = object.unexpectedVerdicts ?? 0;
    message.flakyVerdicts = object.flakyVerdicts ?? 0;
    message.totalVerdicts = object.totalVerdicts ?? 0;
    return message;
  },
};

function createBaseQuerySourcePositionsRequest(): QuerySourcePositionsRequest {
  return {
    project: "",
    testId: "",
    variantHash: "",
    refHash: "",
    startSourcePosition: "0",
    pageSize: 0,
    pageToken: "",
  };
}

export const QuerySourcePositionsRequest = {
  encode(message: QuerySourcePositionsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.testId !== "") {
      writer.uint32(18).string(message.testId);
    }
    if (message.variantHash !== "") {
      writer.uint32(26).string(message.variantHash);
    }
    if (message.refHash !== "") {
      writer.uint32(34).string(message.refHash);
    }
    if (message.startSourcePosition !== "0") {
      writer.uint32(40).int64(message.startSourcePosition);
    }
    if (message.pageSize !== 0) {
      writer.uint32(48).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(58).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QuerySourcePositionsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQuerySourcePositionsRequest() as any;
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

          message.variantHash = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.refHash = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.startSourcePosition = longToString(reader.int64() as Long);
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 7:
          if (tag !== 58) {
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

  fromJSON(object: any): QuerySourcePositionsRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      refHash: isSet(object.refHash) ? globalThis.String(object.refHash) : "",
      startSourcePosition: isSet(object.startSourcePosition) ? globalThis.String(object.startSourcePosition) : "0",
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: QuerySourcePositionsRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.refHash !== "") {
      obj.refHash = message.refHash;
    }
    if (message.startSourcePosition !== "0") {
      obj.startSourcePosition = message.startSourcePosition;
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QuerySourcePositionsRequest>, I>>(base?: I): QuerySourcePositionsRequest {
    return QuerySourcePositionsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QuerySourcePositionsRequest>, I>>(object: I): QuerySourcePositionsRequest {
    const message = createBaseQuerySourcePositionsRequest() as any;
    message.project = object.project ?? "";
    message.testId = object.testId ?? "";
    message.variantHash = object.variantHash ?? "";
    message.refHash = object.refHash ?? "";
    message.startSourcePosition = object.startSourcePosition ?? "0";
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseQuerySourcePositionsResponse(): QuerySourcePositionsResponse {
  return { sourcePositions: [], nextPageToken: "" };
}

export const QuerySourcePositionsResponse = {
  encode(message: QuerySourcePositionsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.sourcePositions) {
      SourcePosition.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QuerySourcePositionsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQuerySourcePositionsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.sourcePositions.push(SourcePosition.decode(reader, reader.uint32()));
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

  fromJSON(object: any): QuerySourcePositionsResponse {
    return {
      sourcePositions: globalThis.Array.isArray(object?.sourcePositions)
        ? object.sourcePositions.map((e: any) => SourcePosition.fromJSON(e))
        : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: QuerySourcePositionsResponse): unknown {
    const obj: any = {};
    if (message.sourcePositions?.length) {
      obj.sourcePositions = message.sourcePositions.map((e) => SourcePosition.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QuerySourcePositionsResponse>, I>>(base?: I): QuerySourcePositionsResponse {
    return QuerySourcePositionsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QuerySourcePositionsResponse>, I>>(object: I): QuerySourcePositionsResponse {
    const message = createBaseQuerySourcePositionsResponse() as any;
    message.sourcePositions = object.sourcePositions?.map((e) => SourcePosition.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseSourcePosition(): SourcePosition {
  return { position: "0", commit: undefined, verdicts: [] };
}

export const SourcePosition = {
  encode(message: SourcePosition, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.position !== "0") {
      writer.uint32(8).int64(message.position);
    }
    if (message.commit !== undefined) {
      Commit.encode(message.commit, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.verdicts) {
      TestVerdict.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SourcePosition {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSourcePosition() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.position = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.commit = Commit.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.verdicts.push(TestVerdict.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): SourcePosition {
    return {
      position: isSet(object.position) ? globalThis.String(object.position) : "0",
      commit: isSet(object.commit) ? Commit.fromJSON(object.commit) : undefined,
      verdicts: globalThis.Array.isArray(object?.verdicts)
        ? object.verdicts.map((e: any) => TestVerdict.fromJSON(e))
        : [],
    };
  },

  toJSON(message: SourcePosition): unknown {
    const obj: any = {};
    if (message.position !== "0") {
      obj.position = message.position;
    }
    if (message.commit !== undefined) {
      obj.commit = Commit.toJSON(message.commit);
    }
    if (message.verdicts?.length) {
      obj.verdicts = message.verdicts.map((e) => TestVerdict.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<SourcePosition>, I>>(base?: I): SourcePosition {
    return SourcePosition.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<SourcePosition>, I>>(object: I): SourcePosition {
    const message = createBaseSourcePosition() as any;
    message.position = object.position ?? "0";
    message.commit = (object.commit !== undefined && object.commit !== null)
      ? Commit.fromPartial(object.commit)
      : undefined;
    message.verdicts = object.verdicts?.map((e) => TestVerdict.fromPartial(e)) || [];
    return message;
  },
};

/**
 * Provide methods to read data for test variant branches including
 * results from changepoint analysis, and test verdicts.
 */
export interface TestVariantBranches {
  /**
   * Retrieves the raw state of test variant branch analysis.
   * For reading test variant branch analyses from Spanner.
   * This enables us to inspect the state of a test variant branch
   * analysis in Spanner (which cannot easily inspected using SQL queries,
   * because the data is encoded).
   * This is currently only for LUCI Analysis admin users.
   */
  GetRaw(request: GetRawTestVariantBranchRequest): Promise<TestVariantBranchRaw>;
  /** Retrieves the current state of segments of test variant branch analysis in batches. */
  BatchGet(request: BatchGetTestVariantBranchRequest): Promise<BatchGetTestVariantBranchResponse>;
  /** Lists commits and the test verdicts at these commits, starting from a source position. */
  QuerySourcePositions(request: QuerySourcePositionsRequest): Promise<QuerySourcePositionsResponse>;
}

export const TestVariantBranchesServiceName = "luci.analysis.v1.TestVariantBranches";
export class TestVariantBranchesClientImpl implements TestVariantBranches {
  static readonly DEFAULT_SERVICE = TestVariantBranchesServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || TestVariantBranchesServiceName;
    this.rpc = rpc;
    this.GetRaw = this.GetRaw.bind(this);
    this.BatchGet = this.BatchGet.bind(this);
    this.QuerySourcePositions = this.QuerySourcePositions.bind(this);
  }
  GetRaw(request: GetRawTestVariantBranchRequest): Promise<TestVariantBranchRaw> {
    const data = GetRawTestVariantBranchRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetRaw", data);
    return promise.then((data) => TestVariantBranchRaw.fromJSON(data));
  }

  BatchGet(request: BatchGetTestVariantBranchRequest): Promise<BatchGetTestVariantBranchResponse> {
    const data = BatchGetTestVariantBranchRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "BatchGet", data);
    return promise.then((data) => BatchGetTestVariantBranchResponse.fromJSON(data));
  }

  QuerySourcePositions(request: QuerySourcePositionsRequest): Promise<QuerySourcePositionsResponse> {
    const data = QuerySourcePositionsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "QuerySourcePositions", data);
    return promise.then((data) => QuerySourcePositionsResponse.fromJSON(data));
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
