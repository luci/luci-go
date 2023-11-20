/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { GitilesCommit } from "../../../buildbucket/proto/common.pb";
import { SuspectVerificationDetails } from "./common.pb";

export const protobufPackage = "luci.bisection.v1";

export enum CulpritActionType {
  UNSPECIFIED = 0,
  /** NO_ACTION - No action has been taken with the culprit. */
  NO_ACTION = 1,
  /** CULPRIT_AUTO_REVERTED - The culprit was auto-reverted by LUCI Bisection. */
  CULPRIT_AUTO_REVERTED = 2,
  /**
   * REVERT_CL_CREATED - The revert CL for the culprit was created.
   * Maybe waiting for a human to review or for the verification process
   * to finish.
   */
  REVERT_CL_CREATED = 3,
  /** CULPRIT_CL_COMMENTED - LUCI Bisection commented on the culprit CL. */
  CULPRIT_CL_COMMENTED = 4,
  /** BUG_COMMENTED - LUCI Bisection commented on the bug for the failure. */
  BUG_COMMENTED = 5,
  /** EXISTING_REVERT_CL_COMMENTED - LUCI Bisection commented on an existing revert CL for the culprit CL. */
  EXISTING_REVERT_CL_COMMENTED = 6,
}

export function culpritActionTypeFromJSON(object: any): CulpritActionType {
  switch (object) {
    case 0:
    case "CULPRIT_ACTION_TYPE_UNSPECIFIED":
      return CulpritActionType.UNSPECIFIED;
    case 1:
    case "NO_ACTION":
      return CulpritActionType.NO_ACTION;
    case 2:
    case "CULPRIT_AUTO_REVERTED":
      return CulpritActionType.CULPRIT_AUTO_REVERTED;
    case 3:
    case "REVERT_CL_CREATED":
      return CulpritActionType.REVERT_CL_CREATED;
    case 4:
    case "CULPRIT_CL_COMMENTED":
      return CulpritActionType.CULPRIT_CL_COMMENTED;
    case 5:
    case "BUG_COMMENTED":
      return CulpritActionType.BUG_COMMENTED;
    case 6:
    case "EXISTING_REVERT_CL_COMMENTED":
      return CulpritActionType.EXISTING_REVERT_CL_COMMENTED;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum CulpritActionType");
  }
}

export function culpritActionTypeToJSON(object: CulpritActionType): string {
  switch (object) {
    case CulpritActionType.UNSPECIFIED:
      return "CULPRIT_ACTION_TYPE_UNSPECIFIED";
    case CulpritActionType.NO_ACTION:
      return "NO_ACTION";
    case CulpritActionType.CULPRIT_AUTO_REVERTED:
      return "CULPRIT_AUTO_REVERTED";
    case CulpritActionType.REVERT_CL_CREATED:
      return "REVERT_CL_CREATED";
    case CulpritActionType.CULPRIT_CL_COMMENTED:
      return "CULPRIT_CL_COMMENTED";
    case CulpritActionType.BUG_COMMENTED:
      return "BUG_COMMENTED";
    case CulpritActionType.EXISTING_REVERT_CL_COMMENTED:
      return "EXISTING_REVERT_CL_COMMENTED";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum CulpritActionType");
  }
}

/**
 * CulpritInactionReason encapsulates common reasons for why culprits found by
 * LUCI Bisection may not have resulted in any perceivable actions.
 */
export enum CulpritInactionReason {
  UNSPECIFIED = 0,
  /** REVERTED_BY_BISECTION - The culprit has been reverted by LUCI Bisection. */
  REVERTED_BY_BISECTION = 1,
  /** REVERTED_MANUALLY - The culprit has been reverted, but not by LUCI Bisection. */
  REVERTED_MANUALLY = 2,
  /**
   * REVERT_OWNED_BY_BISECTION - The culprit has an existing revert, yet to be merged, created by
   * LUCI Bisection.
   */
  REVERT_OWNED_BY_BISECTION = 3,
  /** REVERT_HAS_COMMENT - The culprit's existing revert already has a comment from LUCI Bisection. */
  REVERT_HAS_COMMENT = 4,
  /** CULPRIT_HAS_COMMENT - The culprit already has a comment from LUCI Bisection. */
  CULPRIT_HAS_COMMENT = 5,
  /** ANALYSIS_CANCELED - The analysis that resulted in the culprit has been canceled. */
  ANALYSIS_CANCELED = 6,
  /** ACTIONS_DISABLED - Culprit actions have been disabled via configs. */
  ACTIONS_DISABLED = 7,
  /** TEST_NO_LONGER_UNEXPECTED - The test being analysed is no longer having unexpected status. */
  TEST_NO_LONGER_UNEXPECTED = 8,
}

