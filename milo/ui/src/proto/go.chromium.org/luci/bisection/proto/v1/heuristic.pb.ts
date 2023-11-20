/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { GitilesCommit } from "../../../buildbucket/proto/common.pb";
import { AnalysisStatus, analysisStatusFromJSON, analysisStatusToJSON, SuspectVerificationDetails } from "./common.pb";

export const protobufPackage = "luci.bisection.v1";

export enum SuspectConfidenceLevel {
  UNSPECIFIED = 0,
  LOW = 1,
  MEDIUM = 2,
  HIGH = 3,
}

export function suspectConfidenceLevelFromJSON(object: any): SuspectConfidenceLevel {
  switch (object) {
    case 0:
    case "SUSPECT_CONFIDENCE_LEVEL_UNSPECIFIED":
      return SuspectConfidenceLevel.UNSPECIFIED;
    case 1:
    case "LOW":
      return SuspectConfidenceLevel.LOW;
    case 2:
    case "MEDIUM":
      return SuspectConfidenceLevel.MEDIUM;
    case 3:
    case "HIGH":
      return SuspectConfidenceLevel.HIGH;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum SuspectConfidenceLevel");
  }
}

export function suspectConfidenceLevelToJSON(object: SuspectConfidenceLevel): string {
  switch (object) {
    case SuspectConfidenceLevel.UNSPECIFIED:
      return "SUSPECT_CONFIDENCE_LEVEL_UNSPECIFIED";
    case SuspectConfidenceLevel.LOW:
      return "LOW";
    case SuspectConfidenceLevel.MEDIUM:
      return "MEDIUM";
    case SuspectConfidenceLevel.HIGH:
      return "HIGH";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum SuspectConfidenceLevel");
  }
}

export interface HeuristicAnalysisResult {
  /** The status of the heuristic analysis. */
  readonly status: AnalysisStatus;
  /**
   * One or more suspects of the heuristic analysis.
   * This field exists only when status = FINISHED.
   */
  readonly suspects: readonly HeuristicSuspect[];
  /** Start time of heuristic analysis. */
  readonly startTime:
    | string
    | undefined;
  /** End time of heuristic analysis. */
  readonly endTime: string | undefined;
}

export interface HeuristicSuspect {
  /** A suspect revision of heuristic analysis. */
  readonly gitilesCommit:
    | GitilesCommit
    | undefined;
  /** Review URL for the suspect commit. */
  readonly reviewUrl: string;
  /**
   * Score is an integer representing the how confident we believe the suspect
   * is indeed the culprit.
   * A higher score means a stronger signal that the suspect is responsible for
   * a failure.
   */
  readonly score: number;
  /**
   * The reason why heuristic analysis thinks the suspect caused a build
   * failure.
   */
  readonly justification: string;
  /**
   * Whether the suspect has been verified by the culprit verification
   * component.
   */
  readonly verified: boolean;
  /** The level of confidence we have for the suspect. */
  readonly confidenceLevel: SuspectConfidenceLevel;
  /** Title of the review for the suspect commit. */
  readonly reviewTitle: string;
  /** The details of suspect verification for the suspect. */
  readonly verificationDetails: SuspectVerificationDetails | undefined;
}

function createBaseHeuristicAnalysisResult(): HeuristicAnalysisResult {
  return { status: 0, suspects: [], startTime: undefined, endTime: undefined };
}

