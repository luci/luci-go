/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Project } from "./project.pb";

export const protobufPackage = "luci.analysis.v1";

/**
 * This enum represents the Buganizer priorities.
 * It is equivalent to the one in Buganizer API.
 */
export enum BuganizerPriority {
  /** UNSPECIFIED - Priority unspecified; do not use this value. */
  UNSPECIFIED = 0,
  /** P0 - P0, Highest priority. */
  P0 = 1,
  P1 = 2,
  P2 = 3,
  P3 = 4,
  P4 = 5,
}

export function buganizerPriorityFromJSON(object: any): BuganizerPriority {
  switch (object) {
    case 0:
    case "BUGANIZER_PRIORITY_UNSPECIFIED":
      return BuganizerPriority.UNSPECIFIED;
    case 1:
    case "P0":
      return BuganizerPriority.P0;
    case 2:
    case "P1":
      return BuganizerPriority.P1;
    case 3:
    case "P2":
      return BuganizerPriority.P2;
    case 4:
    case "P3":
      return BuganizerPriority.P3;
    case 5:
    case "P4":
      return BuganizerPriority.P4;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuganizerPriority");
  }
}

export function buganizerPriorityToJSON(object: BuganizerPriority): string {
  switch (object) {
    case BuganizerPriority.UNSPECIFIED:
      return "BUGANIZER_PRIORITY_UNSPECIFIED";
    case BuganizerPriority.P0:
      return "P0";
    case BuganizerPriority.P1:
      return "P1";
    case BuganizerPriority.P2:
      return "P2";
    case BuganizerPriority.P3:
      return "P3";
    case BuganizerPriority.P4:
      return "P4";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum BuganizerPriority");
  }
}

/**
 * A request object with data to fetch the list of projects configured
 * in LUCI Analysis.
 */
export interface ListProjectsRequest {
}

/**
 * A response containing the list of projects which are are using
 * LUCI Analysis.
 */
export interface ListProjectsResponse {
  /** The list of projects using LUCI Analysis. */
  readonly projects: readonly Project[];
}

export interface GetProjectConfigRequest {
  /**
   * The name of the project configuration to retrieve.
   * Format: projects/{project}/config.
   */
  readonly name: string;
}

export interface ProjectConfig {
  /**
   * Resource name of the project configuration.
   * Format: projects/{project}/config.
   * See also https://google.aip.dev/122.
   */
  readonly name: string;
  /** Configuration for automatic bug management. */
  readonly bugManagement: BugManagement | undefined;
}

/** Settings related to bug management. */
export interface BugManagement {
  /**
   * The set of policies which control the (re-)opening, closure and
   * prioritization of bugs under the control of LUCI Analysis.
   */
  readonly policies: readonly BugManagementPolicy[];
  /** Monorail-specific bug filing configuration. */
  readonly monorail: MonorailProject | undefined;
}

/**
 * A bug management policy in LUCI Analysis.
 *
 * Bug management policies control when and how bugs are automatically
 * opened, prioritised, and verified as fixed. Each policy has a user-visible
 * identity in the UI and can post custom instructions on the bug.
 *
 * LUCI Analysis avoids filing multiple bugs for the same failures by
 * allowing multiple policies to activate on the same failure association
 * rule. The bug associated with a rule will only be verified if all policies
 * have de-activated.
 */
