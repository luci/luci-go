/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { GitilesCommit } from "../../../buildbucket/proto/common.pb";
import {
  AnalysisStatus,
  analysisStatusFromJSON,
  analysisStatusToJSON,
  SingleRerun,
  SuspectVerificationDetails,
} from "./common.pb";

export const protobufPackage = "luci.bisection.v1";

export interface NthSectionAnalysisResult {
  /** The status of the nth-section analysis. */
  readonly status: AnalysisStatus;
  /** Timestamp for the start time of the nth-section analysis. */
  readonly startTime:
    | string
    | undefined;
  /** Timestamp for the last updated time of the nth-section analysis. */
  readonly lastUpdatedTime:
    | string
    | undefined;
  /** Timestamp for the end time of the nth-section analysis. */
  readonly endTime:
    | string
    | undefined;
  /** Optional, when status = FOUND. Whether the culprit has been verified. */
  readonly verified: boolean;
  /**
   * Optional, when status = RUNNING. This is the possible commit range of the
   * culprit. This will be updated as the nth-section progress.
   */
  readonly remainingNthSectionRange:
    | RegressionRange
    | undefined;
  /** Optional, when status = ERROR. The error message. */
  readonly errorMessage: string;
  /**
   * List of the reruns that have been run so far for the nth-section analysis.
   * This is useful to analyse the nth-section progress.
   * The runs are sorted by the start timestamp.
   */
  readonly reruns: readonly SingleRerun[];
  /**
   * The blame list of commits to run the nth-section analysis on.
   * The commits are sorted by recency, with the most recent commit first.
   */
  readonly blameList:
    | BlameList
    | undefined;
  /** Optional, when nth-section has found a suspect. */
  readonly suspect: NthSectionSuspect | undefined;
}

export interface BlameList {
  /** The commits in the blame list. */
  readonly commits: readonly BlameListSingleCommit[];
  /**
   * The last pass commit.
   * It is the commit right before the least recent commit in the blamelist.
   */
  readonly lastPassCommit: BlameListSingleCommit | undefined;
}

export interface BlameListSingleCommit {
  /** The commit ID. */
  readonly commit: string;
  /** Review URL for the commit. */
  readonly reviewUrl: string;
  /** Title of the review for the commit. */
  readonly reviewTitle: string;
  /**
   * Commit position of this commit.
   * This field is currently only set for test failure analysis blamelist.
   */
  readonly position: string;
  /** Commit time of this commit. */
  readonly commitTime: string | undefined;
}

export interface NthSectionSuspect {
  /**
   * A suspect revision of nth-section analysis.
   * Deprecating: use commit instead.
   * TODO(beining@): remove this field when frontend switch to use commit field.
   */
  readonly gitilesCommit:
    | GitilesCommit
    | undefined;
  /** Review URL for the commit. */
  readonly reviewUrl: string;
  /** Title of the review for the commit. */
  readonly reviewTitle: string;
  /** The details of suspect verification for the suspect. */
  readonly verificationDetails:
    | SuspectVerificationDetails
    | undefined;
  /** A suspect revision of nth-section analysis. */
  readonly commit: GitilesCommit | undefined;
}

export interface RegressionRange {
  /** The commit that is the latest known to pass. */
  readonly lastPassed:
    | GitilesCommit
    | undefined;
  /** The commit that is the earliest known to fail. */
  readonly firstFailed:
    | GitilesCommit
    | undefined;
  /**
   * How many revisions between last passed (exclusively) and first failed
   * (inclusively).
   */
  readonly numberOfRevisions: number;
}

function createBaseNthSectionAnalysisResult(): NthSectionAnalysisResult {
  return {
    status: 0,
    startTime: undefined,
    lastUpdatedTime: undefined,
    endTime: undefined,
    verified: false,
    remainingNthSectionRange: undefined,
    errorMessage: "",
    reruns: [],
    blameList: undefined,
    suspect: undefined,
  };
}