export function culpritInactionReasonFromJSON(object: any): CulpritInactionReason {
  switch (object) {
    case 0:
    case "CULPRIT_INACTION_REASON_UNSPECIFIED":
      return CulpritInactionReason.UNSPECIFIED;
    case 1:
    case "REVERTED_BY_BISECTION":
      return CulpritInactionReason.REVERTED_BY_BISECTION;
    case 2:
    case "REVERTED_MANUALLY":
      return CulpritInactionReason.REVERTED_MANUALLY;
    case 3:
    case "REVERT_OWNED_BY_BISECTION":
      return CulpritInactionReason.REVERT_OWNED_BY_BISECTION;
    case 4:
    case "REVERT_HAS_COMMENT":
      return CulpritInactionReason.REVERT_HAS_COMMENT;
    case 5:
    case "CULPRIT_HAS_COMMENT":
      return CulpritInactionReason.CULPRIT_HAS_COMMENT;
    case 6:
    case "ANALYSIS_CANCELED":
      return CulpritInactionReason.ANALYSIS_CANCELED;
    case 7:
    case "ACTIONS_DISABLED":
      return CulpritInactionReason.ACTIONS_DISABLED;
    case 8:
    case "TEST_NO_LONGER_UNEXPECTED":
      return CulpritInactionReason.TEST_NO_LONGER_UNEXPECTED;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum CulpritInactionReason");
  }
}

export function culpritInactionReasonToJSON(object: CulpritInactionReason): string {
  switch (object) {
    case CulpritInactionReason.UNSPECIFIED:
      return "CULPRIT_INACTION_REASON_UNSPECIFIED";
    case CulpritInactionReason.REVERTED_BY_BISECTION:
      return "REVERTED_BY_BISECTION";
    case CulpritInactionReason.REVERTED_MANUALLY:
      return "REVERTED_MANUALLY";
    case CulpritInactionReason.REVERT_OWNED_BY_BISECTION:
      return "REVERT_OWNED_BY_BISECTION";
    case CulpritInactionReason.REVERT_HAS_COMMENT:
      return "REVERT_HAS_COMMENT";
    case CulpritInactionReason.CULPRIT_HAS_COMMENT:
      return "CULPRIT_HAS_COMMENT";
    case CulpritInactionReason.ANALYSIS_CANCELED:
      return "ANALYSIS_CANCELED";
    case CulpritInactionReason.ACTIONS_DISABLED:
      return "ACTIONS_DISABLED";
    case CulpritInactionReason.TEST_NO_LONGER_UNEXPECTED:
      return "TEST_NO_LONGER_UNEXPECTED";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum CulpritInactionReason");
  }
}

export interface Culprit {
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
  readonly verificationDetails: SuspectVerificationDetails | undefined;
}

/** An action that LUCI Bisection has taken with the culprit. */
export interface CulpritAction {
  readonly actionType: CulpritActionType;
  /** URL to the revert CL for the culprit. */
  readonly revertClUrl: string;
  /** URL to the bug, if action_type = BUG_COMMENTED. */
  readonly bugUrl: string;
  /** Timestamp of when the culprit action was executed. */
  readonly actionTime:
    | string
    | undefined;
  /**
   * Optional reason for why no action was taken with the culprit, if
   * action_type = NO_ACTION.
   */
  readonly inactionReason: CulpritInactionReason;
}

function createBaseCulprit(): Culprit {
  return { commit: undefined, reviewUrl: "", reviewTitle: "", culpritAction: [], verificationDetails: undefined };
}

