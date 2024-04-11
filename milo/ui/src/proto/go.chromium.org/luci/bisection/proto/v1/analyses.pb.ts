/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { FieldMask } from "../../../../../google/protobuf/field_mask.pb";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { BuilderID } from "../../../buildbucket/proto/builder_common.pb";
import { GitilesCommit } from "../../../buildbucket/proto/common.pb";
import { BugInfo } from "./bugs.pb";
import {
  AnalysisStatus,
  analysisStatusFromJSON,
  analysisStatusToJSON,
  RerunStatus,
  rerunStatusFromJSON,
  rerunStatusToJSON,
  SuspectVerificationStatus,
  suspectVerificationStatusFromJSON,
  suspectVerificationStatusToJSON,
  Variant,
} from "./common.pb";
import { Culprit, CulpritAction } from "./culprits.pb";
import { HeuristicAnalysisResult } from "./heuristic.pb";
import { BlameList, NthSectionAnalysisResult, RegressionRange } from "./nthsection.pb";

export const protobufPackage = "luci.bisection.v1";

/**
 * AnalysisRunStatus focusses on whether the analysis is currently running, not
 * the actual result of the analysis.
 */
export enum AnalysisRunStatus {
  UNSPECIFIED = 0,
  /** STARTED - The analysis started and is still running. */
  STARTED = 2,
  /** ENDED - The analysis has ended (either it stopped naturally or ran into an error). */
  ENDED = 3,
  /** CANCELED - The analysis has been canceled. */
  CANCELED = 4,
}

export function analysisRunStatusFromJSON(object: any): AnalysisRunStatus {
  switch (object) {
    case 0:
    case "ANALYSIS_RUN_STATUS_UNSPECIFIED":
      return AnalysisRunStatus.UNSPECIFIED;
    case 2:
    case "STARTED":
      return AnalysisRunStatus.STARTED;
    case 3:
    case "ENDED":
      return AnalysisRunStatus.ENDED;
    case 4:
    case "CANCELED":
      return AnalysisRunStatus.CANCELED;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum AnalysisRunStatus");
  }
}

export function analysisRunStatusToJSON(object: AnalysisRunStatus): string {
  switch (object) {
    case AnalysisRunStatus.UNSPECIFIED:
      return "ANALYSIS_RUN_STATUS_UNSPECIFIED";
    case AnalysisRunStatus.STARTED:
      return "STARTED";
    case AnalysisRunStatus.ENDED:
      return "ENDED";
    case AnalysisRunStatus.CANCELED:
      return "CANCELED";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum AnalysisRunStatus");
  }
}

export enum BuildFailureType {
  UNSPECIFIED = 0,
  COMPILE = 1,
  TEST = 2,
  INFRA = 3,
  OTHER = 4,
}

export function buildFailureTypeFromJSON(object: any): BuildFailureType {
  switch (object) {
    case 0:
    case "BUILD_FAILURE_TYPE_UNSPECIFIED":
      return BuildFailureType.UNSPECIFIED;
    case 1:
    case "COMPILE":
      return BuildFailureType.COMPILE;
    case 2:
    case "TEST":
      return BuildFailureType.TEST;
    case 3:
    case "INFRA":
      return BuildFailureType.INFRA;
    case 4:
    case "OTHER":
      return BuildFailureType.OTHER;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuildFailureType");
  }
}

export function buildFailureTypeToJSON(object: BuildFailureType): string {
  switch (object) {
    case BuildFailureType.UNSPECIFIED:
      return "BUILD_FAILURE_TYPE_UNSPECIFIED";
    case BuildFailureType.COMPILE:
      return "COMPILE";
    case BuildFailureType.TEST:
      return "TEST";
    case BuildFailureType.INFRA:
      return "INFRA";
    case BuildFailureType.OTHER:
      return "OTHER";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuildFailureType");
  }
}

export interface GetAnalysisRequest {
  /** ID of the analysis. */
  readonly analysisId: string;
}

export interface QueryAnalysisRequest {
  /** The build failure information to query for the analyses. */
  readonly buildFailure: BuildFailure | undefined;
}

export interface QueryAnalysisResponse {
  /** The analyses corresponding to the QueryAnalysisRequest. */
  readonly analyses: readonly Analysis[];
}

export interface ListAnalysesRequest {
  /**
   * Optional. The maximum number of analyses to be returned in the response.
   * The service may return fewer than this value.
   * If unspecified, at most 50 analyses will be returned.
   * The maximum value is 200; values above 200 will be coerced to 200.
   */
  readonly pageSize: number;
  /**
   * Optional. A page token, received from a previous `ListAnalyses` call.
   * Provide this to retrieve the subsequent page.
   * When paginating, all other parameters provided to `ListAnalyses` must
   * match the call that provided the page token,
   * with the exception of page_size and page_token.
   */
  readonly pageToken: string;
}

export interface ListAnalysesResponse {
  /** The analyses corresponding to the ListAnalysesRequest. */
  readonly analyses: readonly Analysis[];
  /**
   * The token to send as `page_token` to retrieve the next page of analyses.
   * If this field is omitted, there are no subsequent pages.
   */
  readonly nextPageToken: string;
}

export interface TriggerAnalysisRequest {
  /** Failure for which to trigger the analysis. */
  readonly buildFailure:
    | BuildFailure
    | undefined;
  /**
   * Optionally, the client can pass the bug associated with the failure.
   * LUCI Bisection will update the bug with analysis progress/result.
   * This is mainly for SoM, which has information about bugs associated
   * with a failure.
   */
  readonly bugInfo: readonly BugInfo[];
}

export interface TriggerAnalysisResponse {
  /**
   * The analysis result corresponding to the request.
   * It is either a new analysis or an existing one.
   */
  readonly result:
    | Analysis
    | undefined;
  /**
   * is_new_analysis will be set to true if a new analysis is triggered.
   * It will be set to false if an existing analysis is used instead.
   */
  readonly isNewAnalysis: boolean;
}

/**
 * Update the information of an analysis,
 * e.g. update the bugs associated with an analysis.
 * LUCI Bisection will comment on the bug with analysis progress/results.
 * Note: Existing bugs associated with the analysis will be replaced.
 */
export interface UpdateAnalysisRequest {
  /** ID of the analysis. */
  readonly analysisId: string;
  readonly bugInfo: readonly BugInfo[];
}

/**
 * Analysis contains result of an analysis.
 * Next available tag: 15.
 */
export interface Analysis {
  /** ID to identify this analysis. */
  readonly analysisId: string;
  /** The failure associated with the analysis. */
  readonly buildFailure:
    | BuildFailure
    | undefined;
  /** Result status of the analysis. */
  readonly status: AnalysisStatus;
  /**
   * Run status of the analysis.
   * See https://go.chromium.org/luci/bisection/proto/v1/#AnalysisRunStatus
   */
  readonly runStatus: AnalysisRunStatus;
  /** Buildbucket ID for the last passed build. */
  readonly lastPassedBbid: string;
  /** Buildbucket ID for the first failed build. */
  readonly firstFailedBbid: string;
  /** Timestamp for the created time of the analysis. */
  readonly createdTime:
    | string
    | undefined;
  /** Timestamp for the last updated time of the analysis. */
  readonly lastUpdatedTime:
    | string
    | undefined;
  /** Timestamp for the end time of the analysis. */
  readonly endTime:
    | string
    | undefined;
  /** Result of heuristic analysis. */
  readonly heuristicResult:
    | HeuristicAnalysisResult
    | undefined;
  /** Result of nth-section analysis. */
  readonly nthSectionResult:
    | NthSectionAnalysisResult
    | undefined;
  /** Builder for the first failed build. */
  readonly builder:
    | BuilderID
    | undefined;
  /** Type of the failure associated with the analysis. */
  readonly buildFailureType: BuildFailureType;
  /**
   * The culprits for the analysis.
   * For some rare cases, we may get more than one culprit for a regression
   * range. So we set it as repeated field.
   */
  readonly culprits: readonly Culprit[];
}

export interface BuildFailure {
  /** Buildbucket ID for the failed build. */
  readonly bbid: string;
  /** failed_step_name should be 'compile' for compile failures. */
  readonly failedStepName: string;
}

export interface ListTestAnalysesRequest {
  /** The project that the test analyses belong to. */
  readonly project: string;
  /**
   * Optional. The maximum number of analyses to be returned in the response.
   * The service may return fewer than this value.
   * If unspecified, at most 50 analyses will be returned.
   * The maximum value is 200; values above 200 will be coerced to 200.
   */
  readonly pageSize: number;
  /**
   * Optional. A page token, received from a previous `ListTestAnalyses` call.
   * Provide this to retrieve the subsequent page.
   * When paginating, all other parameters provided to `ListTestAnalyses` must
   * match the call that provided the page token,
   * with the exception of page_size and page_token.
   */
  readonly pageToken: string;
  /**
   * The fields to be included in the response.
   * By default, all fields are included.
   */
  readonly fields: readonly string[] | undefined;
}

export interface ListTestAnalysesResponse {
  /** The test analyses corresponding to the ListTestAnalysesRequest. */
  readonly analyses: readonly TestAnalysis[];
  /**
   * The token to send as `page_token` to retrieve the next page of analyses.
   * If this field is omitted, there are no subsequent pages.
   */
  readonly nextPageToken: string;
}

export interface GetTestAnalysisRequest {
  /** ID of the analysis. */
  readonly analysisId: string;
  /**
   * The fields to be included in the response.
   * By default, all fields are included.
   */
  readonly fields: readonly string[] | undefined;
}

