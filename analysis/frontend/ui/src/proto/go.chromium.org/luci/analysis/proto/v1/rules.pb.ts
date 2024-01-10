/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { FieldMask } from "../../../../../google/protobuf/field_mask.pb";
import { Timestamp } from "../../../../../google/protobuf/timestamp.pb";
import { AssociatedBug, ClusterId } from "./common.pb";

export const protobufPackage = "luci.analysis.v1";

/** A rule associating failures with a bug. */
export interface Rule {
  /**
   * The resource name of the failure association rule.
   * Can be used to refer to this rule, e.g. in Rules.Get RPC.
   * Format: projects/{project}/rules/{rule_id}.
   * See also https://google.aip.dev/122.
   */
  readonly name: string;
  /** The LUCI Project for which this rule is defined. */
  readonly project: string;
  /**
   * The unique identifier for the failure association rule,
   * as 32 lowercase hexadecimal characters.
   */
  readonly ruleId: string;
  /**
   * The rule predicate, defining which failures are being associated.
   * For example, 'reason LIKE "Some error: %"'.
   *
   * analysis/internal/clustering/rules/lang/lang.go contains the
   * EBNF grammar for the language used to define rule predicates;
   * it is a subset of Google Standard SQL.
   *
   * The maximum allowed length is 65536 characters.
   */
  readonly ruleDefinition: string;
  /** The bug that the failures are associated with. */
  readonly bug:
    | AssociatedBug
    | undefined;
  /**
   * Whether the bug should be updated by LUCI Analysis, and whether
   * failures should still be matched against the rule.
   */
  readonly isActive: boolean;
  /**
   * Whether LUCI Analysis should manage the priority and verified status
   * of the associated bug based on the impact established via this rule.
   */
  readonly isManagingBug: boolean;
  /**
   * Determines whether LUCI Analysis is managing the bug priority updates
   * of the bug.
   */
  readonly isManagingBugPriority: boolean;
  /** Output Only. The time is_managing_bug_priority was last updated. */
  readonly isManagingBugPriorityLastUpdateTime:
    | string
    | undefined;
  /**
   * The suggested cluster this rule was created from (if any).
   * Until re-clustering is complete and has reduced the residual impact
   * of the source cluster, this cluster ID tells bug filing to ignore
   * the source cluster when determining whether new bugs need to be filed.
   * Immutable after creation.
   */
  readonly sourceCluster:
    | ClusterId
    | undefined;
  /**
   * Bug management state.
   * System controlled data, cannot be modified by the user.
   */
  readonly bugManagementState:
    | BugManagementState
    | undefined;
  /** The time the rule was created. */
  readonly createTime:
    | string
    | undefined;
  /**
   * The user which created the rule.
   * This could be an email address or the value 'system' (for rules
   * automaticatically created by LUCI Analysis itself).
   * This value may not be available, as its disclosure is limited
   * to Googlers only and is subject to automatic deletion after 30 days.
   */
  readonly createUser: string;
  /**
   * The last time an auditable field was updated. An auditable field
   * is any field other than a system controlled data field.
   */
  readonly lastAuditableUpdateTime:
    | string
    | undefined;
  /**
   * The last user which updated an auditable field. An auditable field
   * is any field other than a system controlled data field.
   * This could be an email address or the value 'system' (for rules
   * automaticatically modified by LUCI Analysis itself).
   * This value may not be available, as its disclosure is limited
   * to Googlers only and is subject to automatic deletion after 30 days.
   */
  readonly lastAuditableUpdateUser: string;
  /** The time the rule was last updated. */
  readonly lastUpdateTime:
    | string
    | undefined;
  /**
   * The time the rule was last updated in a way that caused the
   * matched failures to change, i.e. because of a change to rule_definition
   * or is_active. (By contrast, updating the associated bug does NOT change
   * the matched failures, so does NOT update this field.)
   * Output only.
   */
  readonly predicateLastUpdateTime:
    | string
    | undefined;
  /**
   * This checksum is computed by the server based on the value of other
   * fields, and may be sent on update requests to ensure the client
   * has an up-to-date value before proceeding.
   * See also https://google.aip.dev/154.
   */
  readonly etag: string;
}

/** BugManagementState is the state of bug management for a rule. */
export interface BugManagementState {
  /** The state of each bug management policy. */
  readonly policyState: readonly BugManagementState_PolicyState[];
}