export const Culprit = {
  encode(message: Culprit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
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
      SuspectVerificationDetails.encode(message.verificationDetails, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Culprit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCulprit() as any;
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

  fromJSON(object: any): Culprit {
    return {
      commit: isSet(object.commit) ? GitilesCommit.fromJSON(object.commit) : undefined,
      reviewUrl: isSet(object.reviewUrl) ? globalThis.String(object.reviewUrl) : "",
      reviewTitle: isSet(object.reviewTitle) ? globalThis.String(object.reviewTitle) : "",
      culpritAction: globalThis.Array.isArray(object?.culpritAction)
        ? object.culpritAction.map((e: any) => CulpritAction.fromJSON(e))
        : [],
      verificationDetails: isSet(object.verificationDetails)
        ? SuspectVerificationDetails.fromJSON(object.verificationDetails)
        : undefined,
    };
  },

  toJSON(message: Culprit): unknown {
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
      obj.verificationDetails = SuspectVerificationDetails.toJSON(message.verificationDetails);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Culprit>, I>>(base?: I): Culprit {
    return Culprit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Culprit>, I>>(object: I): Culprit {
    const message = createBaseCulprit() as any;
    message.commit = (object.commit !== undefined && object.commit !== null)
      ? GitilesCommit.fromPartial(object.commit)
      : undefined;
    message.reviewUrl = object.reviewUrl ?? "";
    message.reviewTitle = object.reviewTitle ?? "";
    message.culpritAction = object.culpritAction?.map((e) => CulpritAction.fromPartial(e)) || [];
    message.verificationDetails = (object.verificationDetails !== undefined && object.verificationDetails !== null)
      ? SuspectVerificationDetails.fromPartial(object.verificationDetails)
      : undefined;
    return message;
  },
};

function createBaseCulpritAction(): CulpritAction {
  return { actionType: 0, revertClUrl: "", bugUrl: "", actionTime: undefined, inactionReason: 0 };
}

export const CulpritAction = {
  encode(message: CulpritAction, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.actionType !== 0) {
      writer.uint32(8).int32(message.actionType);
    }
    if (message.revertClUrl !== "") {
      writer.uint32(18).string(message.revertClUrl);
    }
    if (message.bugUrl !== "") {
      writer.uint32(26).string(message.bugUrl);
    }
    if (message.actionTime !== undefined) {
      Timestamp.encode(toTimestamp(message.actionTime), writer.uint32(34).fork()).ldelim();
    }
    if (message.inactionReason !== 0) {
      writer.uint32(40).int32(message.inactionReason);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CulpritAction {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCulpritAction() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.actionType = reader.int32() as any;
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.revertClUrl = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.bugUrl = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.actionTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.inactionReason = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): CulpritAction {
    return {
      actionType: isSet(object.actionType) ? culpritActionTypeFromJSON(object.actionType) : 0,
      revertClUrl: isSet(object.revertClUrl) ? globalThis.String(object.revertClUrl) : "",
      bugUrl: isSet(object.bugUrl) ? globalThis.String(object.bugUrl) : "",
      actionTime: isSet(object.actionTime) ? globalThis.String(object.actionTime) : undefined,
      inactionReason: isSet(object.inactionReason) ? culpritInactionReasonFromJSON(object.inactionReason) : 0,
    };
  },

  toJSON(message: CulpritAction): unknown {
    const obj: any = {};
    if (message.actionType !== 0) {
      obj.actionType = culpritActionTypeToJSON(message.actionType);
    }
    if (message.revertClUrl !== "") {
      obj.revertClUrl = message.revertClUrl;
    }
    if (message.bugUrl !== "") {
      obj.bugUrl = message.bugUrl;
    }
    if (message.actionTime !== undefined) {
      obj.actionTime = message.actionTime;
    }
    if (message.inactionReason !== 0) {
      obj.inactionReason = culpritInactionReasonToJSON(message.inactionReason);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<CulpritAction>, I>>(base?: I): CulpritAction {
    return CulpritAction.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<CulpritAction>, I>>(object: I): CulpritAction {
    const message = createBaseCulpritAction() as any;
    message.actionType = object.actionType ?? 0;
    message.revertClUrl = object.revertClUrl ?? "";
    message.bugUrl = object.bugUrl ?? "";
    message.actionTime = object.actionTime ?? undefined;
    message.inactionReason = object.inactionReason ?? 0;
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