export interface TestAnalysis {
  /** ID to identify this analysis. */
  readonly analysisId: string;
  /** Timestamp for the create time of the analysis. */
  readonly createdTime:
    | string
    | undefined;
  /** Timestamp for the start time of the analysis. */
  readonly startTime:
    | string
    | undefined;
  /** Timestamp for the end time of the analysis. */
  readonly endTime:
    | string
    | undefined;
  /** Result status of the analysis. */
  readonly status: AnalysisStatus;
  /** Run status of the analysis. */
  readonly runStatus: AnalysisRunStatus;
  /** The verified culprit for the analysis. */
  readonly culprit:
    | TestCulprit
    | undefined;
  /** The builder that the analysis analyzed. */
  readonly builder:
    | BuilderID
    | undefined;
  /**
   * Test failures that the analysis analyzed.
   * The first item will be the primary failure, followed by other failures.
   * Bisection process will follow the path of the primary test failure.
   */
  readonly testFailures: readonly TestFailure[];
  /** The start commit of the regression range (exclusive). */
  readonly startCommit:
    | GitilesCommit
    | undefined;
  /** The end commit of the regression range (inclusive). */
  readonly endCommit:
    | GitilesCommit
    | undefined;
  /** Sample build bucket ID where the primary test failure failed. */
  readonly sampleBbid: string;
  /** Nthsection result. */
  readonly nthSectionResult: TestNthSectionAnalysisResult | undefined;
}

export interface TestFailure {
  /** The ID of the test. */
  readonly testId: string;
  /** The variant hash of the test. */
  readonly variantHash: string;
  /** Hash to identify the branch in the source control. */
  readonly refHash: string;
  /** The variant of the test. */
  readonly variant:
    | Variant
    | undefined;
  /**
   * Whether the test failure was diverged from the primary test failure
   * during the bisection process.
   */
  readonly isDiverged: boolean;
  /** Whether the test failure is a primary failure or not. */
  readonly isPrimary: boolean;
  /** Start hour of the test failure. */
  readonly startHour:
    | string
    | undefined;
  /** The unexpected test result rate at the start position of the changepoint. */
  readonly startUnexpectedResultRate: number;
  /** The unexpected test result rate at the end position of the changepoint. */
  readonly endUnexpectedResultRate: number;
}

export interface TestNthSectionAnalysisResult {
  /** The status of the nth-section analysis. */
  readonly status: AnalysisStatus;
  /** The run status of the nth-section analysis. */
  readonly runStatus: AnalysisRunStatus;
  /** Timestamp for the start time of the nth-section analysis. */
  readonly startTime:
    | string
    | undefined;
  /** Timestamp for the end time of the nth-section analysis. */
  readonly endTime:
    | string
    | undefined;
  /**
   * Optional, when status = RUNNING. This is the possible commit range of the
   * culprit. This will be updated as the nth-section progress.
   * This will only be available if nthsection is still running (not ended).
   */
  readonly remainingNthSectionRange:
    | RegressionRange
    | undefined;
  /**
   * List of the reruns that have been run so far for the nth-section analysis.
   * The runs are sorted by the create timestamp.
   */
  readonly reruns: readonly TestSingleRerun[];
  /**
   * The blame list of commits to run the nth-section analysis on.
   * The commits are sorted by recency, with the most recent commit first.
   */
  readonly blameList:
    | BlameList
    | undefined;
  /** Optional, when nth-section has found a culprit. */
  readonly suspect: TestCulprit | undefined;
}

export interface TestSingleRerun {
  /** Buildbucket ID of the rerun build. */
  readonly bbid: string;
  /** Timestamp for the create time of the rerun. */
  readonly createTime:
    | string
    | undefined;
  /** Timestamp for the start time of the rerun. */
  readonly startTime:
    | string
    | undefined;
  /** Timestamp for the end time of the rerun. */
  readonly endTime:
    | string
    | undefined;
  /** Timestamp when the rerun send the result to bisection from recipe. */
  readonly reportTime:
    | string
    | undefined;
  /** ID of the bot that runs the rerun. */
  readonly botId: string;
  /** Result of the rerun. */
  readonly rerunResult:
    | RerunTestResults
    | undefined;
  /** Gitiles commit to do the rerun with. */
  readonly commit:
    | GitilesCommit
    | undefined;
  /**
   * Index of the commit to rerun within the blamelist, if this is an
   * nth-section rerun. We need to use a string instead of an int here because
   * 0 is a possible valid value but would get lost due to the "omitempty" flag
   * in the generated proto.
   * There is one case where the index is not populated (empty string). It is when
   * the culprit is the (last pass + 1) position, and this rerun is for parent commit
   * of the culprit verification. In such cases, the parent commit (last pass) is not found in the
   * blamelist (this blamelist is (last pass, first fail]). In such case, index will be "".
   */
  readonly index: string;
}

export interface RerunTestResults {
  readonly results: readonly RerunTestSingleResult[];
  /** Status of the rerun. */
  readonly rerunStatus: RerunStatus;
}

export interface RerunTestSingleResult {
  /** Test ID of the result. */
  readonly testId: string;
  /** Variant hash of the result. */
  readonly variantHash: string;
  /** Number of expected results. */
  readonly expectedCount: string;
  /** Number of unexpected results. */
  readonly unexpectedCount: string;
}

export interface TestSuspectVerificationDetails {
  /** The status of the suspect verification. */
  readonly status: SuspectVerificationStatus;
  /** The verification rerun build for the suspect commit. */
  readonly suspectRerun:
    | TestSingleRerun
    | undefined;
  /** The verification rerun build for the parent commit of the suspect. */
  readonly parentRerun: TestSingleRerun | undefined;
}

export interface TestCulprit {
  /** The gitiles commit for the culprit. */
  readonly commit:
    | GitilesCommit
    | undefined;
  /** The review URL for the culprit. */
  readonly reviewUrl: string;
  /** The review title for the culprit. */
  readonly reviewTitle: string;
  /**
   * Actions we have taken with the culprit.
   * More than one action may be taken, for example, reverting the culprit and
   * commenting on the bug.
   */
  readonly culpritAction: readonly CulpritAction[];
  /** The details of suspect verification for the culprit. */
  readonly verificationDetails: TestSuspectVerificationDetails | undefined;
}

export interface BatchGetTestAnalysesRequest {
  /** The LUCI project. */
  readonly project: string;
  /**
   * The response will only contain analyses which analyze failures in this list.
   * It is an error to request for more than 100 test failures.
   */
  readonly testFailures: readonly BatchGetTestAnalysesRequest_TestFailureIdentifier[];
  /**
   * The fields to be included in the response.
   * By default, all fields are included.
   */
  readonly fields: readonly string[] | undefined;
}

/** Identify a test failure. */
export interface BatchGetTestAnalysesRequest_TestFailureIdentifier {
  /**
   * Identify a test variant. All fields are required.
   * This represents the ongoing test failure of this test variant.
   */
  readonly testId: string;
  readonly variantHash: string;
  /**
   * TODO: Add an optional source_position field in this proto.
   * This is the source position where a failure occurs.
   * See go/surface-bisection-and-changepoint-analysis-som.
   */
  readonly refHash: string;
}

export interface BatchGetTestAnalysesResponse {
  /**
   * Test analyses for each test failure in the order they were requested.
   * The test analysis will be null if the requested test failure has not been
   * analyzed by any bisection.
   */
  readonly testAnalyses: readonly TestAnalysis[];
}

function createBaseGetAnalysisRequest(): GetAnalysisRequest {
  return { analysisId: "0" };
}