/** The state of a bug management policy for a rule. */
export interface BugManagementState_PolicyState {
  /** The identifier of the bug management policy. */
  readonly policyId: string;
  /**
   * Whether the given policy is active for the rule.
   * Updated on every bug-filing run as follows:
   * - Set to true if the policy activation criteria was met.
   * - Set to false if the policy deactivation criteria was met.
   */
  readonly isActive: boolean;
  /**
   * The last time the policy was made active.
   * Allows detecting if policy is made active for the first time (as a
   * zero last_activation_time indicates the policy was never active).
   * Allows UI to filter to showing policies that were at least once active.
   * Allows UI to sort which policy was most recently active.
   * Allows UI to show when a policy last activated.
   */
  readonly lastActivationTime:
    | string
    | undefined;
  /**
   * The last time the policy was made inactive.
   * Allows UI to show when a policy last deactivated.
   */
  readonly lastDeactivationTime: string | undefined;
}

export interface GetRuleRequest {
  /**
   * The name of the rule to retrieve.
   * Format: projects/{project}/rules/{rule_id}.
   */
  readonly name: string;
}

export interface ListRulesRequest {
  /**
   * The parent, which owns this collection of rules.
   * Format: projects/{project}.
   */
  readonly parent: string;
}

export interface ListRulesResponse {
  /** The rules. */
  readonly rules: readonly Rule[];
}

export interface CreateRuleRequest {
  /**
   * The parent resource where the rule will be created.
   * Format: projects/{project}.
   */
  readonly parent: string;
  /** The rule to create. */
  readonly rule: Rule | undefined;
}

export interface UpdateRuleRequest {
  /**
   * The rule to update.
   *
   * The rule's `name` field is used to identify the book to update.
   * Format: projects/{project}/rules/{rule_id}.
   */
  readonly rule:
    | Rule
    | undefined;
  /** The list of fields to update. */
  readonly updateMask:
    | readonly string[]
    | undefined;
  /**
   * The current etag of the rule.
   * If an etag is provided and does not match the current etag of the rule,
   * update will be blocked and an ABORTED error will be returned.
   */
  readonly etag: string;
}

export interface LookupBugRequest {
  /**
   * System is the bug tracking system of the bug. This is either
   * "monorail" or "buganizer".
   */
  readonly system: string;
  /**
   * Id is the bug tracking system-specific identity of the bug.
   * For monorail, the scheme is {project}/{numeric_id}, for
   * buganizer the scheme is {numeric_id}.
   */
  readonly id: string;
}

export interface LookupBugResponse {
  /**
   * The rules corresponding to the requested bug.
   * Format: projects/{project}/rules/{rule_id}.
   */
  readonly rules: readonly string[];
}

function createBaseRule(): Rule {
  return {
    name: "",
    project: "",
    ruleId: "",
    ruleDefinition: "",
    bug: undefined,
    isActive: false,
    isManagingBug: false,
    isManagingBugPriority: false,
    isManagingBugPriorityLastUpdateTime: undefined,
    sourceCluster: undefined,
    bugManagementState: undefined,
    createTime: undefined,
    createUser: "",
    lastAuditableUpdateTime: undefined,
    lastAuditableUpdateUser: "",
    lastUpdateTime: undefined,
    predicateLastUpdateTime: undefined,
    etag: "",
  };
}