export interface BugManagementPolicy {
  /**
   * A unique identifier for the bug management policy.
   *
   * Policies are stateful in that LUCI Analysis tracks which bugs have met the
   * activation condition on the policy (and not since met the deactivation
   * condition).
   *
   * Changing this value changes the identity of the policy and hence results in
   * the activation state for the policy being lost for all bugs.
   *
   * Valid syntax: ^[a-z]([a-z0-9-]{0,62}[a-z0-9])?$. (Syntax designed to comply
   * with google.aip.dev/122 for resource IDs.)
   */
  readonly id: string;
  /**
   * The owners of the policy, who can be contacted if there are issues/concerns
   * about the policy. Each item in the list should be an @google.com email
   * address. At least one owner (preferably a group) is required.
   */
  readonly owners: readonly string[];
  /**
   * A short one-line description for the problem the policy identifies, which
   * will appear on the UI and in bugs comments. This is a sentence fragment
   * and not a sentence, so please do NOT include a full stop and or starting
   * capital letter.
   *
   * For example, "test variant(s) are being exonerated in presubmit".
   */
  readonly humanReadableName: string;
  /**
   * The priority of the problem this policy defines.
   *
   * If:
   * - the priority of the bug associated with a rule
   *   differs from this priority, and
   * - the policy is activate on the rule (see `metrics`), and
   * - LUCI Analysis is controlling the priority of the bug
   *   (the "Update bug priority" switch on the rule is enabled),
   * the priority of the bug will be updated to match this priority.
   *
   * Where are there multiple policies active on the same rule,
   * the highest priority (of all active policies) will be used.
   *
   * For monorail projects, the buganizer priority will be converted to the
   * equivalent monorail priority (P0 is converted to Pri-0, P1 to Pri-1,
   * P2 to Pri-2, etc.) until monorail is turned down.
   */
  readonly priority: BuganizerPriority;
  /**
   * The set of metrics which will control activation of the bug-filing policy.
   * If a policy activates on a suggested cluster, a new bug will be filed.
   * If a policy activates on an existing rule cluster, the bug will be
   * updated.
   *
   * The policy will activate if the activation threshold is met on *ANY*
   * metric, and will de-activate only if the deactivation threshold is met
   * on *ALL* metrics.
   *
   * Activation on suggested clusters will be based on the metric values after
   * excluding failures for which a bug has already been filed. This is to
   * avoid duplicate bug filing.
   */
  readonly metrics: readonly BugManagementPolicy_Metric[];
  /**
   * Expanatory text of the problem the policy identified, shown on the
   * user interface when the user requests more information. Required.
   */
  readonly explanation: BugManagementPolicy_Explanation | undefined;
}

/** A metric used to control activation of a bug-filing policy. */
export interface BugManagementPolicy_Metric {
  /**
   * The identifier of the metric.
   *
   * Full list of available metrics here:
   * https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/internal/analysis/metrics/metrics.go
   */
  readonly metricId: string;
  /**
   * The level at which the policy activates. Activation occurs if the
   * cluster impact meets or exceeds this threshold.
   * MUST imply deactivation_threshold.
   */
  readonly activationThreshold:
    | MetricThreshold
    | undefined;
  /**
   * The minimum metric level at which the policy remains active.
   * Deactivation occcurs if the cluster impact is below the de-activation
   * threshold. Deactivation_threshold should be set significantly lower
   * than activation_threshold to prevent policies repeatedly activating
   * and deactivating due to noise in the data, e.g. less tests executed
   * on weekends.
   */
  readonly deactivationThreshold: MetricThreshold | undefined;
}

/**
 * Content displayed on the user interface, to explain the problem and
 * guide a developer to fix it.
 */
export interface BugManagementPolicy_Explanation {
  /**
   * A longer human-readable description of the problem this policy
   * has identified, in HTML.
   *
   * For example, "Test variant(s) in this cluster are being exonerated
   * (ignored) in presubmit because they are too flaky or failing. This
   * means they are no longer effective at preventing the breakage of
   * the functionality the test(s) cover.".
   *
   * MUST be sanitised by UI before rendering. Sanitisation is only
   * required to support simple uses of the following tags: ul, li, a.
   */
  readonly problemHtml: string;
  /**
   * A description of how a human should go about trying to fix the
   * problem, in HTML.
   *
   * For example, "<ul>
   * <li>View recent failures</li>
   * <li><a href="http://goto.google.com/demote-from-cq">Demote</a> the test from CQ</li>
   * </ul>"
   *
   * MUST be sanitised by UI before rendering. Sanitisation is only
   * required to support simple uses of the following tags: ul, li, a.
   */
  readonly actionHtml: string;
}

/**
 * MonorailProject describes the configuration to use when filing bugs
 * into a given monorail project.
 */
export interface MonorailProject {
  /**
   * The monorail project being described.
   * E.g. "chromium".
   */
  readonly project: string;
  /**
   * The prefix that should appear when displaying bugs from the
   * given bug tracking system. E.g. "crbug.com" or "fxbug.dev".
   * If no prefix is specified, only the bug number will appear.
   * Otherwise, the supplifed prefix will appear, followed by a
   * forward slash ("/"), followed by the bug number.
   * Valid prefixes match `^[a-z0-9\-.]{0,64}$`.
   */
  readonly displayPrefix: string;
}