export const HeuristicAnalysisResult = {
  encode(message: HeuristicAnalysisResult, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.status !== 0) {
      writer.uint32(8).int32(message.status);
    }
    for (const v of message.suspects) {
      HeuristicSuspect.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(26).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HeuristicAnalysisResult {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHeuristicAnalysisResult() as any;
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

          message.suspects.push(HeuristicSuspect.decode(reader, reader.uint32()));
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
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): HeuristicAnalysisResult {
    return {
      status: isSet(object.status) ? analysisStatusFromJSON(object.status) : 0,
      suspects: globalThis.Array.isArray(object?.suspects)
        ? object.suspects.map((e: any) => HeuristicSuspect.fromJSON(e))
        : [],
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
    };
  },

  toJSON(message: HeuristicAnalysisResult): unknown {
    const obj: any = {};
    if (message.status !== 0) {
      obj.status = analysisStatusToJSON(message.status);
    }
    if (message.suspects?.length) {
      obj.suspects = message.suspects.map((e) => HeuristicSuspect.toJSON(e));
    }
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<HeuristicAnalysisResult>, I>>(base?: I): HeuristicAnalysisResult {
    return HeuristicAnalysisResult.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<HeuristicAnalysisResult>, I>>(object: I): HeuristicAnalysisResult {
    const message = createBaseHeuristicAnalysisResult() as any;
    message.status = object.status ?? 0;
    message.suspects = object.suspects?.map((e) => HeuristicSuspect.fromPartial(e)) || [];
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    return message;
  },
};

function createBaseHeuristicSuspect(): HeuristicSuspect {
  return {
    gitilesCommit: undefined,
    reviewUrl: "",
    score: 0,
    justification: "",
    verified: false,
    confidenceLevel: 0,
    reviewTitle: "",
    verificationDetails: undefined,
  };
}

export const HeuristicSuspect = {
  encode(message: HeuristicSuspect, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.gitilesCommit !== undefined) {
      GitilesCommit.encode(message.gitilesCommit, writer.uint32(10).fork()).ldelim();
    }
    if (message.reviewUrl !== "") {
      writer.uint32(18).string(message.reviewUrl);
    }
    if (message.score !== 0) {
      writer.uint32(24).int32(message.score);
    }
    if (message.justification !== "") {
      writer.uint32(34).string(message.justification);
    }
    if (message.verified === true) {
      writer.uint32(40).bool(message.verified);
    }
    if (message.confidenceLevel !== 0) {
      writer.uint32(48).int32(message.confidenceLevel);
    }
    if (message.reviewTitle !== "") {
      writer.uint32(58).string(message.reviewTitle);
    }
    if (message.verificationDetails !== undefined) {
      SuspectVerificationDetails.encode(message.verificationDetails, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HeuristicSuspect {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHeuristicSuspect() as any;
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
          if (tag !== 24) {
            break;
          }

          message.score = reader.int32();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.justification = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.verified = reader.bool();
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.confidenceLevel = reader.int32() as any;
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.reviewTitle = reader.string();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.verificationDetails = SuspectVerificationDetails.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): HeuristicSuspect {
    return {
      gitilesCommit: isSet(object.gitilesCommit) ? GitilesCommit.fromJSON(object.gitilesCommit) : undefined,
      reviewUrl: isSet(object.reviewUrl) ? globalThis.String(object.reviewUrl) : "",
      score: isSet(object.score) ? globalThis.Number(object.score) : 0,
      justification: isSet(object.justification) ? globalThis.String(object.justification) : "",
      verified: isSet(object.verified) ? globalThis.Boolean(object.verified) : false,
      confidenceLevel: isSet(object.confidenceLevel) ? suspectConfidenceLevelFromJSON(object.confidenceLevel) : 0,
      reviewTitle: isSet(object.reviewTitle) ? globalThis.String(object.reviewTitle) : "",
      verificationDetails: isSet(object.verificationDetails)
        ? SuspectVerificationDetails.fromJSON(object.verificationDetails)
        : undefined,
    };
  },

  toJSON(message: HeuristicSuspect): unknown {
    const obj: any = {};
    if (message.gitilesCommit !== undefined) {
      obj.gitilesCommit = GitilesCommit.toJSON(message.gitilesCommit);
    }
    if (message.reviewUrl !== "") {
      obj.reviewUrl = message.reviewUrl;
    }
    if (message.score !== 0) {
      obj.score = Math.round(message.score);
    }
    if (message.justification !== "") {
      obj.justification = message.justification;
    }
    if (message.verified === true) {
      obj.verified = message.verified;
    }
    if (message.confidenceLevel !== 0) {
      obj.confidenceLevel = suspectConfidenceLevelToJSON(message.confidenceLevel);
    }
    if (message.reviewTitle !== "") {
      obj.reviewTitle = message.reviewTitle;
    }
    if (message.verificationDetails !== undefined) {
      obj.verificationDetails = SuspectVerificationDetails.toJSON(message.verificationDetails);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<HeuristicSuspect>, I>>(base?: I): HeuristicSuspect {
    return HeuristicSuspect.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<HeuristicSuspect>, I>>(object: I): HeuristicSuspect {
    const message = createBaseHeuristicSuspect() as any;
    message.gitilesCommit = (object.gitilesCommit !== undefined && object.gitilesCommit !== null)
      ? GitilesCommit.fromPartial(object.gitilesCommit)
      : undefined;
    message.reviewUrl = object.reviewUrl ?? "";
    message.score = object.score ?? 0;
    message.justification = object.justification ?? "";
    message.verified = object.verified ?? false;
    message.confidenceLevel = object.confidenceLevel ?? 0;
    message.reviewTitle = object.reviewTitle ?? "";
    message.verificationDetails = (object.verificationDetails !== undefined && object.verificationDetails !== null)
      ? SuspectVerificationDetails.fromPartial(object.verificationDetails)
      : undefined;
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