export const GetAnalysisRequest = {
  encode(message: GetAnalysisRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.analysisId !== "0") {
      writer.uint32(8).int64(message.analysisId);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetAnalysisRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetAnalysisRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.analysisId = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GetAnalysisRequest {
    return { analysisId: isSet(object.analysisId) ? globalThis.String(object.analysisId) : "0" };
  },

  toJSON(message: GetAnalysisRequest): unknown {
    const obj: any = {};
    if (message.analysisId !== "0") {
      obj.analysisId = message.analysisId;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetAnalysisRequest>, I>>(base?: I): GetAnalysisRequest {
    return GetAnalysisRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetAnalysisRequest>, I>>(object: I): GetAnalysisRequest {
    const message = createBaseGetAnalysisRequest() as any;
    message.analysisId = object.analysisId ?? "0";
    return message;
  },
};

function createBaseQueryAnalysisRequest(): QueryAnalysisRequest {
  return { buildFailure: undefined };
}

export const QueryAnalysisRequest = {
  encode(message: QueryAnalysisRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.buildFailure !== undefined) {
      BuildFailure.encode(message.buildFailure, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryAnalysisRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryAnalysisRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buildFailure = BuildFailure.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): QueryAnalysisRequest {
    return { buildFailure: isSet(object.buildFailure) ? BuildFailure.fromJSON(object.buildFailure) : undefined };
  },

  toJSON(message: QueryAnalysisRequest): unknown {
    const obj: any = {};
    if (message.buildFailure !== undefined) {
      obj.buildFailure = BuildFailure.toJSON(message.buildFailure);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryAnalysisRequest>, I>>(base?: I): QueryAnalysisRequest {
    return QueryAnalysisRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryAnalysisRequest>, I>>(object: I): QueryAnalysisRequest {
    const message = createBaseQueryAnalysisRequest() as any;
    message.buildFailure = (object.buildFailure !== undefined && object.buildFailure !== null)
      ? BuildFailure.fromPartial(object.buildFailure)
      : undefined;
    return message;
  },
};

function createBaseQueryAnalysisResponse(): QueryAnalysisResponse {
  return { analyses: [] };
}

export const QueryAnalysisResponse = {
  encode(message: QueryAnalysisResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.analyses) {
      Analysis.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryAnalysisResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseQueryAnalysisResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.analyses.push(Analysis.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): QueryAnalysisResponse {
    return {
      analyses: globalThis.Array.isArray(object?.analyses) ? object.analyses.map((e: any) => Analysis.fromJSON(e)) : [],
    };
  },

  toJSON(message: QueryAnalysisResponse): unknown {
    const obj: any = {};
    if (message.analyses?.length) {
      obj.analyses = message.analyses.map((e) => Analysis.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<QueryAnalysisResponse>, I>>(base?: I): QueryAnalysisResponse {
    return QueryAnalysisResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<QueryAnalysisResponse>, I>>(object: I): QueryAnalysisResponse {
    const message = createBaseQueryAnalysisResponse() as any;
    message.analyses = object.analyses?.map((e) => Analysis.fromPartial(e)) || [];
    return message;
  },
};

function createBaseListAnalysesRequest(): ListAnalysesRequest {
  return { pageSize: 0, pageToken: "" };
}

export const ListAnalysesRequest = {
  encode(message: ListAnalysesRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pageSize !== 0) {
      writer.uint32(8).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(18).string(message.pageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListAnalysesRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListAnalysesRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.pageSize = reader.int32();
          continue;
        case 2:
          if (tag !== 18) {
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

  fromJSON(object: any): ListAnalysesRequest {
    return {
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
    };
  },

  toJSON(message: ListAnalysesRequest): unknown {
    const obj: any = {};
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListAnalysesRequest>, I>>(base?: I): ListAnalysesRequest {
    return ListAnalysesRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListAnalysesRequest>, I>>(object: I): ListAnalysesRequest {
    const message = createBaseListAnalysesRequest() as any;
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    return message;
  },
};

function createBaseListAnalysesResponse(): ListAnalysesResponse {
  return { analyses: [], nextPageToken: "" };
}

export const ListAnalysesResponse = {
  encode(message: ListAnalysesResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.analyses) {
      Analysis.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListAnalysesResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListAnalysesResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.analyses.push(Analysis.decode(reader, reader.uint32()));
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

  fromJSON(object: any): ListAnalysesResponse {
    return {
      analyses: globalThis.Array.isArray(object?.analyses) ? object.analyses.map((e: any) => Analysis.fromJSON(e)) : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: ListAnalysesResponse): unknown {
    const obj: any = {};
    if (message.analyses?.length) {
      obj.analyses = message.analyses.map((e) => Analysis.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListAnalysesResponse>, I>>(base?: I): ListAnalysesResponse {
    return ListAnalysesResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListAnalysesResponse>, I>>(object: I): ListAnalysesResponse {
    const message = createBaseListAnalysesResponse() as any;
    message.analyses = object.analyses?.map((e) => Analysis.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseTriggerAnalysisRequest(): TriggerAnalysisRequest {
  return { buildFailure: undefined, bugInfo: [] };
}

export const TriggerAnalysisRequest = {
  encode(message: TriggerAnalysisRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.buildFailure !== undefined) {
      BuildFailure.encode(message.buildFailure, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.bugInfo) {
      BugInfo.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TriggerAnalysisRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTriggerAnalysisRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buildFailure = BuildFailure.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.bugInfo.push(BugInfo.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TriggerAnalysisRequest {
    return {
      buildFailure: isSet(object.buildFailure) ? BuildFailure.fromJSON(object.buildFailure) : undefined,
      bugInfo: globalThis.Array.isArray(object?.bugInfo) ? object.bugInfo.map((e: any) => BugInfo.fromJSON(e)) : [],
    };
  },

  toJSON(message: TriggerAnalysisRequest): unknown {
    const obj: any = {};
    if (message.buildFailure !== undefined) {
      obj.buildFailure = BuildFailure.toJSON(message.buildFailure);
    }
    if (message.bugInfo?.length) {
      obj.bugInfo = message.bugInfo.map((e) => BugInfo.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TriggerAnalysisRequest>, I>>(base?: I): TriggerAnalysisRequest {
    return TriggerAnalysisRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TriggerAnalysisRequest>, I>>(object: I): TriggerAnalysisRequest {
    const message = createBaseTriggerAnalysisRequest() as any;
    message.buildFailure = (object.buildFailure !== undefined && object.buildFailure !== null)
      ? BuildFailure.fromPartial(object.buildFailure)
      : undefined;
    message.bugInfo = object.bugInfo?.map((e) => BugInfo.fromPartial(e)) || [];
    return message;
  },
};

function createBaseTriggerAnalysisResponse(): TriggerAnalysisResponse {
  return { result: undefined, isNewAnalysis: false };
}

export const TriggerAnalysisResponse = {
  encode(message: TriggerAnalysisResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.result !== undefined) {
      Analysis.encode(message.result, writer.uint32(10).fork()).ldelim();
    }
    if (message.isNewAnalysis === true) {
      writer.uint32(16).bool(message.isNewAnalysis);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TriggerAnalysisResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTriggerAnalysisResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.result = Analysis.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.isNewAnalysis = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TriggerAnalysisResponse {
    return {
      result: isSet(object.result) ? Analysis.fromJSON(object.result) : undefined,
      isNewAnalysis: isSet(object.isNewAnalysis) ? globalThis.Boolean(object.isNewAnalysis) : false,
    };
  },

  toJSON(message: TriggerAnalysisResponse): unknown {
    const obj: any = {};
    if (message.result !== undefined) {
      obj.result = Analysis.toJSON(message.result);
    }
    if (message.isNewAnalysis === true) {
      obj.isNewAnalysis = message.isNewAnalysis;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TriggerAnalysisResponse>, I>>(base?: I): TriggerAnalysisResponse {
    return TriggerAnalysisResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TriggerAnalysisResponse>, I>>(object: I): TriggerAnalysisResponse {
    const message = createBaseTriggerAnalysisResponse() as any;
    message.result = (object.result !== undefined && object.result !== null)
      ? Analysis.fromPartial(object.result)
      : undefined;
    message.isNewAnalysis = object.isNewAnalysis ?? false;
    return message;
  },
};

function createBaseUpdateAnalysisRequest(): UpdateAnalysisRequest {
  return { analysisId: "", bugInfo: [] };
}

export const UpdateAnalysisRequest = {
  encode(message: UpdateAnalysisRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.analysisId !== "") {
      writer.uint32(10).string(message.analysisId);
    }
    for (const v of message.bugInfo) {
      BugInfo.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UpdateAnalysisRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUpdateAnalysisRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.analysisId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.bugInfo.push(BugInfo.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): UpdateAnalysisRequest {
    return {
      analysisId: isSet(object.analysisId) ? globalThis.String(object.analysisId) : "",
      bugInfo: globalThis.Array.isArray(object?.bugInfo) ? object.bugInfo.map((e: any) => BugInfo.fromJSON(e)) : [],
    };
  },

  toJSON(message: UpdateAnalysisRequest): unknown {
    const obj: any = {};
    if (message.analysisId !== "") {
      obj.analysisId = message.analysisId;
    }
    if (message.bugInfo?.length) {
      obj.bugInfo = message.bugInfo.map((e) => BugInfo.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<UpdateAnalysisRequest>, I>>(base?: I): UpdateAnalysisRequest {
    return UpdateAnalysisRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<UpdateAnalysisRequest>, I>>(object: I): UpdateAnalysisRequest {
    const message = createBaseUpdateAnalysisRequest() as any;
    message.analysisId = object.analysisId ?? "";
    message.bugInfo = object.bugInfo?.map((e) => BugInfo.fromPartial(e)) || [];
    return message;
  },
};

function createBaseAnalysis(): Analysis {
  return {
    analysisId: "0",
    buildFailure: undefined,
    status: 0,
    runStatus: 0,
    lastPassedBbid: "0",
    firstFailedBbid: "0",
    createdTime: undefined,
    lastUpdatedTime: undefined,
    endTime: undefined,
    heuristicResult: undefined,
    nthSectionResult: undefined,
    builder: undefined,
    buildFailureType: 0,
    culprits: [],
  };
}

export const Analysis = {
  encode(message: Analysis, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.analysisId !== "0") {
      writer.uint32(8).int64(message.analysisId);
    }
    if (message.buildFailure !== undefined) {
      BuildFailure.encode(message.buildFailure, writer.uint32(18).fork()).ldelim();
    }
    if (message.status !== 0) {
      writer.uint32(24).int32(message.status);
    }
    if (message.runStatus !== 0) {
      writer.uint32(32).int32(message.runStatus);
    }
    if (message.lastPassedBbid !== "0") {
      writer.uint32(40).int64(message.lastPassedBbid);
    }
    if (message.firstFailedBbid !== "0") {
      writer.uint32(48).int64(message.firstFailedBbid);
    }
    if (message.createdTime !== undefined) {
      Timestamp.encode(toTimestamp(message.createdTime), writer.uint32(58).fork()).ldelim();
    }
    if (message.lastUpdatedTime !== undefined) {
      Timestamp.encode(toTimestamp(message.lastUpdatedTime), writer.uint32(66).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(74).fork()).ldelim();
    }
    if (message.heuristicResult !== undefined) {
      HeuristicAnalysisResult.encode(message.heuristicResult, writer.uint32(82).fork()).ldelim();
    }
    if (message.nthSectionResult !== undefined) {
      NthSectionAnalysisResult.encode(message.nthSectionResult, writer.uint32(90).fork()).ldelim();
    }
    if (message.builder !== undefined) {
      BuilderID.encode(message.builder, writer.uint32(98).fork()).ldelim();
    }
    if (message.buildFailureType !== 0) {
      writer.uint32(104).int32(message.buildFailureType);
    }
    for (const v of message.culprits) {
      Culprit.encode(v!, writer.uint32(114).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Analysis {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseAnalysis() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.analysisId = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.buildFailure = BuildFailure.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.runStatus = reader.int32() as any;
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.lastPassedBbid = longToString(reader.int64() as Long);
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.firstFailedBbid = longToString(reader.int64() as Long);
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.createdTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.lastUpdatedTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.endTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.heuristicResult = HeuristicAnalysisResult.decode(reader, reader.uint32());
          continue;
        case 11:
          if (tag !== 90) {
            break;
          }

          message.nthSectionResult = NthSectionAnalysisResult.decode(reader, reader.uint32());
          continue;
        case 12:
          if (tag !== 98) {
            break;
          }

          message.builder = BuilderID.decode(reader, reader.uint32());
          continue;
        case 13:
          if (tag !== 104) {
            break;
          }

          message.buildFailureType = reader.int32() as any;
          continue;
        case 14:
          if (tag !== 114) {
            break;
          }

          message.culprits.push(Culprit.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Analysis {
    return {
      analysisId: isSet(object.analysisId) ? globalThis.String(object.analysisId) : "0",
      buildFailure: isSet(object.buildFailure) ? BuildFailure.fromJSON(object.buildFailure) : undefined,
      status: isSet(object.status) ? analysisStatusFromJSON(object.status) : 0,
      runStatus: isSet(object.runStatus) ? analysisRunStatusFromJSON(object.runStatus) : 0,
      lastPassedBbid: isSet(object.lastPassedBbid) ? globalThis.String(object.lastPassedBbid) : "0",
      firstFailedBbid: isSet(object.firstFailedBbid) ? globalThis.String(object.firstFailedBbid) : "0",
      createdTime: isSet(object.createdTime) ? globalThis.String(object.createdTime) : undefined,
      lastUpdatedTime: isSet(object.lastUpdatedTime) ? globalThis.String(object.lastUpdatedTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      heuristicResult: isSet(object.heuristicResult)
        ? HeuristicAnalysisResult.fromJSON(object.heuristicResult)
        : undefined,
      nthSectionResult: isSet(object.nthSectionResult)
        ? NthSectionAnalysisResult.fromJSON(object.nthSectionResult)
        : undefined,
      builder: isSet(object.builder) ? BuilderID.fromJSON(object.builder) : undefined,
      buildFailureType: isSet(object.buildFailureType) ? buildFailureTypeFromJSON(object.buildFailureType) : 0,
      culprits: globalThis.Array.isArray(object?.culprits) ? object.culprits.map((e: any) => Culprit.fromJSON(e)) : [],
    };
  },

  toJSON(message: Analysis): unknown {
    const obj: any = {};
    if (message.analysisId !== "0") {
      obj.analysisId = message.analysisId;
    }
    if (message.buildFailure !== undefined) {
      obj.buildFailure = BuildFailure.toJSON(message.buildFailure);
    }
    if (message.status !== 0) {
      obj.status = analysisStatusToJSON(message.status);
    }
    if (message.runStatus !== 0) {
      obj.runStatus = analysisRunStatusToJSON(message.runStatus);
    }
    if (message.lastPassedBbid !== "0") {
      obj.lastPassedBbid = message.lastPassedBbid;
    }
    if (message.firstFailedBbid !== "0") {
      obj.firstFailedBbid = message.firstFailedBbid;
    }
    if (message.createdTime !== undefined) {
      obj.createdTime = message.createdTime;
    }
    if (message.lastUpdatedTime !== undefined) {
      obj.lastUpdatedTime = message.lastUpdatedTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    if (message.heuristicResult !== undefined) {
      obj.heuristicResult = HeuristicAnalysisResult.toJSON(message.heuristicResult);
    }
    if (message.nthSectionResult !== undefined) {
      obj.nthSectionResult = NthSectionAnalysisResult.toJSON(message.nthSectionResult);
    }
    if (message.builder !== undefined) {
      obj.builder = BuilderID.toJSON(message.builder);
    }
    if (message.buildFailureType !== 0) {
      obj.buildFailureType = buildFailureTypeToJSON(message.buildFailureType);
    }
    if (message.culprits?.length) {
      obj.culprits = message.culprits.map((e) => Culprit.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Analysis>, I>>(base?: I): Analysis {
    return Analysis.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Analysis>, I>>(object: I): Analysis {
    const message = createBaseAnalysis() as any;
    message.analysisId = object.analysisId ?? "0";
    message.buildFailure = (object.buildFailure !== undefined && object.buildFailure !== null)
      ? BuildFailure.fromPartial(object.buildFailure)
      : undefined;
    message.status = object.status ?? 0;
    message.runStatus = object.runStatus ?? 0;
    message.lastPassedBbid = object.lastPassedBbid ?? "0";
    message.firstFailedBbid = object.firstFailedBbid ?? "0";
    message.createdTime = object.createdTime ?? undefined;
    message.lastUpdatedTime = object.lastUpdatedTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.heuristicResult = (object.heuristicResult !== undefined && object.heuristicResult !== null)
      ? HeuristicAnalysisResult.fromPartial(object.heuristicResult)
      : undefined;
    message.nthSectionResult = (object.nthSectionResult !== undefined && object.nthSectionResult !== null)
      ? NthSectionAnalysisResult.fromPartial(object.nthSectionResult)
      : undefined;
    message.builder = (object.builder !== undefined && object.builder !== null)
      ? BuilderID.fromPartial(object.builder)
      : undefined;
    message.buildFailureType = object.buildFailureType ?? 0;
    message.culprits = object.culprits?.map((e) => Culprit.fromPartial(e)) || [];
    return message;
  },
};

function createBaseBuildFailure(): BuildFailure {
  return { bbid: "0", failedStepName: "" };
}

export const BuildFailure = {
  encode(message: BuildFailure, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bbid !== "0") {
      writer.uint32(8).int64(message.bbid);
    }
    if (message.failedStepName !== "") {
      writer.uint32(18).string(message.failedStepName);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildFailure {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildFailure() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.bbid = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.failedStepName = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildFailure {
    return {
      bbid: isSet(object.bbid) ? globalThis.String(object.bbid) : "0",
      failedStepName: isSet(object.failedStepName) ? globalThis.String(object.failedStepName) : "",
    };
  },

  toJSON(message: BuildFailure): unknown {
    const obj: any = {};
    if (message.bbid !== "0") {
      obj.bbid = message.bbid;
    }
    if (message.failedStepName !== "") {
      obj.failedStepName = message.failedStepName;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildFailure>, I>>(base?: I): BuildFailure {
    return BuildFailure.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildFailure>, I>>(object: I): BuildFailure {
    const message = createBaseBuildFailure() as any;
    message.bbid = object.bbid ?? "0";
    message.failedStepName = object.failedStepName ?? "";
    return message;
  },
};

function createBaseListTestAnalysesRequest(): ListTestAnalysesRequest {
  return { project: "", pageSize: 0, pageToken: "", fields: undefined };
}

export const ListTestAnalysesRequest = {
  encode(message: ListTestAnalysesRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.pageSize !== 0) {
      writer.uint32(16).int32(message.pageSize);
    }
    if (message.pageToken !== "") {
      writer.uint32(26).string(message.pageToken);
    }
    if (message.fields !== undefined) {
      FieldMask.encode(FieldMask.wrap(message.fields), writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListTestAnalysesRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListTestAnalysesRequest() as any;
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
        case 4:
          if (tag !== 34) {
            break;
          }

          message.fields = FieldMask.unwrap(FieldMask.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListTestAnalysesRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      pageSize: isSet(object.pageSize) ? globalThis.Number(object.pageSize) : 0,
      pageToken: isSet(object.pageToken) ? globalThis.String(object.pageToken) : "",
      fields: isSet(object.fields) ? FieldMask.unwrap(FieldMask.fromJSON(object.fields)) : undefined,
    };
  },

  toJSON(message: ListTestAnalysesRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.pageSize !== 0) {
      obj.pageSize = Math.round(message.pageSize);
    }
    if (message.pageToken !== "") {
      obj.pageToken = message.pageToken;
    }
    if (message.fields !== undefined) {
      obj.fields = FieldMask.toJSON(FieldMask.wrap(message.fields));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListTestAnalysesRequest>, I>>(base?: I): ListTestAnalysesRequest {
    return ListTestAnalysesRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListTestAnalysesRequest>, I>>(object: I): ListTestAnalysesRequest {
    const message = createBaseListTestAnalysesRequest() as any;
    message.project = object.project ?? "";
    message.pageSize = object.pageSize ?? 0;
    message.pageToken = object.pageToken ?? "";
    message.fields = object.fields ?? undefined;
    return message;
  },
};

function createBaseListTestAnalysesResponse(): ListTestAnalysesResponse {
  return { analyses: [], nextPageToken: "" };
}

export const ListTestAnalysesResponse = {
  encode(message: ListTestAnalysesResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.analyses) {
      TestAnalysis.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.nextPageToken !== "") {
      writer.uint32(18).string(message.nextPageToken);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListTestAnalysesResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListTestAnalysesResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.analyses.push(TestAnalysis.decode(reader, reader.uint32()));
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

  fromJSON(object: any): ListTestAnalysesResponse {
    return {
      analyses: globalThis.Array.isArray(object?.analyses)
        ? object.analyses.map((e: any) => TestAnalysis.fromJSON(e))
        : [],
      nextPageToken: isSet(object.nextPageToken) ? globalThis.String(object.nextPageToken) : "",
    };
  },

  toJSON(message: ListTestAnalysesResponse): unknown {
    const obj: any = {};
    if (message.analyses?.length) {
      obj.analyses = message.analyses.map((e) => TestAnalysis.toJSON(e));
    }
    if (message.nextPageToken !== "") {
      obj.nextPageToken = message.nextPageToken;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListTestAnalysesResponse>, I>>(base?: I): ListTestAnalysesResponse {
    return ListTestAnalysesResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListTestAnalysesResponse>, I>>(object: I): ListTestAnalysesResponse {
    const message = createBaseListTestAnalysesResponse() as any;
    message.analyses = object.analyses?.map((e) => TestAnalysis.fromPartial(e)) || [];
    message.nextPageToken = object.nextPageToken ?? "";
    return message;
  },
};

function createBaseGetTestAnalysisRequest(): GetTestAnalysisRequest {
  return { analysisId: "0", fields: undefined };
}

export const GetTestAnalysisRequest = {
  encode(message: GetTestAnalysisRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.analysisId !== "0") {
      writer.uint32(8).int64(message.analysisId);
    }
    if (message.fields !== undefined) {
      FieldMask.encode(FieldMask.wrap(message.fields), writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetTestAnalysisRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetTestAnalysisRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.analysisId = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.fields = FieldMask.unwrap(FieldMask.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GetTestAnalysisRequest {
    return {
      analysisId: isSet(object.analysisId) ? globalThis.String(object.analysisId) : "0",
      fields: isSet(object.fields) ? FieldMask.unwrap(FieldMask.fromJSON(object.fields)) : undefined,
    };
  },

  toJSON(message: GetTestAnalysisRequest): unknown {
    const obj: any = {};
    if (message.analysisId !== "0") {
      obj.analysisId = message.analysisId;
    }
    if (message.fields !== undefined) {
      obj.fields = FieldMask.toJSON(FieldMask.wrap(message.fields));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetTestAnalysisRequest>, I>>(base?: I): GetTestAnalysisRequest {
    return GetTestAnalysisRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetTestAnalysisRequest>, I>>(object: I): GetTestAnalysisRequest {
    const message = createBaseGetTestAnalysisRequest() as any;
    message.analysisId = object.analysisId ?? "0";
    message.fields = object.fields ?? undefined;
    return message;
  },
};

function createBaseTestAnalysis(): TestAnalysis {
  return {
    analysisId: "0",
    createdTime: undefined,
    startTime: undefined,
    endTime: undefined,
    status: 0,
    runStatus: 0,
    culprit: undefined,
    builder: undefined,
    testFailures: [],
    startCommit: undefined,
    endCommit: undefined,
    sampleBbid: "0",
    nthSectionResult: undefined,
  };
}

export const TestAnalysis = {
  encode(message: TestAnalysis, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.analysisId !== "0") {
      writer.uint32(8).int64(message.analysisId);
    }
    if (message.createdTime !== undefined) {
      Timestamp.encode(toTimestamp(message.createdTime), writer.uint32(18).fork()).ldelim();
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(26).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(34).fork()).ldelim();
    }
    if (message.status !== 0) {
      writer.uint32(40).int32(message.status);
    }
    if (message.runStatus !== 0) {
      writer.uint32(48).int32(message.runStatus);
    }
    if (message.culprit !== undefined) {
      TestCulprit.encode(message.culprit, writer.uint32(58).fork()).ldelim();
    }
    if (message.builder !== undefined) {
      BuilderID.encode(message.builder, writer.uint32(66).fork()).ldelim();
    }
    for (const v of message.testFailures) {
      TestFailure.encode(v!, writer.uint32(74).fork()).ldelim();
    }
    if (message.startCommit !== undefined) {
      GitilesCommit.encode(message.startCommit, writer.uint32(82).fork()).ldelim();
    }
    if (message.endCommit !== undefined) {
      GitilesCommit.encode(message.endCommit, writer.uint32(90).fork()).ldelim();
    }
    if (message.sampleBbid !== "0") {
      writer.uint32(112).int64(message.sampleBbid);
    }
    if (message.nthSectionResult !== undefined) {
      TestNthSectionAnalysisResult.encode(message.nthSectionResult, writer.uint32(122).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestAnalysis {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestAnalysis() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.analysisId = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.createdTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.endTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.runStatus = reader.int32() as any;
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.culprit = TestCulprit.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.builder = BuilderID.decode(reader, reader.uint32());
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.testFailures.push(TestFailure.decode(reader, reader.uint32()));
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.startCommit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 11:
          if (tag !== 90) {
            break;
          }

          message.endCommit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 14:
          if (tag !== 112) {
            break;
          }

          message.sampleBbid = longToString(reader.int64() as Long);
          continue;
        case 15:
          if (tag !== 122) {
            break;
          }

          message.nthSectionResult = TestNthSectionAnalysisResult.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestAnalysis {
    return {
      analysisId: isSet(object.analysisId) ? globalThis.String(object.analysisId) : "0",
      createdTime: isSet(object.createdTime) ? globalThis.String(object.createdTime) : undefined,
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      status: isSet(object.status) ? analysisStatusFromJSON(object.status) : 0,
      runStatus: isSet(object.runStatus) ? analysisRunStatusFromJSON(object.runStatus) : 0,
      culprit: isSet(object.culprit) ? TestCulprit.fromJSON(object.culprit) : undefined,
      builder: isSet(object.builder) ? BuilderID.fromJSON(object.builder) : undefined,
      testFailures: globalThis.Array.isArray(object?.testFailures)
        ? object.testFailures.map((e: any) => TestFailure.fromJSON(e))
        : [],
      startCommit: isSet(object.startCommit) ? GitilesCommit.fromJSON(object.startCommit) : undefined,
      endCommit: isSet(object.endCommit) ? GitilesCommit.fromJSON(object.endCommit) : undefined,
      sampleBbid: isSet(object.sampleBbid) ? globalThis.String(object.sampleBbid) : "0",
      nthSectionResult: isSet(object.nthSectionResult)
        ? TestNthSectionAnalysisResult.fromJSON(object.nthSectionResult)
        : undefined,
    };
  },

  toJSON(message: TestAnalysis): unknown {
    const obj: any = {};
    if (message.analysisId !== "0") {
      obj.analysisId = message.analysisId;
    }
    if (message.createdTime !== undefined) {
      obj.createdTime = message.createdTime;
    }
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    if (message.status !== 0) {
      obj.status = analysisStatusToJSON(message.status);
    }
    if (message.runStatus !== 0) {
      obj.runStatus = analysisRunStatusToJSON(message.runStatus);
    }
    if (message.culprit !== undefined) {
      obj.culprit = TestCulprit.toJSON(message.culprit);
    }
    if (message.builder !== undefined) {
      obj.builder = BuilderID.toJSON(message.builder);
    }
    if (message.testFailures?.length) {
      obj.testFailures = message.testFailures.map((e) => TestFailure.toJSON(e));
    }
    if (message.startCommit !== undefined) {
      obj.startCommit = GitilesCommit.toJSON(message.startCommit);
    }
    if (message.endCommit !== undefined) {
      obj.endCommit = GitilesCommit.toJSON(message.endCommit);
    }
    if (message.sampleBbid !== "0") {
      obj.sampleBbid = message.sampleBbid;
    }
    if (message.nthSectionResult !== undefined) {
      obj.nthSectionResult = TestNthSectionAnalysisResult.toJSON(message.nthSectionResult);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestAnalysis>, I>>(base?: I): TestAnalysis {
    return TestAnalysis.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestAnalysis>, I>>(object: I): TestAnalysis {
    const message = createBaseTestAnalysis() as any;
    message.analysisId = object.analysisId ?? "0";
    message.createdTime = object.createdTime ?? undefined;
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.status = object.status ?? 0;
    message.runStatus = object.runStatus ?? 0;
    message.culprit = (object.culprit !== undefined && object.culprit !== null)
      ? TestCulprit.fromPartial(object.culprit)
      : undefined;
    message.builder = (object.builder !== undefined && object.builder !== null)
      ? BuilderID.fromPartial(object.builder)
      : undefined;
    message.testFailures = object.testFailures?.map((e) => TestFailure.fromPartial(e)) || [];
    message.startCommit = (object.startCommit !== undefined && object.startCommit !== null)
      ? GitilesCommit.fromPartial(object.startCommit)
      : undefined;
    message.endCommit = (object.endCommit !== undefined && object.endCommit !== null)
      ? GitilesCommit.fromPartial(object.endCommit)
      : undefined;
    message.sampleBbid = object.sampleBbid ?? "0";
    message.nthSectionResult = (object.nthSectionResult !== undefined && object.nthSectionResult !== null)
      ? TestNthSectionAnalysisResult.fromPartial(object.nthSectionResult)
      : undefined;
    return message;
  },
};

function createBaseTestFailure(): TestFailure {
  return {
    testId: "",
    variantHash: "",
    refHash: "",
    variant: undefined,
    isDiverged: false,
    isPrimary: false,
    startHour: undefined,
    startUnexpectedResultRate: 0,
    endUnexpectedResultRate: 0,
  };
}

export const TestFailure = {
  encode(message: TestFailure, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.testId !== "") {
      writer.uint32(10).string(message.testId);
    }
    if (message.variantHash !== "") {
      writer.uint32(18).string(message.variantHash);
    }
    if (message.refHash !== "") {
      writer.uint32(26).string(message.refHash);
    }
    if (message.variant !== undefined) {
      Variant.encode(message.variant, writer.uint32(34).fork()).ldelim();
    }
    if (message.isDiverged === true) {
      writer.uint32(40).bool(message.isDiverged);
    }
    if (message.isPrimary === true) {
      writer.uint32(48).bool(message.isPrimary);
    }
    if (message.startHour !== undefined) {
      Timestamp.encode(toTimestamp(message.startHour), writer.uint32(58).fork()).ldelim();
    }
    if (message.startUnexpectedResultRate !== 0) {
      writer.uint32(69).float(message.startUnexpectedResultRate);
    }
    if (message.endUnexpectedResultRate !== 0) {
      writer.uint32(77).float(message.endUnexpectedResultRate);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestFailure {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestFailure() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.variantHash = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.refHash = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.variant = Variant.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.isDiverged = reader.bool();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.isPrimary = reader.bool();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.startHour = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 8:
          if (tag !== 69) {
            break;
          }

          message.startUnexpectedResultRate = reader.float();
          continue;
        case 9:
          if (tag !== 77) {
            break;
          }

          message.endUnexpectedResultRate = reader.float();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestFailure {
    return {
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      refHash: isSet(object.refHash) ? globalThis.String(object.refHash) : "",
      variant: isSet(object.variant) ? Variant.fromJSON(object.variant) : undefined,
      isDiverged: isSet(object.isDiverged) ? globalThis.Boolean(object.isDiverged) : false,
      isPrimary: isSet(object.isPrimary) ? globalThis.Boolean(object.isPrimary) : false,
      startHour: isSet(object.startHour) ? globalThis.String(object.startHour) : undefined,
      startUnexpectedResultRate: isSet(object.startUnexpectedResultRate)
        ? globalThis.Number(object.startUnexpectedResultRate)
        : 0,
      endUnexpectedResultRate: isSet(object.endUnexpectedResultRate)
        ? globalThis.Number(object.endUnexpectedResultRate)
        : 0,
    };
  },

  toJSON(message: TestFailure): unknown {
    const obj: any = {};
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
    if (message.isDiverged === true) {
      obj.isDiverged = message.isDiverged;
    }
    if (message.isPrimary === true) {
      obj.isPrimary = message.isPrimary;
    }
    if (message.startHour !== undefined) {
      obj.startHour = message.startHour;
    }
    if (message.startUnexpectedResultRate !== 0) {
      obj.startUnexpectedResultRate = message.startUnexpectedResultRate;
    }
    if (message.endUnexpectedResultRate !== 0) {
      obj.endUnexpectedResultRate = message.endUnexpectedResultRate;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestFailure>, I>>(base?: I): TestFailure {
    return TestFailure.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestFailure>, I>>(object: I): TestFailure {
    const message = createBaseTestFailure() as any;
    message.testId = object.testId ?? "";
    message.variantHash = object.variantHash ?? "";
    message.refHash = object.refHash ?? "";
    message.variant = (object.variant !== undefined && object.variant !== null)
      ? Variant.fromPartial(object.variant)
      : undefined;
    message.isDiverged = object.isDiverged ?? false;
    message.isPrimary = object.isPrimary ?? false;
    message.startHour = object.startHour ?? undefined;
    message.startUnexpectedResultRate = object.startUnexpectedResultRate ?? 0;
    message.endUnexpectedResultRate = object.endUnexpectedResultRate ?? 0;
    return message;
  },
};

function createBaseTestNthSectionAnalysisResult(): TestNthSectionAnalysisResult {
  return {
    status: 0,
    runStatus: 0,
    startTime: undefined,
    endTime: undefined,
    remainingNthSectionRange: undefined,
    reruns: [],
    blameList: undefined,
    suspect: undefined,
  };
}

export const TestNthSectionAnalysisResult = {
  encode(message: TestNthSectionAnalysisResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.status !== 0) {
      writer.uint32(8).int32(message.status);
    }
    if (message.runStatus !== 0) {
      writer.uint32(16).int32(message.runStatus);
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(26).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(34).fork()).ldelim();
    }
    if (message.remainingNthSectionRange !== undefined) {
      RegressionRange.encode(message.remainingNthSectionRange, writer.uint32(42).fork()).ldelim();
    }
    for (const v of message.reruns) {
      TestSingleRerun.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    if (message.blameList !== undefined) {
      BlameList.encode(message.blameList, writer.uint32(58).fork()).ldelim();
    }
    if (message.suspect !== undefined) {
      TestCulprit.encode(message.suspect, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestNthSectionAnalysisResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestNthSectionAnalysisResult() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.runStatus = reader.int32() as any;
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.endTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.remainingNthSectionRange = RegressionRange.decode(reader, reader.uint32());
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.reruns.push(TestSingleRerun.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.blameList = BlameList.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.suspect = TestCulprit.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestNthSectionAnalysisResult {
    return {
      status: isSet(object.status) ? analysisStatusFromJSON(object.status) : 0,
      runStatus: isSet(object.runStatus) ? analysisRunStatusFromJSON(object.runStatus) : 0,
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      remainingNthSectionRange: isSet(object.remainingNthSectionRange)
        ? RegressionRange.fromJSON(object.remainingNthSectionRange)
        : undefined,
      reruns: globalThis.Array.isArray(object?.reruns)
        ? object.reruns.map((e: any) => TestSingleRerun.fromJSON(e))
        : [],
      blameList: isSet(object.blameList) ? BlameList.fromJSON(object.blameList) : undefined,
      suspect: isSet(object.suspect) ? TestCulprit.fromJSON(object.suspect) : undefined,
    };
  },

  toJSON(message: TestNthSectionAnalysisResult): unknown {
    const obj: any = {};
    if (message.status !== 0) {
      obj.status = analysisStatusToJSON(message.status);
    }
    if (message.runStatus !== 0) {
      obj.runStatus = analysisRunStatusToJSON(message.runStatus);
    }
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    if (message.remainingNthSectionRange !== undefined) {
      obj.remainingNthSectionRange = RegressionRange.toJSON(message.remainingNthSectionRange);
    }
    if (message.reruns?.length) {
      obj.reruns = message.reruns.map((e) => TestSingleRerun.toJSON(e));
    }
    if (message.blameList !== undefined) {
      obj.blameList = BlameList.toJSON(message.blameList);
    }
    if (message.suspect !== undefined) {
      obj.suspect = TestCulprit.toJSON(message.suspect);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestNthSectionAnalysisResult>, I>>(base?: I): TestNthSectionAnalysisResult {
    return TestNthSectionAnalysisResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestNthSectionAnalysisResult>, I>>(object: I): TestNthSectionAnalysisResult {
    const message = createBaseTestNthSectionAnalysisResult() as any;
    message.status = object.status ?? 0;
    message.runStatus = object.runStatus ?? 0;
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.remainingNthSectionRange =
      (object.remainingNthSectionRange !== undefined && object.remainingNthSectionRange !== null)
        ? RegressionRange.fromPartial(object.remainingNthSectionRange)
        : undefined;
    message.reruns = object.reruns?.map((e) => TestSingleRerun.fromPartial(e)) || [];
    message.blameList = (object.blameList !== undefined && object.blameList !== null)
      ? BlameList.fromPartial(object.blameList)
      : undefined;
    message.suspect = (object.suspect !== undefined && object.suspect !== null)
      ? TestCulprit.fromPartial(object.suspect)
      : undefined;
    return message;
  },
};

function createBaseTestSingleRerun(): TestSingleRerun {
  return {
    bbid: "0",
    createTime: undefined,
    startTime: undefined,
    endTime: undefined,
    reportTime: undefined,
    botId: "",
    rerunResult: undefined,
    commit: undefined,
    index: "",
  };
}

export const TestSingleRerun = {
  encode(message: TestSingleRerun, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.bbid !== "0") {
      writer.uint32(8).int64(message.bbid);
    }
    if (message.createTime !== undefined) {
      Timestamp.encode(toTimestamp(message.createTime), writer.uint32(18).fork()).ldelim();
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(26).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(34).fork()).ldelim();
    }
    if (message.reportTime !== undefined) {
      Timestamp.encode(toTimestamp(message.reportTime), writer.uint32(42).fork()).ldelim();
    }
    if (message.botId !== "") {
      writer.uint32(50).string(message.botId);
    }
    if (message.rerunResult !== undefined) {
      RerunTestResults.encode(message.rerunResult, writer.uint32(58).fork()).ldelim();
    }
    if (message.commit !== undefined) {
      GitilesCommit.encode(message.commit, writer.uint32(66).fork()).ldelim();
    }
    if (message.index !== "") {
      writer.uint32(74).string(message.index);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestSingleRerun {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestSingleRerun() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.bbid = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.createTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.endTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.reportTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.botId = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.rerunResult = RerunTestResults.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.commit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.index = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestSingleRerun {
    return {
      bbid: isSet(object.bbid) ? globalThis.String(object.bbid) : "0",
      createTime: isSet(object.createTime) ? globalThis.String(object.createTime) : undefined,
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      reportTime: isSet(object.reportTime) ? globalThis.String(object.reportTime) : undefined,
      botId: isSet(object.botId) ? globalThis.String(object.botId) : "",
      rerunResult: isSet(object.rerunResult) ? RerunTestResults.fromJSON(object.rerunResult) : undefined,
      commit: isSet(object.commit) ? GitilesCommit.fromJSON(object.commit) : undefined,
      index: isSet(object.index) ? globalThis.String(object.index) : "",
    };
  },

  toJSON(message: TestSingleRerun): unknown {
    const obj: any = {};
    if (message.bbid !== "0") {
      obj.bbid = message.bbid;
    }
    if (message.createTime !== undefined) {
      obj.createTime = message.createTime;
    }
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    if (message.reportTime !== undefined) {
      obj.reportTime = message.reportTime;
    }
    if (message.botId !== "") {
      obj.botId = message.botId;
    }
    if (message.rerunResult !== undefined) {
      obj.rerunResult = RerunTestResults.toJSON(message.rerunResult);
    }
    if (message.commit !== undefined) {
      obj.commit = GitilesCommit.toJSON(message.commit);
    }
    if (message.index !== "") {
      obj.index = message.index;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestSingleRerun>, I>>(base?: I): TestSingleRerun {
    return TestSingleRerun.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestSingleRerun>, I>>(object: I): TestSingleRerun {
    const message = createBaseTestSingleRerun() as any;
    message.bbid = object.bbid ?? "0";
    message.createTime = object.createTime ?? undefined;
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.reportTime = object.reportTime ?? undefined;
    message.botId = object.botId ?? "";
    message.rerunResult = (object.rerunResult !== undefined && object.rerunResult !== null)
      ? RerunTestResults.fromPartial(object.rerunResult)
      : undefined;
    message.commit = (object.commit !== undefined && object.commit !== null)
      ? GitilesCommit.fromPartial(object.commit)
      : undefined;
    message.index = object.index ?? "";
    return message;
  },
};

function createBaseRerunTestResults(): RerunTestResults {
  return { results: [], rerunStatus: 0 };
}

export const RerunTestResults = {
  encode(message: RerunTestResults, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.results) {
      RerunTestSingleResult.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.rerunStatus !== 0) {
      writer.uint32(16).int32(message.rerunStatus);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RerunTestResults {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRerunTestResults() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.results.push(RerunTestSingleResult.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.rerunStatus = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): RerunTestResults {
    return {
      results: globalThis.Array.isArray(object?.results)
        ? object.results.map((e: any) => RerunTestSingleResult.fromJSON(e))
        : [],
      rerunStatus: isSet(object.rerunStatus) ? rerunStatusFromJSON(object.rerunStatus) : 0,
    };
  },

  toJSON(message: RerunTestResults): unknown {
    const obj: any = {};
    if (message.results?.length) {
      obj.results = message.results.map((e) => RerunTestSingleResult.toJSON(e));
    }
    if (message.rerunStatus !== 0) {
      obj.rerunStatus = rerunStatusToJSON(message.rerunStatus);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<RerunTestResults>, I>>(base?: I): RerunTestResults {
    return RerunTestResults.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<RerunTestResults>, I>>(object: I): RerunTestResults {
    const message = createBaseRerunTestResults() as any;
    message.results = object.results?.map((e) => RerunTestSingleResult.fromPartial(e)) || [];
    message.rerunStatus = object.rerunStatus ?? 0;
    return message;
  },
};

function createBaseRerunTestSingleResult(): RerunTestSingleResult {
  return { testId: "", variantHash: "", expectedCount: "0", unexpectedCount: "0" };
}

export const RerunTestSingleResult = {
  encode(message: RerunTestSingleResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.testId !== "") {
      writer.uint32(10).string(message.testId);
    }
    if (message.variantHash !== "") {
      writer.uint32(18).string(message.variantHash);
    }
    if (message.expectedCount !== "0") {
      writer.uint32(24).int64(message.expectedCount);
    }
    if (message.unexpectedCount !== "0") {
      writer.uint32(32).int64(message.unexpectedCount);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RerunTestSingleResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRerunTestSingleResult() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testId = reader.string();
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

          message.expectedCount = longToString(reader.int64() as Long);
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.unexpectedCount = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): RerunTestSingleResult {
    return {
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      expectedCount: isSet(object.expectedCount) ? globalThis.String(object.expectedCount) : "0",
      unexpectedCount: isSet(object.unexpectedCount) ? globalThis.String(object.unexpectedCount) : "0",
    };
  },

  toJSON(message: RerunTestSingleResult): unknown {
    const obj: any = {};
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.expectedCount !== "0") {
      obj.expectedCount = message.expectedCount;
    }
    if (message.unexpectedCount !== "0") {
      obj.unexpectedCount = message.unexpectedCount;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<RerunTestSingleResult>, I>>(base?: I): RerunTestSingleResult {
    return RerunTestSingleResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<RerunTestSingleResult>, I>>(object: I): RerunTestSingleResult {
    const message = createBaseRerunTestSingleResult() as any;
    message.testId = object.testId ?? "";
    message.variantHash = object.variantHash ?? "";
    message.expectedCount = object.expectedCount ?? "0";
    message.unexpectedCount = object.unexpectedCount ?? "0";
    return message;
  },
};

function createBaseTestSuspectVerificationDetails(): TestSuspectVerificationDetails {
  return { status: 0, suspectRerun: undefined, parentRerun: undefined };
}

export const TestSuspectVerificationDetails = {
  encode(message: TestSuspectVerificationDetails, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.status !== 0) {
      writer.uint32(8).int32(message.status);
    }
    if (message.suspectRerun !== undefined) {
      TestSingleRerun.encode(message.suspectRerun, writer.uint32(18).fork()).ldelim();
    }
    if (message.parentRerun !== undefined) {
      TestSingleRerun.encode(message.parentRerun, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestSuspectVerificationDetails {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestSuspectVerificationDetails() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.suspectRerun = TestSingleRerun.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.parentRerun = TestSingleRerun.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestSuspectVerificationDetails {
    return {
      status: isSet(object.status) ? suspectVerificationStatusFromJSON(object.status) : 0,
      suspectRerun: isSet(object.suspectRerun) ? TestSingleRerun.fromJSON(object.suspectRerun) : undefined,
      parentRerun: isSet(object.parentRerun) ? TestSingleRerun.fromJSON(object.parentRerun) : undefined,
    };
  },

  toJSON(message: TestSuspectVerificationDetails): unknown {
    const obj: any = {};
    if (message.status !== 0) {
      obj.status = suspectVerificationStatusToJSON(message.status);
    }
    if (message.suspectRerun !== undefined) {
      obj.suspectRerun = TestSingleRerun.toJSON(message.suspectRerun);
    }
    if (message.parentRerun !== undefined) {
      obj.parentRerun = TestSingleRerun.toJSON(message.parentRerun);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestSuspectVerificationDetails>, I>>(base?: I): TestSuspectVerificationDetails {
    return TestSuspectVerificationDetails.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestSuspectVerificationDetails>, I>>(
    object: I,
  ): TestSuspectVerificationDetails {
    const message = createBaseTestSuspectVerificationDetails() as any;
    message.status = object.status ?? 0;
    message.suspectRerun = (object.suspectRerun !== undefined && object.suspectRerun !== null)
      ? TestSingleRerun.fromPartial(object.suspectRerun)
      : undefined;
    message.parentRerun = (object.parentRerun !== undefined && object.parentRerun !== null)
      ? TestSingleRerun.fromPartial(object.parentRerun)
      : undefined;
    return message;
  },
};

function createBaseTestCulprit(): TestCulprit {
  return { commit: undefined, reviewUrl: "", reviewTitle: "", culpritAction: [], verificationDetails: undefined };
}

export const TestCulprit = {
  encode(message: TestCulprit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.commit !== undefined) {
      GitilesCommit.encode(message.commit, writer.uint32(10).fork()).ldelim();
    }
    if (message.reviewUrl !== "") {
      writer.uint32(18).string(message.reviewUrl);
    }
    if (message.reviewTitle !== "") {
      writer.uint32(26).string(message.reviewTitle);
    }
    for (const v of message.culpritAction) {
      CulpritAction.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    if (message.verificationDetails !== undefined) {
      TestSuspectVerificationDetails.encode(message.verificationDetails, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TestCulprit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTestCulprit() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.commit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.reviewUrl = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.reviewTitle = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.culpritAction.push(CulpritAction.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.verificationDetails = TestSuspectVerificationDetails.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TestCulprit {
    return {
      commit: isSet(object.commit) ? GitilesCommit.fromJSON(object.commit) : undefined,
      reviewUrl: isSet(object.reviewUrl) ? globalThis.String(object.reviewUrl) : "",
      reviewTitle: isSet(object.reviewTitle) ? globalThis.String(object.reviewTitle) : "",
      culpritAction: globalThis.Array.isArray(object?.culpritAction)
        ? object.culpritAction.map((e: any) => CulpritAction.fromJSON(e))
        : [],
      verificationDetails: isSet(object.verificationDetails)
        ? TestSuspectVerificationDetails.fromJSON(object.verificationDetails)
        : undefined,
    };
  },

  toJSON(message: TestCulprit): unknown {
    const obj: any = {};
    if (message.commit !== undefined) {
      obj.commit = GitilesCommit.toJSON(message.commit);
    }
    if (message.reviewUrl !== "") {
      obj.reviewUrl = message.reviewUrl;
    }
    if (message.reviewTitle !== "") {
      obj.reviewTitle = message.reviewTitle;
    }
    if (message.culpritAction?.length) {
      obj.culpritAction = message.culpritAction.map((e) => CulpritAction.toJSON(e));
    }
    if (message.verificationDetails !== undefined) {
      obj.verificationDetails = TestSuspectVerificationDetails.toJSON(message.verificationDetails);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TestCulprit>, I>>(base?: I): TestCulprit {
    return TestCulprit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TestCulprit>, I>>(object: I): TestCulprit {
    const message = createBaseTestCulprit() as any;
    message.commit = (object.commit !== undefined && object.commit !== null)
      ? GitilesCommit.fromPartial(object.commit)
      : undefined;
    message.reviewUrl = object.reviewUrl ?? "";
    message.reviewTitle = object.reviewTitle ?? "";
    message.culpritAction = object.culpritAction?.map((e) => CulpritAction.fromPartial(e)) || [];
    message.verificationDetails = (object.verificationDetails !== undefined && object.verificationDetails !== null)
      ? TestSuspectVerificationDetails.fromPartial(object.verificationDetails)
      : undefined;
    return message;
  },
};

function createBaseBatchGetTestAnalysesRequest(): BatchGetTestAnalysesRequest {
  return { project: "", testFailures: [], fields: undefined };
}

export const BatchGetTestAnalysesRequest = {
  encode(message: BatchGetTestAnalysesRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    for (const v of message.testFailures) {
      BatchGetTestAnalysesRequest_TestFailureIdentifier.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.fields !== undefined) {
      FieldMask.encode(FieldMask.wrap(message.fields), writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetTestAnalysesRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetTestAnalysesRequest() as any;
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

          message.testFailures.push(BatchGetTestAnalysesRequest_TestFailureIdentifier.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.fields = FieldMask.unwrap(FieldMask.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchGetTestAnalysesRequest {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      testFailures: globalThis.Array.isArray(object?.testFailures)
        ? object.testFailures.map((e: any) => BatchGetTestAnalysesRequest_TestFailureIdentifier.fromJSON(e))
        : [],
      fields: isSet(object.fields) ? FieldMask.unwrap(FieldMask.fromJSON(object.fields)) : undefined,
    };
  },

  toJSON(message: BatchGetTestAnalysesRequest): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.testFailures?.length) {
      obj.testFailures = message.testFailures.map((e) => BatchGetTestAnalysesRequest_TestFailureIdentifier.toJSON(e));
    }
    if (message.fields !== undefined) {
      obj.fields = FieldMask.toJSON(FieldMask.wrap(message.fields));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetTestAnalysesRequest>, I>>(base?: I): BatchGetTestAnalysesRequest {
    return BatchGetTestAnalysesRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetTestAnalysesRequest>, I>>(object: I): BatchGetTestAnalysesRequest {
    const message = createBaseBatchGetTestAnalysesRequest() as any;
    message.project = object.project ?? "";
    message.testFailures =
      object.testFailures?.map((e) => BatchGetTestAnalysesRequest_TestFailureIdentifier.fromPartial(e)) || [];
    message.fields = object.fields ?? undefined;
    return message;
  },
};

function createBaseBatchGetTestAnalysesRequest_TestFailureIdentifier(): BatchGetTestAnalysesRequest_TestFailureIdentifier {
  return { testId: "", variantHash: "", refHash: "" };
}

export const BatchGetTestAnalysesRequest_TestFailureIdentifier = {
  encode(
    message: BatchGetTestAnalysesRequest_TestFailureIdentifier,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.testId !== "") {
      writer.uint32(10).string(message.testId);
    }
    if (message.variantHash !== "") {
      writer.uint32(18).string(message.variantHash);
    }
    if (message.refHash !== "") {
      writer.uint32(26).string(message.refHash);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetTestAnalysesRequest_TestFailureIdentifier {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetTestAnalysesRequest_TestFailureIdentifier() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.variantHash = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.refHash = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchGetTestAnalysesRequest_TestFailureIdentifier {
    return {
      testId: isSet(object.testId) ? globalThis.String(object.testId) : "",
      variantHash: isSet(object.variantHash) ? globalThis.String(object.variantHash) : "",
      refHash: isSet(object.refHash) ? globalThis.String(object.refHash) : "",
    };
  },

  toJSON(message: BatchGetTestAnalysesRequest_TestFailureIdentifier): unknown {
    const obj: any = {};
    if (message.testId !== "") {
      obj.testId = message.testId;
    }
    if (message.variantHash !== "") {
      obj.variantHash = message.variantHash;
    }
    if (message.refHash !== "") {
      obj.refHash = message.refHash;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetTestAnalysesRequest_TestFailureIdentifier>, I>>(
    base?: I,
  ): BatchGetTestAnalysesRequest_TestFailureIdentifier {
    return BatchGetTestAnalysesRequest_TestFailureIdentifier.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetTestAnalysesRequest_TestFailureIdentifier>, I>>(
    object: I,
  ): BatchGetTestAnalysesRequest_TestFailureIdentifier {
    const message = createBaseBatchGetTestAnalysesRequest_TestFailureIdentifier() as any;
    message.testId = object.testId ?? "";
    message.variantHash = object.variantHash ?? "";
    message.refHash = object.refHash ?? "";
    return message;
  },
};

function createBaseBatchGetTestAnalysesResponse(): BatchGetTestAnalysesResponse {
  return { testAnalyses: [] };
}

export const BatchGetTestAnalysesResponse = {
  encode(message: BatchGetTestAnalysesResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.testAnalyses) {
      TestAnalysis.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BatchGetTestAnalysesResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBatchGetTestAnalysesResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.testAnalyses.push(TestAnalysis.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BatchGetTestAnalysesResponse {
    return {
      testAnalyses: globalThis.Array.isArray(object?.testAnalyses)
        ? object.testAnalyses.map((e: any) => TestAnalysis.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BatchGetTestAnalysesResponse): unknown {
    const obj: any = {};
    if (message.testAnalyses?.length) {
      obj.testAnalyses = message.testAnalyses.map((e) => TestAnalysis.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BatchGetTestAnalysesResponse>, I>>(base?: I): BatchGetTestAnalysesResponse {
    return BatchGetTestAnalysesResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BatchGetTestAnalysesResponse>, I>>(object: I): BatchGetTestAnalysesResponse {
    const message = createBaseBatchGetTestAnalysesResponse() as any;
    message.testAnalyses = object.testAnalyses?.map((e) => TestAnalysis.fromPartial(e)) || [];
    return message;
  },
};

/**
 * Analyses service includes all methods related to failure analyses
 * called from LUCI Bisection clients, such as SoM.
 */
export interface Analyses {
  /** GetAnalysis is used to get an analysis by analysis ID. */
  GetAnalysis(request: GetAnalysisRequest): Promise<Analysis>;
  /**
   * QueryAnalysis is used to query for the status and result of analyses.
   * The user can pass in the failure information to retrieve the analyses.
   */
  QueryAnalysis(request: QueryAnalysisRequest): Promise<QueryAnalysisResponse>;
  /**
   * ListAnalyses is used to get existing analyses.
   * Most recently created analyses are returned first.
   */
  ListAnalyses(request: ListAnalysesRequest): Promise<ListAnalysesResponse>;
  /**
   * TriggerAnalysis is used to trigger an analysis for a failed build.
   * This RPC is called from a LUCI Bisection client like SoM or Milo.
   * If an existing analysis is found for the same failure, no new analysis
   * will be triggered.
   */
  TriggerAnalysis(request: TriggerAnalysisRequest): Promise<TriggerAnalysisResponse>;
  /**
   * Update the information of an analysis,
   * e.g. update the bugs associated with an analysis.
   * Mainly used by SoM, since LUCI Bisection does not have any information
   * about bugs created by sheriffs.
   */
  UpdateAnalysis(request: UpdateAnalysisRequest): Promise<Analysis>;
  /**
   * ListTestAnalyses is used to get existing test analyses.
   * Most recently created test analyses are returned first.
   */
  ListTestAnalyses(request: ListTestAnalysesRequest): Promise<ListTestAnalysesResponse>;
  /** GetTestAnalysis is used to get a test analysis by its ID. */
  GetTestAnalysis(request: GetTestAnalysisRequest): Promise<TestAnalysis>;
  /**
   * BatchGetTestAnalyses is an RPC to batch get test analyses for test failures.
   * At this moment it only support getting the bisection for the ongoing test failure.
   * TODO(@beining): This endpoint can be extended to support returning bisection for
   * any test failure by specifying source position in the request.
   */
  BatchGetTestAnalyses(request: BatchGetTestAnalysesRequest): Promise<BatchGetTestAnalysesResponse>;
}

export const AnalysesServiceName = "luci.bisection.v1.Analyses";
export class AnalysesClientImpl implements Analyses {
  static readonly DEFAULT_SERVICE = AnalysesServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || AnalysesServiceName;
    this.rpc = rpc;
    this.GetAnalysis = this.GetAnalysis.bind(this);
    this.QueryAnalysis = this.QueryAnalysis.bind(this);
    this.ListAnalyses = this.ListAnalyses.bind(this);
    this.TriggerAnalysis = this.TriggerAnalysis.bind(this);
    this.UpdateAnalysis = this.UpdateAnalysis.bind(this);
    this.ListTestAnalyses = this.ListTestAnalyses.bind(this);
    this.GetTestAnalysis = this.GetTestAnalysis.bind(this);
    this.BatchGetTestAnalyses = this.BatchGetTestAnalyses.bind(this);
  }
  GetAnalysis(request: GetAnalysisRequest): Promise<Analysis> {
    const data = GetAnalysisRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetAnalysis", data);
    return promise.then((data) => Analysis.fromJSON(data));
  }

  QueryAnalysis(request: QueryAnalysisRequest): Promise<QueryAnalysisResponse> {
    const data = QueryAnalysisRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "QueryAnalysis", data);
    return promise.then((data) => QueryAnalysisResponse.fromJSON(data));
  }

  ListAnalyses(request: ListAnalysesRequest): Promise<ListAnalysesResponse> {
    const data = ListAnalysesRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ListAnalyses", data);
    return promise.then((data) => ListAnalysesResponse.fromJSON(data));
  }

  TriggerAnalysis(request: TriggerAnalysisRequest): Promise<TriggerAnalysisResponse> {
    const data = TriggerAnalysisRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "TriggerAnalysis", data);
    return promise.then((data) => TriggerAnalysisResponse.fromJSON(data));
  }

  UpdateAnalysis(request: UpdateAnalysisRequest): Promise<Analysis> {
    const data = UpdateAnalysisRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "UpdateAnalysis", data);
    return promise.then((data) => Analysis.fromJSON(data));
  }

  ListTestAnalyses(request: ListTestAnalysesRequest): Promise<ListTestAnalysesResponse> {
    const data = ListTestAnalysesRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "ListTestAnalyses", data);
    return promise.then((data) => ListTestAnalysesResponse.fromJSON(data));
  }

  GetTestAnalysis(request: GetTestAnalysisRequest): Promise<TestAnalysis> {
    const data = GetTestAnalysisRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetTestAnalysis", data);
    return promise.then((data) => TestAnalysis.fromJSON(data));
  }

  BatchGetTestAnalyses(request: BatchGetTestAnalysesRequest): Promise<BatchGetTestAnalysesResponse> {
    const data = BatchGetTestAnalysesRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "BatchGetTestAnalyses", data);
    return promise.then((data) => BatchGetTestAnalysesResponse.fromJSON(data));
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