/**
 * MetricThreshold specifies thresholds for a particular metric.
 * The threshold is considered satisfied if any of the individual metric
 * thresholds is met or exceeded (i.e. if multiple thresholds are set, they
 * are combined using an OR-semantic). If no threshold is set, the threshold
 * as a whole is unsatisfiable.
 */
export interface MetricThreshold {
  /** The threshold for one day. */
  readonly oneDay?:
    | string
    | undefined;
  /** The threshold for three day. */
  readonly threeDay?:
    | string
    | undefined;
  /** The threshold for seven days. */
  readonly sevenDay?: string | undefined;
}

function createBaseListProjectsRequest(): ListProjectsRequest {
  return {};
}

export const ListProjectsRequest = {
  encode(_: ListProjectsRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListProjectsRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListProjectsRequest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(_: any): ListProjectsRequest {
    return {};
  },

  toJSON(_: ListProjectsRequest): unknown {
    const obj: any = {};
    return obj;
  },

  create<I extends Exact<DeepPartial<ListProjectsRequest>, I>>(base?: I): ListProjectsRequest {
    return ListProjectsRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListProjectsRequest>, I>>(_: I): ListProjectsRequest {
    const message = createBaseListProjectsRequest() as any;
    return message;
  },
};

function createBaseListProjectsResponse(): ListProjectsResponse {
  return { projects: [] };
}

export const ListProjectsResponse = {
  encode(message: ListProjectsResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.projects) {
      Project.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListProjectsResponse {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseListProjectsResponse() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.projects.push(Project.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ListProjectsResponse {
    return {
      projects: globalThis.Array.isArray(object?.projects) ? object.projects.map((e: any) => Project.fromJSON(e)) : [],
    };
  },

  toJSON(message: ListProjectsResponse): unknown {
    const obj: any = {};
    if (message.projects?.length) {
      obj.projects = message.projects.map((e) => Project.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ListProjectsResponse>, I>>(base?: I): ListProjectsResponse {
    return ListProjectsResponse.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ListProjectsResponse>, I>>(object: I): ListProjectsResponse {
    const message = createBaseListProjectsResponse() as any;
    message.projects = object.projects?.map((e) => Project.fromPartial(e)) || [];
    return message;
  },
};

function createBaseGetProjectConfigRequest(): GetProjectConfigRequest {
  return { name: "" };
}

export const GetProjectConfigRequest = {
  encode(message: GetProjectConfigRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GetProjectConfigRequest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGetProjectConfigRequest() as any;
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

  fromJSON(object: any): GetProjectConfigRequest {
    return { name: isSet(object.name) ? globalThis.String(object.name) : "" };
  },

  toJSON(message: GetProjectConfigRequest): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GetProjectConfigRequest>, I>>(base?: I): GetProjectConfigRequest {
    return GetProjectConfigRequest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GetProjectConfigRequest>, I>>(object: I): GetProjectConfigRequest {
    const message = createBaseGetProjectConfigRequest() as any;
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseProjectConfig(): ProjectConfig {
  return { name: "", bugManagement: undefined };
}

export const ProjectConfig = {
  encode(message: ProjectConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.bugManagement !== undefined) {
      BugManagement.encode(message.bugManagement, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ProjectConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseProjectConfig() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.bugManagement = BugManagement.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ProjectConfig {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      bugManagement: isSet(object.bugManagement) ? BugManagement.fromJSON(object.bugManagement) : undefined,
    };
  },

  toJSON(message: ProjectConfig): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.bugManagement !== undefined) {
      obj.bugManagement = BugManagement.toJSON(message.bugManagement);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ProjectConfig>, I>>(base?: I): ProjectConfig {
    return ProjectConfig.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ProjectConfig>, I>>(object: I): ProjectConfig {
    const message = createBaseProjectConfig() as any;
    message.name = object.name ?? "";
    message.bugManagement = (object.bugManagement !== undefined && object.bugManagement !== null)
      ? BugManagement.fromPartial(object.bugManagement)
      : undefined;
    return message;
  },
};

function createBaseBugManagement(): BugManagement {
  return { policies: [], monorail: undefined };
}

export const BugManagement = {
  encode(message: BugManagement, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.policies) {
      BugManagementPolicy.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.monorail !== undefined) {
      MonorailProject.encode(message.monorail, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugManagement {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugManagement() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.policies.push(BugManagementPolicy.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.monorail = MonorailProject.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugManagement {
    return {
      policies: globalThis.Array.isArray(object?.policies)
        ? object.policies.map((e: any) => BugManagementPolicy.fromJSON(e))
        : [],
      monorail: isSet(object.monorail) ? MonorailProject.fromJSON(object.monorail) : undefined,
    };
  },

  toJSON(message: BugManagement): unknown {
    const obj: any = {};
    if (message.policies?.length) {
      obj.policies = message.policies.map((e) => BugManagementPolicy.toJSON(e));
    }
    if (message.monorail !== undefined) {
      obj.monorail = MonorailProject.toJSON(message.monorail);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugManagement>, I>>(base?: I): BugManagement {
    return BugManagement.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugManagement>, I>>(object: I): BugManagement {
    const message = createBaseBugManagement() as any;
    message.policies = object.policies?.map((e) => BugManagementPolicy.fromPartial(e)) || [];
    message.monorail = (object.monorail !== undefined && object.monorail !== null)
      ? MonorailProject.fromPartial(object.monorail)
      : undefined;
    return message;
  },
};

function createBaseBugManagementPolicy(): BugManagementPolicy {
  return { id: "", owners: [], humanReadableName: "", priority: 0, metrics: [], explanation: undefined };
}

export const BugManagementPolicy = {
  encode(message: BugManagementPolicy, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    for (const v of message.owners) {
      writer.uint32(50).string(v!);
    }
    if (message.humanReadableName !== "") {
      writer.uint32(18).string(message.humanReadableName);
    }
    if (message.priority !== 0) {
      writer.uint32(24).int32(message.priority);
    }
    for (const v of message.metrics) {
      BugManagementPolicy_Metric.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    if (message.explanation !== undefined) {
      BugManagementPolicy_Explanation.encode(message.explanation, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugManagementPolicy {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugManagementPolicy() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.id = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.owners.push(reader.string());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.humanReadableName = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.priority = reader.int32() as any;
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.metrics.push(BugManagementPolicy_Metric.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.explanation = BugManagementPolicy_Explanation.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugManagementPolicy {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : "",
      owners: globalThis.Array.isArray(object?.owners) ? object.owners.map((e: any) => globalThis.String(e)) : [],
      humanReadableName: isSet(object.humanReadableName) ? globalThis.String(object.humanReadableName) : "",
      priority: isSet(object.priority) ? buganizerPriorityFromJSON(object.priority) : 0,
      metrics: globalThis.Array.isArray(object?.metrics)
        ? object.metrics.map((e: any) => BugManagementPolicy_Metric.fromJSON(e))
        : [],
      explanation: isSet(object.explanation) ? BugManagementPolicy_Explanation.fromJSON(object.explanation) : undefined,
    };
  },

  toJSON(message: BugManagementPolicy): unknown {
    const obj: any = {};
    if (message.id !== "") {
      obj.id = message.id;
    }
    if (message.owners?.length) {
      obj.owners = message.owners;
    }
    if (message.humanReadableName !== "") {
      obj.humanReadableName = message.humanReadableName;
    }
    if (message.priority !== 0) {
      obj.priority = buganizerPriorityToJSON(message.priority);
    }
    if (message.metrics?.length) {
      obj.metrics = message.metrics.map((e) => BugManagementPolicy_Metric.toJSON(e));
    }
    if (message.explanation !== undefined) {
      obj.explanation = BugManagementPolicy_Explanation.toJSON(message.explanation);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugManagementPolicy>, I>>(base?: I): BugManagementPolicy {
    return BugManagementPolicy.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugManagementPolicy>, I>>(object: I): BugManagementPolicy {
    const message = createBaseBugManagementPolicy() as any;
    message.id = object.id ?? "";
    message.owners = object.owners?.map((e) => e) || [];
    message.humanReadableName = object.humanReadableName ?? "";
    message.priority = object.priority ?? 0;
    message.metrics = object.metrics?.map((e) => BugManagementPolicy_Metric.fromPartial(e)) || [];
    message.explanation = (object.explanation !== undefined && object.explanation !== null)
      ? BugManagementPolicy_Explanation.fromPartial(object.explanation)
      : undefined;
    return message;
  },
};

function createBaseBugManagementPolicy_Metric(): BugManagementPolicy_Metric {
  return { metricId: "", activationThreshold: undefined, deactivationThreshold: undefined };
}

export const BugManagementPolicy_Metric = {
  encode(message: BugManagementPolicy_Metric, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.metricId !== "") {
      writer.uint32(10).string(message.metricId);
    }
    if (message.activationThreshold !== undefined) {
      MetricThreshold.encode(message.activationThreshold, writer.uint32(18).fork()).ldelim();
    }
    if (message.deactivationThreshold !== undefined) {
      MetricThreshold.encode(message.deactivationThreshold, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugManagementPolicy_Metric {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugManagementPolicy_Metric() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.metricId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.activationThreshold = MetricThreshold.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.deactivationThreshold = MetricThreshold.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugManagementPolicy_Metric {
    return {
      metricId: isSet(object.metricId) ? globalThis.String(object.metricId) : "",
      activationThreshold: isSet(object.activationThreshold)
        ? MetricThreshold.fromJSON(object.activationThreshold)
        : undefined,
      deactivationThreshold: isSet(object.deactivationThreshold)
        ? MetricThreshold.fromJSON(object.deactivationThreshold)
        : undefined,
    };
  },

  toJSON(message: BugManagementPolicy_Metric): unknown {
    const obj: any = {};
    if (message.metricId !== "") {
      obj.metricId = message.metricId;
    }
    if (message.activationThreshold !== undefined) {
      obj.activationThreshold = MetricThreshold.toJSON(message.activationThreshold);
    }
    if (message.deactivationThreshold !== undefined) {
      obj.deactivationThreshold = MetricThreshold.toJSON(message.deactivationThreshold);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugManagementPolicy_Metric>, I>>(base?: I): BugManagementPolicy_Metric {
    return BugManagementPolicy_Metric.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugManagementPolicy_Metric>, I>>(object: I): BugManagementPolicy_Metric {
    const message = createBaseBugManagementPolicy_Metric() as any;
    message.metricId = object.metricId ?? "";
    message.activationThreshold = (object.activationThreshold !== undefined && object.activationThreshold !== null)
      ? MetricThreshold.fromPartial(object.activationThreshold)
      : undefined;
    message.deactivationThreshold =
      (object.deactivationThreshold !== undefined && object.deactivationThreshold !== null)
        ? MetricThreshold.fromPartial(object.deactivationThreshold)
        : undefined;
    return message;
  },
};

function createBaseBugManagementPolicy_Explanation(): BugManagementPolicy_Explanation {
  return { problemHtml: "", actionHtml: "" };
}

export const BugManagementPolicy_Explanation = {
  encode(message: BugManagementPolicy_Explanation, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.problemHtml !== "") {
      writer.uint32(10).string(message.problemHtml);
    }
    if (message.actionHtml !== "") {
      writer.uint32(18).string(message.actionHtml);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BugManagementPolicy_Explanation {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBugManagementPolicy_Explanation() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.problemHtml = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.actionHtml = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BugManagementPolicy_Explanation {
    return {
      problemHtml: isSet(object.problemHtml) ? globalThis.String(object.problemHtml) : "",
      actionHtml: isSet(object.actionHtml) ? globalThis.String(object.actionHtml) : "",
    };
  },

  toJSON(message: BugManagementPolicy_Explanation): unknown {
    const obj: any = {};
    if (message.problemHtml !== "") {
      obj.problemHtml = message.problemHtml;
    }
    if (message.actionHtml !== "") {
      obj.actionHtml = message.actionHtml;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BugManagementPolicy_Explanation>, I>>(base?: I): BugManagementPolicy_Explanation {
    return BugManagementPolicy_Explanation.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BugManagementPolicy_Explanation>, I>>(
    object: I,
  ): BugManagementPolicy_Explanation {
    const message = createBaseBugManagementPolicy_Explanation() as any;
    message.problemHtml = object.problemHtml ?? "";
    message.actionHtml = object.actionHtml ?? "";
    return message;
  },
};

function createBaseMonorailProject(): MonorailProject {
  return { project: "", displayPrefix: "" };
}

export const MonorailProject = {
  encode(message: MonorailProject, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.project !== "") {
      writer.uint32(10).string(message.project);
    }
    if (message.displayPrefix !== "") {
      writer.uint32(18).string(message.displayPrefix);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MonorailProject {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMonorailProject() as any;
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

          message.displayPrefix = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): MonorailProject {
    return {
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      displayPrefix: isSet(object.displayPrefix) ? globalThis.String(object.displayPrefix) : "",
    };
  },

  toJSON(message: MonorailProject): unknown {
    const obj: any = {};
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.displayPrefix !== "") {
      obj.displayPrefix = message.displayPrefix;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<MonorailProject>, I>>(base?: I): MonorailProject {
    return MonorailProject.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<MonorailProject>, I>>(object: I): MonorailProject {
    const message = createBaseMonorailProject() as any;
    message.project = object.project ?? "";
    message.displayPrefix = object.displayPrefix ?? "";
    return message;
  },
};

function createBaseMetricThreshold(): MetricThreshold {
  return { oneDay: undefined, threeDay: undefined, sevenDay: undefined };
}

export const MetricThreshold = {
  encode(message: MetricThreshold, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.oneDay !== undefined) {
      writer.uint32(8).int64(message.oneDay);
    }
    if (message.threeDay !== undefined) {
      writer.uint32(16).int64(message.threeDay);
    }
    if (message.sevenDay !== undefined) {
      writer.uint32(24).int64(message.sevenDay);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): MetricThreshold {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseMetricThreshold() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.oneDay = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.threeDay = longToString(reader.int64() as Long);
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.sevenDay = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): MetricThreshold {
    return {
      oneDay: isSet(object.oneDay) ? globalThis.String(object.oneDay) : undefined,
      threeDay: isSet(object.threeDay) ? globalThis.String(object.threeDay) : undefined,
      sevenDay: isSet(object.sevenDay) ? globalThis.String(object.sevenDay) : undefined,
    };
  },

  toJSON(message: MetricThreshold): unknown {
    const obj: any = {};
    if (message.oneDay !== undefined) {
      obj.oneDay = message.oneDay;
    }
    if (message.threeDay !== undefined) {
      obj.threeDay = message.threeDay;
    }
    if (message.sevenDay !== undefined) {
      obj.sevenDay = message.sevenDay;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<MetricThreshold>, I>>(base?: I): MetricThreshold {
    return MetricThreshold.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<MetricThreshold>, I>>(object: I): MetricThreshold {
    const message = createBaseMetricThreshold() as any;
    message.oneDay = object.oneDay ?? undefined;
    message.threeDay = object.threeDay ?? undefined;
    message.sevenDay = object.sevenDay ?? undefined;
    return message;
  },
};

/** Provides methods to access the projects which are using LUCI Analysis. */
export interface Projects {
  /**
   * Gets LUCI Analysis configuration for a LUCI Project.
   *
   * RPC desigend to comply with https://google.aip.dev/131.
   */
  getConfig(request: GetProjectConfigRequest): Promise<ProjectConfig>;
  /**
   * Lists LUCI Projects visible to the user.
   *
   * RPC compliant with https://google.aip.dev/132.
   * This RPC is incomplete. Future breaking changes are
   * expressly flagged.
   */
  list(request: ListProjectsRequest): Promise<ListProjectsResponse>;
}

export const ProjectsServiceName = "luci.analysis.v1.Projects";
export class ProjectsClientImpl implements Projects {
  static readonly DEFAULT_SERVICE = ProjectsServiceName;
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || ProjectsServiceName;
    this.rpc = rpc;
    this.getConfig = this.getConfig.bind(this);
    this.list = this.list.bind(this);
  }
  getConfig(request: GetProjectConfigRequest): Promise<ProjectConfig> {
    const data = GetProjectConfigRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "GetConfig", data);
    return promise.then((data) => ProjectConfig.fromJSON(data));
  }

  list(request: ListProjectsRequest): Promise<ListProjectsResponse> {
    const data = ListProjectsRequest.toJSON(request);
    const promise = this.rpc.request(this.service, "List", data);
    return promise.then((data) => ListProjectsResponse.fromJSON(data));
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