export const NthSectionAnalysisResult = {
  encode(message: NthSectionAnalysisResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.status !== 0) {
      writer.uint32(8).int32(message.status);
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(18).fork()).ldelim();
    }
    if (message.lastUpdatedTime !== undefined) {
      Timestamp.encode(toTimestamp(message.lastUpdatedTime), writer.uint32(26).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(34).fork()).ldelim();
    }
    if (message.verified === true) {
      writer.uint32(40).bool(message.verified);
    }
    if (message.remainingNthSectionRange !== undefined) {
      RegressionRange.encode(message.remainingNthSectionRange, writer.uint32(50).fork()).ldelim();
    }
    if (message.errorMessage !== "") {
      writer.uint32(58).string(message.errorMessage);
    }
    for (const v of message.reruns) {
      SingleRerun.encode(v!, writer.uint32(66).fork()).ldelim();
    }
    if (message.blameList !== undefined) {
      BlameList.encode(message.blameList, writer.uint32(74).fork()).ldelim();
    }
    if (message.suspect !== undefined) {
      NthSectionSuspect.encode(message.suspect, writer.uint32(82).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): NthSectionAnalysisResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseNthSectionAnalysisResult() as any;
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

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.lastUpdatedTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
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

          message.verified = reader.bool();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.remainingNthSectionRange = RegressionRange.decode(reader, reader.uint32());
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.errorMessage = reader.string();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.reruns.push(SingleRerun.decode(reader, reader.uint32()));
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.blameList = BlameList.decode(reader, reader.uint32());
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.suspect = NthSectionSuspect.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): NthSectionAnalysisResult {
    return {
      status: isSet(object.status) ? analysisStatusFromJSON(object.status) : 0,
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      lastUpdatedTime: isSet(object.lastUpdatedTime) ? globalThis.String(object.lastUpdatedTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      verified: isSet(object.verified) ? globalThis.Boolean(object.verified) : false,
      remainingNthSectionRange: isSet(object.remainingNthSectionRange)
        ? RegressionRange.fromJSON(object.remainingNthSectionRange)
        : undefined,
      errorMessage: isSet(object.errorMessage) ? globalThis.String(object.errorMessage) : "",
      reruns: globalThis.Array.isArray(object?.reruns) ? object.reruns.map((e: any) => SingleRerun.fromJSON(e)) : [],
      blameList: isSet(object.blameList) ? BlameList.fromJSON(object.blameList) : undefined,
      suspect: isSet(object.suspect) ? NthSectionSuspect.fromJSON(object.suspect) : undefined,
    };
  },

  toJSON(message: NthSectionAnalysisResult): unknown {
    const obj: any = {};
    if (message.status !== 0) {
      obj.status = analysisStatusToJSON(message.status);
    }
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.lastUpdatedTime !== undefined) {
      obj.lastUpdatedTime = message.lastUpdatedTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    if (message.verified === true) {
      obj.verified = message.verified;
    }
    if (message.remainingNthSectionRange !== undefined) {
      obj.remainingNthSectionRange = RegressionRange.toJSON(message.remainingNthSectionRange);
    }
    if (message.errorMessage !== "") {
      obj.errorMessage = message.errorMessage;
    }
    if (message.reruns?.length) {
      obj.reruns = message.reruns.map((e) => SingleRerun.toJSON(e));
    }
    if (message.blameList !== undefined) {
      obj.blameList = BlameList.toJSON(message.blameList);
    }
    if (message.suspect !== undefined) {
      obj.suspect = NthSectionSuspect.toJSON(message.suspect);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<NthSectionAnalysisResult>, I>>(base?: I): NthSectionAnalysisResult {
    return NthSectionAnalysisResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<NthSectionAnalysisResult>, I>>(object: I): NthSectionAnalysisResult {
    const message = createBaseNthSectionAnalysisResult() as any;
    message.status = object.status ?? 0;
    message.startTime = object.startTime ?? undefined;
    message.lastUpdatedTime = object.lastUpdatedTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.verified = object.verified ?? false;
    message.remainingNthSectionRange =
      (object.remainingNthSectionRange !== undefined && object.remainingNthSectionRange !== null)
        ? RegressionRange.fromPartial(object.remainingNthSectionRange)
        : undefined;
    message.errorMessage = object.errorMessage ?? "";
    message.reruns = object.reruns?.map((e) => SingleRerun.fromPartial(e)) || [];
    message.blameList = (object.blameList !== undefined && object.blameList !== null)
      ? BlameList.fromPartial(object.blameList)
      : undefined;
    message.suspect = (object.suspect !== undefined && object.suspect !== null)
      ? NthSectionSuspect.fromPartial(object.suspect)
      : undefined;
    return message;
  },
};

function createBaseBlameList(): BlameList {
  return { commits: [], lastPassCommit: undefined };
}

export const BlameList = {
  encode(message: BlameList, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.commits) {
      BlameListSingleCommit.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.lastPassCommit !== undefined) {
      BlameListSingleCommit.encode(message.lastPassCommit, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BlameList {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBlameList() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.commits.push(BlameListSingleCommit.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.lastPassCommit = BlameListSingleCommit.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BlameList {
    return {
      commits: globalThis.Array.isArray(object?.commits)
        ? object.commits.map((e: any) => BlameListSingleCommit.fromJSON(e))
        : [],
      lastPassCommit: isSet(object.lastPassCommit) ? BlameListSingleCommit.fromJSON(object.lastPassCommit) : undefined,
    };
  },

  toJSON(message: BlameList): unknown {
    const obj: any = {};
    if (message.commits?.length) {
      obj.commits = message.commits.map((e) => BlameListSingleCommit.toJSON(e));
    }
    if (message.lastPassCommit !== undefined) {
      obj.lastPassCommit = BlameListSingleCommit.toJSON(message.lastPassCommit);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BlameList>, I>>(base?: I): BlameList {
    return BlameList.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BlameList>, I>>(object: I): BlameList {
    const message = createBaseBlameList() as any;
    message.commits = object.commits?.map((e) => BlameListSingleCommit.fromPartial(e)) || [];
    message.lastPassCommit = (object.lastPassCommit !== undefined && object.lastPassCommit !== null)
      ? BlameListSingleCommit.fromPartial(object.lastPassCommit)
      : undefined;
    return message;
  },
};

function createBaseBlameListSingleCommit(): BlameListSingleCommit {
  return { commit: "", reviewUrl: "", reviewTitle: "", position: "0", commitTime: undefined };
}

export const BlameListSingleCommit = {
  encode(message: BlameListSingleCommit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.commit !== "") {
      writer.uint32(10).string(message.commit);
    }
    if (message.reviewUrl !== "") {
      writer.uint32(18).string(message.reviewUrl);
    }
    if (message.reviewTitle !== "") {
      writer.uint32(26).string(message.reviewTitle);
    }
    if (message.position !== "0") {
      writer.uint32(32).int64(message.position);
    }
    if (message.commitTime !== undefined) {
      Timestamp.encode(toTimestamp(message.commitTime), writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BlameListSingleCommit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBlameListSingleCommit() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.commit = reader.string();
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
          if (tag !== 32) {
            break;
          }

          message.position = longToString(reader.int64() as Long);
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.commitTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BlameListSingleCommit {
    return {
      commit: isSet(object.commit) ? globalThis.String(object.commit) : "",
      reviewUrl: isSet(object.reviewUrl) ? globalThis.String(object.reviewUrl) : "",
      reviewTitle: isSet(object.reviewTitle) ? globalThis.String(object.reviewTitle) : "",
      position: isSet(object.position) ? globalThis.String(object.position) : "0",
      commitTime: isSet(object.commitTime) ? globalThis.String(object.commitTime) : undefined,
    };
  },

  toJSON(message: BlameListSingleCommit): unknown {
    const obj: any = {};
    if (message.commit !== "") {
      obj.commit = message.commit;
    }
    if (message.reviewUrl !== "") {
      obj.reviewUrl = message.reviewUrl;
    }
    if (message.reviewTitle !== "") {
      obj.reviewTitle = message.reviewTitle;
    }
    if (message.position !== "0") {
      obj.position = message.position;
    }
    if (message.commitTime !== undefined) {
      obj.commitTime = message.commitTime;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BlameListSingleCommit>, I>>(base?: I): BlameListSingleCommit {
    return BlameListSingleCommit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BlameListSingleCommit>, I>>(object: I): BlameListSingleCommit {
    const message = createBaseBlameListSingleCommit() as any;
    message.commit = object.commit ?? "";
    message.reviewUrl = object.reviewUrl ?? "";
    message.reviewTitle = object.reviewTitle ?? "";
    message.position = object.position ?? "0";
    message.commitTime = object.commitTime ?? undefined;
    return message;
  },
};

function createBaseNthSectionSuspect(): NthSectionSuspect {
  return {
    gitilesCommit: undefined,
    reviewUrl: "",
    reviewTitle: "",
    verificationDetails: undefined,
    commit: undefined,
  };
}

export const NthSectionSuspect = {
  encode(message: NthSectionSuspect, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.gitilesCommit !== undefined) {
      GitilesCommit.encode(message.gitilesCommit, writer.uint32(10).fork()).ldelim();
    }
    if (message.reviewUrl !== "") {
      writer.uint32(18).string(message.reviewUrl);
    }
    if (message.reviewTitle !== "") {
      writer.uint32(26).string(message.reviewTitle);
    }
    if (message.verificationDetails !== undefined) {
      SuspectVerificationDetails.encode(message.verificationDetails, writer.uint32(34).fork()).ldelim();
    }
    if (message.commit !== undefined) {
      GitilesCommit.encode(message.commit, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): NthSectionSuspect {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseNthSectionSuspect() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.gitilesCommit = GitilesCommit.decode(reader, reader.uint32());
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

          message.verificationDetails = SuspectVerificationDetails.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.commit = GitilesCommit.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): NthSectionSuspect {
    return {
      gitilesCommit: isSet(object.gitilesCommit) ? GitilesCommit.fromJSON(object.gitilesCommit) : undefined,
      reviewUrl: isSet(object.reviewUrl) ? globalThis.String(object.reviewUrl) : "",
      reviewTitle: isSet(object.reviewTitle) ? globalThis.String(object.reviewTitle) : "",
      verificationDetails: isSet(object.verificationDetails)
        ? SuspectVerificationDetails.fromJSON(object.verificationDetails)
        : undefined,
      commit: isSet(object.commit) ? GitilesCommit.fromJSON(object.commit) : undefined,
    };
  },

  toJSON(message: NthSectionSuspect): unknown {
    const obj: any = {};
    if (message.gitilesCommit !== undefined) {
      obj.gitilesCommit = GitilesCommit.toJSON(message.gitilesCommit);
    }
    if (message.reviewUrl !== "") {
      obj.reviewUrl = message.reviewUrl;
    }
    if (message.reviewTitle !== "") {
      obj.reviewTitle = message.reviewTitle;
    }
    if (message.verificationDetails !== undefined) {
      obj.verificationDetails = SuspectVerificationDetails.toJSON(message.verificationDetails);
    }
    if (message.commit !== undefined) {
      obj.commit = GitilesCommit.toJSON(message.commit);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<NthSectionSuspect>, I>>(base?: I): NthSectionSuspect {
    return NthSectionSuspect.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<NthSectionSuspect>, I>>(object: I): NthSectionSuspect {
    const message = createBaseNthSectionSuspect() as any;
    message.gitilesCommit = (object.gitilesCommit !== undefined && object.gitilesCommit !== null)
      ? GitilesCommit.fromPartial(object.gitilesCommit)
      : undefined;
    message.reviewUrl = object.reviewUrl ?? "";
    message.reviewTitle = object.reviewTitle ?? "";
    message.verificationDetails = (object.verificationDetails !== undefined && object.verificationDetails !== null)
      ? SuspectVerificationDetails.fromPartial(object.verificationDetails)
      : undefined;
    message.commit = (object.commit !== undefined && object.commit !== null)
      ? GitilesCommit.fromPartial(object.commit)
      : undefined;
    return message;
  },
};

function createBaseRegressionRange(): RegressionRange {
  return { lastPassed: undefined, firstFailed: undefined, numberOfRevisions: 0 };
}

export const RegressionRange = {
  encode(message: RegressionRange, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.lastPassed !== undefined) {
      GitilesCommit.encode(message.lastPassed, writer.uint32(10).fork()).ldelim();
    }
    if (message.firstFailed !== undefined) {
      GitilesCommit.encode(message.firstFailed, writer.uint32(18).fork()).ldelim();
    }
    if (message.numberOfRevisions !== 0) {
      writer.uint32(24).int32(message.numberOfRevisions);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RegressionRange {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRegressionRange() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.lastPassed = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.firstFailed = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.numberOfRevisions = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): RegressionRange {
    return {
      lastPassed: isSet(object.lastPassed) ? GitilesCommit.fromJSON(object.lastPassed) : undefined,
      firstFailed: isSet(object.firstFailed) ? GitilesCommit.fromJSON(object.firstFailed) : undefined,
      numberOfRevisions: isSet(object.numberOfRevisions) ? globalThis.Number(object.numberOfRevisions) : 0,
    };
  },

  toJSON(message: RegressionRange): unknown {
    const obj: any = {};
    if (message.lastPassed !== undefined) {
      obj.lastPassed = GitilesCommit.toJSON(message.lastPassed);
    }
    if (message.firstFailed !== undefined) {
      obj.firstFailed = GitilesCommit.toJSON(message.firstFailed);
    }
    if (message.numberOfRevisions !== 0) {
      obj.numberOfRevisions = Math.round(message.numberOfRevisions);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<RegressionRange>, I>>(base?: I): RegressionRange {
    return RegressionRange.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<RegressionRange>, I>>(object: I): RegressionRange {
    const message = createBaseRegressionRange() as any;
    message.lastPassed = (object.lastPassed !== undefined && object.lastPassed !== null)
      ? GitilesCommit.fromPartial(object.lastPassed)
      : undefined;
    message.firstFailed = (object.firstFailed !== undefined && object.firstFailed !== null)
      ? GitilesCommit.fromPartial(object.firstFailed)
      : undefined;
    message.numberOfRevisions = object.numberOfRevisions ?? 0;
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