export const Rule = {
  encode(message: Rule, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.ruleId !== "") {
      writer.uint32(26).string(message.ruleId);
    }
    if (message.ruleDefinition !== "") {
      writer.uint32(34).string(message.ruleDefinition);
    }
    if (message.bug !== undefined) {
      AssociatedBug.encode(message.bug, writer.uint32(42).fork()).ldelim();
    }
    if (message.isActive === true) {
      writer.uint32(48).bool(message.isActive);
    }
    if (message.isManagingBug === true) {
      writer.uint32(112).bool(message.isManagingBug);
    }
    if (message.isManagingBugPriority === true) {
      writer.uint32(120).bool(message.isManagingBugPriority);
    }
    if (message.isManagingBugPriorityLastUpdateTime !== undefined) {
      Timestamp.encode(toTimestamp(message.isManagingBugPriorityLastUpdateTime), writer.uint32(130).fork()).ldelim();
    }
    if (message.sourceCluster !== undefined) {
      ClusterId.encode(message.sourceCluster, writer.uint32(58).fork()).ldelim();
    }
    if (message.bugManagementState !== undefined) {
      BugManagementState.encode(message.bugManagementState, writer.uint32(138).fork()).ldelim();
    }
    if (message.createTime !== undefined) {
      Timestamp.encode(toTimestamp(message.createTime), writer.uint32(66).fork()).ldelim();
    }
    if (message.createUser !== "") {
      writer.uint32(74).string(message.createUser);
    }
    if (message.lastAuditableUpdateTime !== undefined) {
      Timestamp.encode(toTimestamp(message.lastAuditableUpdateTime), writer.uint32(146).fork()).ldelim();
    }
    if (message.lastAuditableUpdateUser !== "") {
      writer.uint32(154).string(message.lastAuditableUpdateUser);
    }
    if (message.lastUpdateTime !== undefined) {
      Timestamp.encode(toTimestamp(message.lastUpdateTime), writer.uint32(82).fork()).ldelim();
    }
    if (message.predicateLastUpdateTime !== undefined) {
      Timestamp.encode(toTimestamp(message.predicateLastUpdateTime), writer.uint32(106).fork()).ldelim();
    }
    if (message.etag !== "") {
      writer.uint32(98).string(message.etag);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Rule {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRule() as any;
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

          message.ruleId = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.ruleDefinition = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.bug = AssociatedBug.decode(reader, reader.uint32());
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.isActive = reader.bool();
          continue;
        case 14:
          if (tag !== 112) {
            break;
          }

          message.isManagingBug = reader.bool();
          continue;
        case 15:
          if (tag !== 120) {
            break;
          }

          message.isManagingBugPriority = reader.bool();
          continue;
        case 16:
          if (tag !== 130) {
            break;
          }

          message.isManagingBugPriorityLastUpdateTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.sourceCluster = ClusterId.decode(reader, reader.uint32());
          continue;
        case 17:
          if (tag !== 138) {
            break;
          }

          message.bugManagementState = BugManagementState.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.createTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.createUser = reader.string();
          continue;
        case 18:
          if (tag !== 146) {
            break;
          }

          message.lastAuditableUpdateTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 19:
          if (tag !== 154) {
            break;
          }

          message.lastAuditableUpdateUser = reader.string();
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.lastUpdateTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 13:
          if (tag !== 106) {
            break;
          }

          message.predicateLastUpdateTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 12:
          if (tag !== 98) {
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

  fromJSON(object: any): Rule {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      ruleId: isSet(object.ruleId) ? globalThis.String(object.ruleId) : "",
      ruleDefinition: isSet(object.ruleDefinition) ? globalThis.String(object.ruleDefinition) : "",
      bug: isSet(object.bug) ? AssociatedBug.fromJSON(object.bug) : undefined,
      isActive: isSet(object.isActive) ? globalThis.Boolean(object.isActive) : false,
      isManagingBug: isSet(object.isManagingBug) ? globalThis.Boolean(object.isManagingBug) : false,
      isManagingBugPriority: isSet(object.isManagingBugPriority)
        ? globalThis.Boolean(object.isManagingBugPriority)
        : false,
      isManagingBugPriorityLastUpdateTime: isSet(object.isManagingBugPriorityLastUpdateTime)
        ? globalThis.String(object.isManagingBugPriorityLastUpdateTime)
        : undefined,
      sourceCluster: isSet(object.sourceCluster) ? ClusterId.fromJSON(object.sourceCluster) : undefined,
      bugManagementState: isSet(object.bugManagementState)
        ? BugManagementState.fromJSON(object.bugManagementState)
        : undefined,
      createTime: isSet(object.createTime) ? globalThis.String(object.createTime) : undefined,
      createUser: isSet(object.createUser) ? globalThis.String(object.createUser) : "",
      lastAuditableUpdateTime: isSet(object.lastAuditableUpdateTime)
        ? globalThis.String(object.lastAuditableUpdateTime)
        : undefined,
      lastAuditableUpdateUser: isSet(object.lastAuditableUpdateUser)
        ? globalThis.String(object.lastAuditableUpdateUser)
        : "",
      lastUpdateTime: isSet(object.lastUpdateTime) ? globalThis.String(object.lastUpdateTime) : undefined,
      predicateLastUpdateTime: isSet(object.predicateLastUpdateTime)
        ? globalThis.String(object.predicateLastUpdateTime)
        : undefined,
      etag: isSet(object.etag) ? globalThis.String(object.etag) : "",
    };
  },

  toJSON(message: Rule): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.ruleId !== "") {
      obj.ruleId = message.ruleId;
    }
    if (message.ruleDefinition !== "") {
      obj.ruleDefinition = message.ruleDefinition;
    }
    if (message.bug !== undefined) {
      obj.bug = AssociatedBug.toJSON(message.bug);
    }
    if (message.isActive === true) {
      obj.isActive = message.isActive;
    }
    if (message.isManagingBug === true) {
      obj.isManagingBug = message.isManagingBug;
    }
    if (message.isManagingBugPriority === true) {
      obj.isManagingBugPriority = message.isManagingBugPriority;
    }
    if (message.isManagingBugPriorityLastUpdateTime !== undefined) {
      obj.isManagingBugPriorityLastUpdateTime = message.isManagingBugPriorityLastUpdateTime;
    }
    if (message.sourceCluster !== undefined) {
      obj.sourceCluster = ClusterId.toJSON(message.sourceCluster);
    }
    if (message.bugManagementState !== undefined) {
      obj.bugManagementState = BugManagementState.toJSON(message.bugManagementState);
    }
    if (message.createTime !== undefined) {
      obj.createTime = message.createTime;
    }
    if (message.createUser !== "") {
      obj.createUser = message.createUser;
    }
    if (message.lastAuditableUpdateTime !== undefined) {
      obj.lastAuditableUpdateTime = message.lastAuditableUpdateTime;
    }
    if (message.lastAuditableUpdateUser !== "") {
      obj.lastAuditableUpdateUser = message.lastAuditableUpdateUser;
    }
    if (message.lastUpdateTime !== undefined) {
      obj.lastUpdateTime = message.lastUpdateTime;
    }
    if (message.predicateLastUpdateTime !== undefined) {
      obj.predicateLastUpdateTime = message.predicateLastUpdateTime;
    }
    if (message.etag !== "") {
      obj.etag = message.etag;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Rule>, I>>(base?: I): Rule {
    return Rule.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Rule>, I>>(object: I): Rule {
    const message = createBaseRule() as any;
    message.name = object.name ?? "";
    message.project = object.project ?? "";
    message.ruleId = object.ruleId ?? "";
    message.ruleDefinition = object.ruleDefinition ?? "";
    message.bug = (object.bug !== undefined && object.bug !== null) ? AssociatedBug.fromPartial(object.bug) : undefined;
    message.isActive = object.isActive ?? false;
    message.isManagingBug = object.isManagingBug ?? false;
    message.isManagingBugPriority = object.isManagingBugPriority ?? false;
    message.isManagingBugPriorityLastUpdateTime = object.isManagingBugPriorityLastUpdateTime ?? undefined;
    message.sourceCluster = (object.sourceCluster !== undefined && object.sourceCluster !== null)
      ? ClusterId.fromPartial(object.sourceCluster)
      : undefined;
    message.bugManagementState = (object.bugManagementState !== undefined && object.bugManagementState !== null)
      ? BugManagementState.fromPartial(object.bugManagementState)
      : undefined;
    message.createTime = object.createTime ?? undefined;
    message.createUser = object.createUser ?? "";
    message.lastAuditableUpdateTime = object.lastAuditableUpdateTime ?? undefined;
    message.lastAuditableUpdateUser = object.lastAuditableUpdateUser ?? "";
    message.lastUpdateTime = object.lastUpdateTime ?? undefined;
    message.predicateLastUpdateTime = object.predicateLastUpdateTime ?? undefined;
    message.etag = object.etag ?? "";
    return message;
  },
};

function createBaseBugManagementState(): BugManagementState {
  return { policyState: [] };
}

export const BugManagementState = {
  encode(message: BugManagementState, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.policyState) {
      BugManagementState_PolicyState.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugManagementState {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugManagementState() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.policyState.push(BugManagementState_PolicyState.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugManagementState {
    return {
      policyState: globalThis.Array.isArray(object?.policyState)
        ? object.policyState.map((e: any) => BugManagementState_PolicyState.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BugManagementState): unknown {
    const obj: any = {};
    if (message.policyState?.length) {
      obj.policyState = message.policyState.map((e) => BugManagementState_PolicyState.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugManagementState>, I>>(base?: I): BugManagementState {
    return BugManagementState.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugManagementState>, I>>(object: I): BugManagementState {
    const message = createBaseBugManagementState() as any;
    message.policyState = object.policyState?.map((e) => BugManagementState_PolicyState.fromPartial(e)) || [];
    return message;
  },
};

function createBaseBugManagementState_PolicyState(): BugManagementState_PolicyState {
  return { policyId: "", isActive: false, lastActivationTime: undefined, lastDeactivationTime: undefined };
}

export const BugManagementState_PolicyState = {
  encode(message: BugManagementState_PolicyState, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.policyId !== "") {
      writer.uint32(10).string(message.policyId);
    }
    if (message.isActive === true) {
      writer.uint32(16).bool(message.isActive);
    }
    if (message.lastActivationTime !== undefined) {
      Timestamp.encode(toTimestamp(message.lastActivationTime), writer.uint32(26).fork()).ldelim();
    }
    if (message.lastDeactivationTime !== undefined) {
      Timestamp.encode(toTimestamp(message.lastDeactivationTime), writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugManagementState_PolicyState {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugManagementState_PolicyState() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.policyId = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.isActive = reader.bool();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.lastActivationTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.lastDeactivationTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugManagementState_PolicyState {
    return {
      policyId: isSet(object.policyId) ? globalThis.String(object.policyId) : "",
      isActive: isSet(object.isActive) ? globalThis.Boolean(object.isActive) : false,
      lastActivationTime: isSet(object.lastActivationTime) ? globalThis.String(object.lastActivationTime) : undefined,
      lastDeactivationTime: isSet(object.lastDeactivationTime)
        ? globalThis.String(object.lastDeactivationTime)
        : undefined,
    };
  },

  toJSON(message: BugManagementState_PolicyState): unknown {
    const obj: any = {};
    if (message.policyId !== "") {
      obj.policyId = message.policyId;
    }
    if (message.isActive === true) {
      obj.isActive = message.isActive;
    }
    if (message.lastActivationTime !== undefined) {
      obj.lastActivationTime = message.lastActivationTime;
    }
    if (message.lastDeactivationTime !== undefined) {
      obj.lastDeactivationTime = message.lastDeactivationTime;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugManagementState_PolicyState>, I>>(base?: I): BugManagementState_PolicyState {
    return BugManagementState_PolicyState.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugManagementState_PolicyState>, I>>(
    object: I,
  ): BugManagementState_PolicyState {
    const message = createBaseBugManagementState_PolicyState() as any;
    message.policyId = object.policyId ?? "";
    message.isActive = object.isActive ?? false;
    message.lastActivationTime = object.lastActivationTime ?? undefined;
    message.lastDeactivationTime = object.lastDeactivationTime ?? undefined;
    return message;
  },
};

function createBaseGetRuleRequest(): GetRuleRequest {
  return { name: "" };
}

export const GetRuleRequest = {
  encode(message: GetRuleRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetRuleRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetRuleRequest() as any;
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

  fromJSON(object: any): GetRuleRequest {
    return { name: isSet(object.name) ? globalThis.String(object.name) : "" };
  },

  toJSON(message: GetRuleRequest): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetRuleRequest>, I>>(base?: I): GetRuleRequest {
    return GetRuleRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetRuleRequest>, I>>(object: I): GetRuleRequest {
    const message = createBaseGetRuleRequest() as any;
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseListRulesRequest(): ListRulesRequest {
  return { parent: "" };
}

export const ListRulesRequest = {
  encode(message: ListRulesRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.parent !== "") {
      writer.uint32(10).string(message.parent);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListRulesRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListRulesRequest() as any;
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

  fromJSON(object: any): ListRulesRequest {
    return { parent: isSet(object.parent) ? globalThis.String(object.parent) : "" };
  },

  toJSON(message: ListRulesRequest): unknown {
    const obj: any = {};
    if (message.parent !== "") {
      obj.parent = message.parent;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListRulesRequest>, I>>(base?: I): ListRulesRequest {
    return ListRulesRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListRulesRequest>, I>>(object: I): ListRulesRequest {
    const message = createBaseListRulesRequest() as any;
    message.parent = object.parent ?? "";
    return message;
  },
};

function createBaseListRulesResponse(): ListRulesResponse {
  return { rules: [] };
}

export const ListRulesResponse = {
  encode(message: ListRulesResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.rules) {
      Rule.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListRulesResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListRulesResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.rules.push(Rule.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListRulesResponse {
    return { rules: globalThis.Array.isArray(object?.rules) ? object.rules.map((e: any) => Rule.fromJSON(e)) : [] };
  },

  toJSON(message: ListRulesResponse): unknown {
    const obj: any = {};
    if (message.rules?.length) {
      obj.rules = message.rules.map((e) => Rule.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListRulesResponse>, I>>(base?: I): ListRulesResponse {
    return ListRulesResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListRulesResponse>, I>>(object: I): ListRulesResponse {
    const message = createBaseListRulesResponse() as any;
    message.rules = object.rules?.map((e) => Rule.fromPartial(e)) || [];
    return message;
  },
};

function createBaseCreateRuleRequest(): CreateRuleRequest {
  return { parent: "", rule: undefined };
}

export const CreateRuleRequest = {
  encode(message: CreateRuleRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.parent !== "") {
      writer.uint32(10).string(message.parent);
    }
    if (message.rule !== undefined) {
      Rule.encode(message.rule, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CreateRuleRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCreateRuleRequest() as any;
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
          if (tag !== 18) {
            break;
          }

          message.rule = Rule.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): CreateRuleRequest {
    return {
      parent: isSet(object.parent) ? globalThis.String(object.parent) : "",
      rule: isSet(object.rule) ? Rule.fromJSON(object.rule) : undefined,
    };
  },

  toJSON(message: CreateRuleRequest): unknown {
    const obj: any = {};
    if (message.parent !== "") {
      obj.parent = message.parent;
    }
    if (message.rule !== undefined) {
      obj.rule = Rule.toJSON(message.rule);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<CreateRuleRequest>, I>>(base?: I): CreateRuleRequest {
    return CreateRuleRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<CreateRuleRequest>, I>>(object: I): CreateRuleRequest {
    const message = createBaseCreateRuleRequest() as any;
    message.parent = object.parent ?? "";
    message.rule = (object.rule !== undefined && object.rule !== null) ? Rule.fromPartial(object.rule) : undefined;
    return message;
  },
};

function createBaseUpdateRuleRequest(): UpdateRuleRequest {
  return { rule: undefined, updateMask: undefined, etag: "" };
}

export const UpdateRuleRequest = {
  encode(message: UpdateRuleRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.rule !== undefined) {
      Rule.encode(message.rule, writer.uint32(10).fork()).ldelim();
    }
    if (message.updateMask !== undefined) {
      FieldMask.encode(FieldMask.wrap(message.updateMask), writer.uint32(18).fork()).ldelim();
    }
    if (message.etag !== "") {
      writer.uint32(26).string(message.etag);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): UpdateRuleRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseUpdateRuleRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.rule = Rule.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.updateMask = FieldMask.unwrap(FieldMask.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
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

  fromJSON(object: any): UpdateRuleRequest {
    return {
      rule: isSet(object.rule) ? Rule.fromJSON(object.rule) : undefined,
      updateMask: isSet(object.updateMask) ? FieldMask.unwrap(FieldMask.fromJSON(object.updateMask)) : undefined,
      etag: isSet(object.etag) ? globalThis.String(object.etag) : "",
    };
  },

  toJSON(message: UpdateRuleRequest): unknown {
    const obj: any = {};
    if (message.rule !== undefined) {
      obj.rule = Rule.toJSON(message.rule);
    }
    if (message.updateMask !== undefined) {
      obj.updateMask = FieldMask.toJSON(FieldMask.wrap(message.updateMask));
    }
    if (message.etag !== "") {
      obj.etag = message.etag;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<UpdateRuleRequest>, I>>(base?: I): UpdateRuleRequest {
    return UpdateRuleRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<UpdateRuleRequest>, I>>(object: I): UpdateRuleRequest {
    const message = createBaseUpdateRuleRequest() as any;
    message.rule = (object.rule !== undefined && object.rule !== null) ? Rule.fromPartial(object.rule) : undefined;
    message.updateMask = object.updateMask ?? undefined;
    message.etag = object.etag ?? "";
    return message;
  },
};

function createBaseLookupBugRequest(): LookupBugRequest {
  return { system: "", id: "" };
}

export const LookupBugRequest = {
  encode(message: LookupBugRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.system !== "") {
      writer.uint32(10).string(message.system);
    }
    if (message.id !== "") {
      writer.uint32(18).string(message.id);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LookupBugRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLookupBugRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.system = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.id = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): LookupBugRequest {
    return {
      system: isSet(object.system) ? globalThis.String(object.system) : "",
      id: isSet(object.id) ? globalThis.String(object.id) : "",
    };
  },

  toJSON(message: LookupBugRequest): unknown {
    const obj: any = {};
    if (message.system !== "") {
      obj.system = message.system;
    }
    if (message.id !== "") {
      obj.id = message.id;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<LookupBugRequest>, I>>(base?: I): LookupBugRequest {
    return LookupBugRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<LookupBugRequest>, I>>(object: I): LookupBugRequest {
    const message = createBaseLookupBugRequest() as any;
    message.system = object.system ?? "";
    message.id = object.id ?? "";
    return message;
  },
};

function createBaseLookupBugResponse(): LookupBugResponse {
  return { rules: [] };
}

export const LookupBugResponse = {
  encode(message: LookupBugResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.rules) {
      writer.uint32(18).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): LookupBugResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLookupBugResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 2:
          if (tag !== 18) {
            break;
          }

          message.rules.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): LookupBugResponse {
    return { rules: globalThis.Array.isArray(object?.rules) ? object.rules.map((e: any) => globalThis.String(e)) : [] };
  },

  toJSON(message: LookupBugResponse): unknown {
    const obj: any = {};
    if (message.rules?.length) {
      obj.rules = message.rules;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<LookupBugResponse>, I>>(base?: I): LookupBugResponse {
    return LookupBugResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<LookupBugResponse>, I>>(object: I): LookupBugResponse {
    const message = createBaseLookupBugResponse() as any;
    message.rules = object.rules?.map((e) => e) || [];
    return message;
  },
};

/**
 * Provides methods to manipulate rules in LUCI Analysis, used to associate
 * failures with bugs.
 */
export interface Rules {
  /**
   * Retrieves a rule.
   * Designed to conform to https://google.aip.dev/131.
   */
  get(request: GetRuleRequest): Promise<Rule>;
  /**
   * Lists rules.
   * TODO: implement pagination to make this
   * RPC compliant with https://google.aip.dev/132.
   * This RPC is incomplete. Future breaking changes are
   * expressly flagged.
   */
  list(request: ListRulesRequest): Promise<ListRulesResponse>;
  /**
   * Creates a new rule.
   * Designed to conform to https://google.aip.dev/133.
   */
  create(request: CreateRuleRequest): Promise<Rule>;
  /**
   * Updates a rule.
   * Designed to conform to https://google.aip.dev/134.
   */
  update(request: UpdateRuleRequest): Promise<Rule>;
  /**
   * Looks up the rule associated with a given bug, without knowledge
   * of the LUCI project the rule is in.
   * Designed to conform to https://google.aip.dev/136.
   */
  lookupBug(request: LookupBugRequest): Promise<LookupBugResponse>;
}

export const RulesServiceName = "luci.analysis.v1.Rules";
export class RulesClientImpl implements Rules {
  static readonly DEFAULT_SERVICE = RulesServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || RulesServiceName;
    this.rpc = rpc;
    this.get = this.get.bind(this);
    this.list = this.list.bind(this);
    this.create = this.create.bind(this);
    this.update = this.update.bind(this);
    this.lookupBug = this.lookupBug.bind(this);
  }
  get(request: GetRuleRequest): Promise<Rule> {
    const data = GetRuleRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "Get", data);
    return promise.then((data) => Rule.fromJSON(data));
  }

  list(request: ListRulesRequest): Promise<ListRulesResponse> {
    const data = ListRulesRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "List", data);
    return promise.then((data) => ListRulesResponse.fromJSON(data));
  }

  create(request: CreateRuleRequest): Promise<Rule> {
    const data = CreateRuleRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "Create", data);
    return promise.then((data) => Rule.fromJSON(data));
  }

  update(request: UpdateRuleRequest): Promise<Rule> {
    const data = UpdateRuleRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "Update", data);
    return promise.then((data) => Rule.fromJSON(data));
  }

  lookupBug(request: LookupBugRequest): Promise<LookupBugResponse> {
    const data = LookupBugRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "LookupBug", data);
    return promise.then((data) => LookupBugResponse.fromJSON(data));
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
