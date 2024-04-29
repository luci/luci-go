/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Duration } from "../../../../google/protobuf/duration.pb";
import { Struct } from "../../../../google/protobuf/struct.pb";
import { Timestamp } from "../../../../google/protobuf/timestamp.pb";
import { BigQueryExport, HistoryOptions } from "../../resultdb/proto/v1/invocation.pb";
import { BuilderID } from "./builder_common.pb";
import {
  CacheEntry,
  Executable,
  GerritChange,
  GitilesCommit,
  Log,
  RequestedDimension,
  Status,
  StatusDetails,
  statusFromJSON,
  statusToJSON,
  StringPair,
  Trinary,
  trinaryFromJSON,
  trinaryToJSON,
} from "./common.pb";
import { Step } from "./step.pb";
import { Task } from "./task.pb";

export const protobufPackage = "buildbucket.v2";

/**
 * A single build, identified by an int64 ID.
 * Belongs to a builder.
 *
 * RPC: see Builds service for build creation and retrieval.
 * Some Build fields are marked as excluded from responses by default.
 * Use "mask" request field to specify that a field must be included.
 *
 * BigQuery: this message also defines schema of a BigQuery table of completed
 * builds. A BigQuery row is inserted soon after build ends, i.e. a row
 * represents a state of a build at completion time and does not change after
 * that. All fields are included.
 *
 * Next id: 36.
 */
export interface Build {
  /**
   * Identifier of the build, unique per LUCI deployment.
   * IDs are monotonically decreasing.
   */
  readonly id: string;
  /**
   * Required. The builder this build belongs to.
   *
   * Tuple (builder.project, builder.bucket) defines build ACL
   * which may change after build has ended.
   */
  readonly builder: BuilderID | undefined;
  readonly builderInfo:
    | Build_BuilderInfo
    | undefined;
  /**
   * Human-readable identifier of the build with the following properties:
   * - unique within the builder
   * - a monotonically increasing number
   * - mostly contiguous
   * - much shorter than id
   *
   * Caution: populated (positive number) iff build numbers were enabled
   * in the builder configuration at the time of build creation.
   *
   * Caution: Build numbers are not guaranteed to be contiguous.
   * There may be gaps during outages.
   *
   * Caution: Build numbers, while monotonically increasing, do not
   * necessarily reflect source-code order. For example, force builds
   * or rebuilds can allocate new, higher, numbers, but build an older-
   * than-HEAD version of the source.
   */
  readonly number: number;
  /** Verified LUCI identity that created this build. */
  readonly createdBy: string;
  /** Redirect url for the build. */
  readonly viewUrl: string;
  /**
   * Verified LUCI identity that canceled this build.
   *
   * Special values:
   * * buildbucket: The build is canceled by buildbucket. This can happen if the
   * build's parent has ended, and the build cannot outlive its parent.
   * * backend: The build's backend task is canceled. For example the build's
   * Swarming task is killed.
   */
  readonly canceledBy: string;
  /** When the build was created. */
  readonly createTime:
    | string
    | undefined;
  /**
   * When the build started.
   * Required iff status is STARTED, SUCCESS or FAILURE.
   */
  readonly startTime:
    | string
    | undefined;
  /**
   * When the build ended.
   * Present iff status is terminal.
   * MUST NOT be before start_time.
   */
  readonly endTime:
    | string
    | undefined;
  /**
   * When the build was most recently updated.
   *
   * RPC: can be > end_time if, e.g. new tags were attached to a completed
   * build.
   */
  readonly updateTime:
    | string
    | undefined;
  /**
   * When the cancel process of the build started.
   * Note it's not the time that the cancellation completed, which would be
   * tracked by end_time.
   *
   * During the cancel process, the build still accepts updates.
   *
   * bbagent checks this field at the frequency of
   * buildbucket.MinUpdateBuildInterval. When bbagent sees the build is in
   * cancel process, there are two states:
   *  * it has NOT yet started the exe payload,
   *  * it HAS started the exe payload.
   *
   * In the first state, bbagent will immediately terminate the build without
   * invoking the exe payload at all.
   *
   * In the second state, bbagent will send SIGTERM/CTRL-BREAK to the exe
   * (according to the deadline protocol described in
   * https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md).
   * After grace_period it will then try to kill the exe.
   *
   * NOTE: There is a race condition here; If bbagent starts the luciexe and
   * then immediately notices that the build is canceled, it's possible that
   * bbagent can send SIGTERM/CTRL-BREAK to the exe before that exe sets up
   * interrupt handlers. There is a bug on file (crbug.com/1311821)
   * which we plan to implement at some point as a mitigation for this.
   *
   * Additionally, the Buildbucket service itself will launch an asynchronous
   * task to terminate the build via the backend API (e.g. Swarming cancellation)
   * if bbagent cannot successfully terminate the exe in time.
   */
  readonly cancelTime:
    | string
    | undefined;
  /**
   * Status of the build.
   * Must be specified, i.e. not STATUS_UNSPECIFIED.
   *
   * RPC: Responses have most current status.
   *
   * BigQuery: Final status of the build. Cannot be SCHEDULED or STARTED.
   */
  readonly status: Status;
  /**
   * Human-readable summary of the build in Markdown format
   * (https://spec.commonmark.org/0.28/).
   * Explains status.
   * Up to 4 KB.
   */
  readonly summaryMarkdown: string;
  /**
   * Markdown reasoning for cancelling the build.
   * Human readable and should be following https://spec.commonmark.org/0.28/.
   */
  readonly cancellationMarkdown: string;
  /**
   * If NO, then the build status SHOULD NOT be used to assess correctness of
   * the input gitiles_commit or gerrit_changes.
   * For example, if a pre-submit build has failed, CQ MAY still land the CL.
   * For example, if a post-submit build has failed, CLs MAY continue landing.
   */
  readonly critical: Trinary;
  /**
   * Machine-readable details of the current status.
   * Human-readable status reason is available in summary_markdown.
   */
  readonly statusDetails:
    | StatusDetails
    | undefined;
  /** Input to the build executable. */
  readonly input:
    | Build_Input
    | undefined;
  /**
   * Output of the build executable.
   * SHOULD depend only on input field and NOT other fields.
   * MUST be unset if build status is SCHEDULED.
   *
   * RPC: By default, this field is excluded from responses.
   * Updated while the build is running and finalized when the build ends.
   */
  readonly output:
    | Build_Output
    | undefined;
  /**
   * Current list of build steps.
   * Updated as build runs.
   *
   * May take up to 1MB after zlib compression.
   * MUST be unset if build status is SCHEDULED.
   *
   * RPC: By default, this field is excluded from responses.
   */
  readonly steps: readonly Step[];
  /**
   * Build infrastructure used by the build.
   *
   * RPC: By default, this field is excluded from responses.
   */
  readonly infra:
    | BuildInfra
    | undefined;
  /**
   * Arbitrary annotations for the build.
   * One key may have multiple values, which is why this is not a map<string,string>.
   * Indexed by the server, see also BuildPredicate.tags.
   */
  readonly tags: readonly StringPair[];
  /** What to run when the build is ready to start. */
  readonly exe:
    | Executable
    | undefined;
  /**
   * DEPRECATED
   *
   * Equivalent to `"luci.buildbucket.canary_software" in input.experiments`.
   *
   * See `Builder.experiments` for well-known experiments.
   */
  readonly canary: boolean;
  /**
   * Maximum build pending time.
   * If the timeout is reached, the build is marked as INFRA_FAILURE status
   * and both status_details.{timeout, resource_exhaustion} are set.
   */
  readonly schedulingTimeout:
    | Duration
    | undefined;
  /**
   * Maximum build execution time.
   *
   * Not to be confused with scheduling_timeout.
   *
   * If the timeout is reached, the task will be signaled according to the
   * `deadline` section of
   * https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md
   * and status_details.timeout is set.
   *
   * The task will have `grace_period` amount of time to handle cleanup
   * before being forcefully terminated.
   */
  readonly executionTimeout:
    | Duration
    | undefined;
  /**
   * Amount of cleanup time after execution_timeout.
   *
   * After being signaled according to execution_timeout, the task will
   * have this duration to clean up before being forcefully terminated.
   *
   * The signalling process is explained in the `deadline` section of
   * https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md.
   */
  readonly gracePeriod:
    | Duration
    | undefined;
  /**
   * If set, swarming was requested to wait until it sees at least one bot
   * report a superset of the build's requested dimensions.
   */
  readonly waitForCapacity: boolean;
  /**
   * Flag to control if the build can outlive its parent.
   *
   * This field is only meaningful if the build has ancestors.
   * If the build has ancestors and the value is false, it means that the build
   * SHOULD reach a terminal status (SUCCESS, FAILURE, INFRA_FAILURE or
   * CANCELED) before its parent. If the child fails to do so, Buildbucket will
   * cancel it some time after the parent build reaches a terminal status.
   *
   * A build that can outlive its parent can also outlive its parent's ancestors.
   */
  readonly canOutliveParent: boolean;
  /**
   * IDs of the build's ancestors. This includes all parents/grandparents/etc.
   * This is ordered from top-to-bottom so `ancestor_ids[0]` is the root of
   * the builds tree, and `ancestor_ids[-1]` is this build's immediate parent.
   * This does not include any "siblings" at higher levels of the tree, just
   * the direct chain of ancestors from root to this build.
   */
  readonly ancestorIds: readonly string[];
  /**
   * If UNSET, retrying the build is implicitly allowed;
   * If YES, retrying the build is explicitly allowed;
   * If NO, retrying the build is explicitly disallowed,
   *   * any UI displaying the build should remove "retry" button(s),
   *   * ScheduleBuild using the build as template should fail,
   *   * but the build can still be synthesized by SynthesizeBuild.
   */
  readonly retriable: Trinary;
}

/**
 * Defines what to build/test.
 *
 * Behavior of a build executable MAY depend on Input.
 * It MAY NOT modify its behavior based on anything outside of Input.
 * It MAY read non-Input fields to display for debugging or to pass-through to
 * triggered builds. For example the "tags" field may be passed to triggered
 * builds, or the "infra" field may be printed for debugging purposes.
 */
export interface Build_Input {
  /**
   * Arbitrary JSON object. Available at build run time.
   *
   * RPC: By default, this field is excluded from responses.
   *
   * V1 equivalent: corresponds to "properties" key in "parameters_json".
   */
  readonly properties:
    | { readonly [key: string]: any }
    | undefined;
  /**
   * The Gitiles commit to run against.
   * Usually present in CI builds, set by LUCI Scheduler.
   * If not present, the build may checkout "refs/heads/master".
   * NOT a blamelist.
   *
   * V1 equivalent: supersedes "revision" property and "buildset"
   * tag that starts with "commit/gitiles/".
   */
  readonly gitilesCommit:
    | GitilesCommit
    | undefined;
  /**
   * Gerrit patchsets to run against.
   * Usually present in tryjobs, set by CQ, Gerrit, git-cl-try.
   * Applied on top of gitiles_commit if specified, otherwise tip of the tree.
   *
   * V1 equivalent: supersedes patch_* properties and "buildset"
   * tag that starts with "patch/gerrit/".
   */
  readonly gerritChanges: readonly GerritChange[];
  /**
   * DEPRECATED
   *
   * Equivalent to `"luci.non_production" in experiments`.
   *
   * See `Builder.experiments` for well-known experiments.
   */
  readonly experimental: boolean;
  /**
   * The sorted list of experiments enabled on this build.
   *
   * See `Builder.experiments` for a detailed breakdown on how experiments
   * work, and go/buildbucket-settings.cfg for the current state of global
   * experiments.
   */
  readonly experiments: readonly string[];
}

/** Result of the build executable. */
export interface Build_Output {
  /**
   * Arbitrary JSON object produced by the build.
   *
   * In recipes, use step_result.presentation.properties to set these,
   * for example
   *
   *   step_result = api.step(['echo'])
   *   step_result.presentation.properties['foo'] = 'bar'
   *
   * More docs: https://chromium.googlesource.com/infra/luci/recipes-py/+/HEAD/doc/old_user_guide.md#Setting-properties
   *
   * V1 equivalent: corresponds to "properties" key in
   * "result_details_json".
   * In V1 output properties are not populated until build ends.
   */
  readonly properties:
    | { readonly [key: string]: any }
    | undefined;
  /**
   * Build checked out and executed on this commit.
   *
   * Should correspond to Build.Input.gitiles_commit.
   * May be present even if Build.Input.gitiles_commit is not set, for example
   * in cron builders.
   *
   * V1 equivalent: this supersedes all got_revision output property.
   */
  readonly gitilesCommit:
    | GitilesCommit
    | undefined;
  /** Logs produced by the build script, typically "stdout" and "stderr". */
  readonly logs: readonly Log[];
  /** Build status which is reported by the client via StartBuild or UpdateBuild. */
  readonly status: Status;
  readonly statusDetails:
    | StatusDetails
    | undefined;
  /**
   * Deprecated. Use summary_markdown instead.
   *
   * @deprecated
   */
  readonly summaryHtml: string;
  readonly summaryMarkdown: string;
}

/**
 * Information of the builder, propagated from builder config.
 *
 * The info captures the state of the builder at creation time.
 * If any information is updated, all future builds will have the new
 * information, while the historical builds persist the old information.
 */
export interface Build_BuilderInfo {
  readonly description: string;
}

export interface InputDataRef {
  readonly cas?: InputDataRef_CAS | undefined;
  readonly cipd?:
    | InputDataRef_CIPD
    | undefined;
  /**
   * TODO(crbug.com/1266060): TBD. `on_path` may need to move out to be incorporated into a field which captures other envvars.
   * Subdirectories relative to the root of `ref` which should be set as a prefix to
   * the $PATH variable.
   *
   * A substitute of `env_prefixes` in SwarmingRpcsTaskProperties field -
   * https://chromium.googlesource.com/infra/luci/luci-go/+/0048a84944e872776fba3542aa96d5943ae64bab/common/api/swarming/swarming/v1/swarming-gen.go#1495
   */
  readonly onPath: readonly string[];
}

export interface InputDataRef_CAS {
  /**
   * Full name of RBE-CAS instance. `projects/{project_id}/instances/{instance}`.
   * e.g. projects/chromium-swarm/instances/default_instance
   */
  readonly casInstance: string;
  readonly digest: InputDataRef_CAS_Digest | undefined;
}

/**
 * This is a [Digest][build.bazel.remote.execution.v2.Digest] of a blob on
 * RBE-CAS. See the explanations at the original definition.
 * https://github.com/bazelbuild/remote-apis/blob/77cfb44a88577a7ade5dd2400425f6d50469ec6d/build/bazel/remote/execution/v2/remote_execution.proto#L753-L791
 */
export interface InputDataRef_CAS_Digest {
  readonly hash: string;
  readonly sizeBytes: string;
}

export interface InputDataRef_CIPD {
  readonly server: string;
  readonly specs: readonly InputDataRef_CIPD_PkgSpec[];
}

export interface InputDataRef_CIPD_PkgSpec {
  /**
   * Package MAY include CIPD variables, including conditional variables like
   * `${os=windows}`. Additionally, version may be a ref or a tag.
   */
  readonly package: string;
  readonly version: string;
}

export interface ResolvedDataRef {
  readonly cas?: ResolvedDataRef_CAS | undefined;
  readonly cipd?: ResolvedDataRef_CIPD | undefined;
}

export interface ResolvedDataRef_Timing {
  readonly fetchDuration: Duration | undefined;
  readonly installDuration: Duration | undefined;
}

export interface ResolvedDataRef_CAS {
  /**
   * TODO(crbug.com/1266060): potential fields can be
   * int64 cache_hits = ?;
   * int64 cache_hit_size = ?:
   * int64 cache_misses = ?;
   * int64 cache_miss_size = ?;
   * need more thinking and better to determine when starting writing code
   * to download binaries in bbagent.
   */
  readonly timing: ResolvedDataRef_Timing | undefined;
}

export interface ResolvedDataRef_CIPD {
  readonly specs: readonly ResolvedDataRef_CIPD_PkgSpec[];
}

export interface ResolvedDataRef_CIPD_PkgSpec {
  /**
   * True if this package wasn't installed because `package` contained a
   * non-applicable conditional (e.g. ${os=windows} on a mac machine).
   */
  readonly skipped: boolean;
  /** fully resolved */
  readonly package: string;
  /** fully resolved */
  readonly version: string;
  readonly wasCached: Trinary;
  /** optional */
  readonly timing: ResolvedDataRef_Timing | undefined;
}

/** Build infrastructure that was used for a particular build. */
export interface BuildInfra {
  readonly buildbucket: BuildInfra_Buildbucket | undefined;
  readonly swarming: BuildInfra_Swarming | undefined;
  readonly logdog: BuildInfra_LogDog | undefined;
  readonly recipe: BuildInfra_Recipe | undefined;
  readonly resultdb: BuildInfra_ResultDB | undefined;
  readonly bbagent: BuildInfra_BBAgent | undefined;
  readonly backend:
    | BuildInfra_Backend
    | undefined;
  /** It should only be set for led builds. */
  readonly led: BuildInfra_Led | undefined;
}

/** Buildbucket-specific information, captured at the build creation time. */
export interface BuildInfra_Buildbucket {
  /**
   * Version of swarming task template. Defines
   * versions of kitchen, git, git wrapper, python, vpython, etc.
   */
  readonly serviceConfigRevision: string;
  /**
   * Properties that were specified in ScheduleBuildRequest to create this
   * build.
   *
   * In particular, CQ uses this to decide whether the build created by
   * someone else is appropriate for CQ, e.g. it was created with the same
   * properties that CQ would use.
   */
  readonly requestedProperties:
    | { readonly [key: string]: any }
    | undefined;
  /**
   * Dimensions that were specified in ScheduleBuildRequest to create this
   * build.
   */
  readonly requestedDimensions: readonly RequestedDimension[];
  /** Buildbucket hostname, e.g. "cr-buildbucket.appspot.com". */
  readonly hostname: string;
  /**
   * This contains a map of all the experiments involved for this build, as
   * well as which bit of configuration lead to them being set (or unset).
   *
   * Note that if the reason here is EXPERIMENT_REASON_GLOBAL_INACTIVE,
   * then that means that the experiment is completely disabled and has no
   * effect, but your builder or ScheduleBuildRequest still indicated that
   * the experiment should be set. If you see this, then please remove it
   * from your configuration and/or requests.
   */
  readonly experimentReasons: { [key: string]: BuildInfra_Buildbucket_ExperimentReason };
  /**
   * The agent binary (bbagent or kitchen) resolutions Buildbucket made for this build.
   * This includes all agent_executable references supplied to
   * the TaskBackend in "original" (CIPD) form, to facilitate debugging.
   * DEPRECATED: Use agent.source instead.
   *
   * @deprecated
   */
  readonly agentExecutable: { [key: string]: ResolvedDataRef };
  readonly agent: BuildInfra_Buildbucket_Agent | undefined;
  readonly knownPublicGerritHosts: readonly string[];
  /** Flag for if the build should have a build number. */
  readonly buildNumber: boolean;
}

export enum BuildInfra_Buildbucket_ExperimentReason {
  /** UNSET - This value is unused (i.e. if you see this, it's a bug). */
  UNSET = 0,
  /**
   * GLOBAL_DEFAULT - This experiment was configured from the 'default_value' of a global
   * experiment.
   *
   * See go/buildbucket-settings.cfg for the list of global experiments.
   */
  GLOBAL_DEFAULT = 1,
  /** BUILDER_CONFIG - This experiment was configured from the Builder configuration. */
  BUILDER_CONFIG = 2,
  /**
   * GLOBAL_MINIMUM - This experiment was configured from the 'minimum_value' of a global
   * experiment.
   *
   * See go/buildbucket-settings.cfg for the list of global experiments.
   */
  GLOBAL_MINIMUM = 3,
  /** REQUESTED - This experiment was explicitly set from the ScheduleBuildRequest. */
  REQUESTED = 4,
  /**
   * GLOBAL_INACTIVE - This experiment is inactive and so was removed from the Build.
   *
   * See go/buildbucket-settings.cfg for the list of global experiments.
   */
  GLOBAL_INACTIVE = 5,
}

export function buildInfra_Buildbucket_ExperimentReasonFromJSON(object: any): BuildInfra_Buildbucket_ExperimentReason {
  switch (object) {
    case 0:
    case "EXPERIMENT_REASON_UNSET":
      return BuildInfra_Buildbucket_ExperimentReason.UNSET;
    case 1:
    case "EXPERIMENT_REASON_GLOBAL_DEFAULT":
      return BuildInfra_Buildbucket_ExperimentReason.GLOBAL_DEFAULT;
    case 2:
    case "EXPERIMENT_REASON_BUILDER_CONFIG":
      return BuildInfra_Buildbucket_ExperimentReason.BUILDER_CONFIG;
    case 3:
    case "EXPERIMENT_REASON_GLOBAL_MINIMUM":
      return BuildInfra_Buildbucket_ExperimentReason.GLOBAL_MINIMUM;
    case 4:
    case "EXPERIMENT_REASON_REQUESTED":
      return BuildInfra_Buildbucket_ExperimentReason.REQUESTED;
    case 5:
    case "EXPERIMENT_REASON_GLOBAL_INACTIVE":
      return BuildInfra_Buildbucket_ExperimentReason.GLOBAL_INACTIVE;
    default:
      throw new globalThis.Error(
        "Unrecognized enum value " + object + " for enum BuildInfra_Buildbucket_ExperimentReason",
      );
  }
}

export function buildInfra_Buildbucket_ExperimentReasonToJSON(object: BuildInfra_Buildbucket_ExperimentReason): string {
  switch (object) {
    case BuildInfra_Buildbucket_ExperimentReason.UNSET:
      return "EXPERIMENT_REASON_UNSET";
    case BuildInfra_Buildbucket_ExperimentReason.GLOBAL_DEFAULT:
      return "EXPERIMENT_REASON_GLOBAL_DEFAULT";
    case BuildInfra_Buildbucket_ExperimentReason.BUILDER_CONFIG:
      return "EXPERIMENT_REASON_BUILDER_CONFIG";
    case BuildInfra_Buildbucket_ExperimentReason.GLOBAL_MINIMUM:
      return "EXPERIMENT_REASON_GLOBAL_MINIMUM";
    case BuildInfra_Buildbucket_ExperimentReason.REQUESTED:
      return "EXPERIMENT_REASON_REQUESTED";
    case BuildInfra_Buildbucket_ExperimentReason.GLOBAL_INACTIVE:
      return "EXPERIMENT_REASON_GLOBAL_INACTIVE";
    default:
      throw new globalThis.Error(
        "Unrecognized enum value " + object + " for enum BuildInfra_Buildbucket_ExperimentReason",
      );
  }
}

/** bbagent will interpret Agent.input, as well as update Agent.output. */
export interface BuildInfra_Buildbucket_Agent {
  /**
   * TODO(crbug.com/1297809): for a long-term solution, we may need to add
   * a top-level `on_path` array field in the input and read the value from
   * configuration files (eg.settings.cfg, builder configs). So it can store
   * the intended order of PATH env var. Then the per-inputDataRef level
   * `on_path` field will be deprecated.
   * Currently, the new BBagent flow merges all inputDataRef-level `on_path`
   * values and sort. This mimics the same behavior of PyBB backend in order
   * to have the cipd_installation migration to roll out first under a minimal risk.
   */
  readonly input: BuildInfra_Buildbucket_Agent_Input | undefined;
  readonly output: BuildInfra_Buildbucket_Agent_Output | undefined;
  readonly source:
    | BuildInfra_Buildbucket_Agent_Source
    | undefined;
  /**
   * Maps the relative-to-root directory path in both `input` and `output`
   * to the Purpose of the software in that directory.
   *
   * If a path is not listed here, it is the same as PURPOSE_UNSPECIFIED.
   */
  readonly purposes: { [key: string]: BuildInfra_Buildbucket_Agent_Purpose };
  /**
   * Cache for the cipd client.
   * The cache name should be in the format like `cipd_client_<sha(client_version)>`.
   */
  readonly cipdClientCache:
    | CacheEntry
    | undefined;
  /**
   * Cache for the cipd packages.
   * The cache name should be in the format like `cipd_cache_<sha(task_service_account)>`.
   */
  readonly cipdPackagesCache: CacheEntry | undefined;
}

export enum BuildInfra_Buildbucket_Agent_Purpose {
  /** UNSPECIFIED - No categorized/known purpose. */
  UNSPECIFIED = 0,
  /** EXE_PAYLOAD - This path contains the contents of the build's `exe.cipd_package`. */
  EXE_PAYLOAD = 1,
  /**
   * BBAGENT_UTILITY - This path contains data specifically for bbagent's own use.
   *
   * There's a proposal currently to add `nsjail` support to bbagent, and it
   * would need to bring a copy of `nsjail` in order to run the user binary
   * but we wouldn't necessarily want to expose it to the user binary.
   */
  BBAGENT_UTILITY = 2,
}

export function buildInfra_Buildbucket_Agent_PurposeFromJSON(object: any): BuildInfra_Buildbucket_Agent_Purpose {
  switch (object) {
    case 0:
    case "PURPOSE_UNSPECIFIED":
      return BuildInfra_Buildbucket_Agent_Purpose.UNSPECIFIED;
    case 1:
    case "PURPOSE_EXE_PAYLOAD":
      return BuildInfra_Buildbucket_Agent_Purpose.EXE_PAYLOAD;
    case 2:
    case "PURPOSE_BBAGENT_UTILITY":
      return BuildInfra_Buildbucket_Agent_Purpose.BBAGENT_UTILITY;
    default:
      throw new globalThis.Error(
        "Unrecognized enum value " + object + " for enum BuildInfra_Buildbucket_Agent_Purpose",
      );
  }
}

export function buildInfra_Buildbucket_Agent_PurposeToJSON(object: BuildInfra_Buildbucket_Agent_Purpose): string {
  switch (object) {
    case BuildInfra_Buildbucket_Agent_Purpose.UNSPECIFIED:
      return "PURPOSE_UNSPECIFIED";
    case BuildInfra_Buildbucket_Agent_Purpose.EXE_PAYLOAD:
      return "PURPOSE_EXE_PAYLOAD";
    case BuildInfra_Buildbucket_Agent_Purpose.BBAGENT_UTILITY:
      return "PURPOSE_BBAGENT_UTILITY";
    default:
      throw new globalThis.Error(
        "Unrecognized enum value " + object + " for enum BuildInfra_Buildbucket_Agent_Purpose",
      );
  }
}

/** Source describes where the Agent should be fetched from. */
export interface BuildInfra_Buildbucket_Agent_Source {
  readonly cipd?: BuildInfra_Buildbucket_Agent_Source_CIPD | undefined;
}

export interface BuildInfra_Buildbucket_Agent_Source_CIPD {
  /**
   * The CIPD package to use for the agent.
   *
   * Must end in "/${platform}" with no other CIPD variables.
   *
   * If using an experimental agent binary, please make sure the package
   * prefix has been configured here -
   * https://chrome-internal.googlesource.com/infradata/config/+/refs/heads/main/configs/chrome-infra-packages/bootstrap.cfg
   */
  readonly package: string;
  /** The CIPD version to use for the agent. */
  readonly version: string;
  /** The CIPD server to use. */
  readonly server: string;
  /**
   * maps ${platform} -> instance_id for resolved agent packages.
   *
   * Will be overwritten at CreateBuild time, should be left empty
   * when creating a new Build.
   */
  readonly resolvedInstances: { [key: string]: string };
}

export interface BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry {
  readonly key: string;
  readonly value: string;
}

export interface BuildInfra_Buildbucket_Agent_Input {
  /**
   * Maps relative-to-root directory to the data.
   *
   * For now, data is only allowed at the 'leaves', e.g. you cannot
   * specify data at "a/b/c" and "a/b" (but "a/b/c" and "a/q" would be OK).
   * All directories beginning with "luci." are reserved for Buildbucket's own use.
   *
   * TODO(crbug.com/1266060): Enforce the above constraints in a later phase.
   * Currently users don't have the flexibility to set the parent directory path.
   */
  readonly data: { [key: string]: InputDataRef };
  /**
   * Maps relative-to-root directory to the cipd package itself.
   * This is the CIPD client itself and  should be downloaded first so that
   * the packages in the data field above can be downloaded.
   */
  readonly cipdSource: { [key: string]: InputDataRef };
}

export interface BuildInfra_Buildbucket_Agent_Input_DataEntry {
  readonly key: string;
  readonly value: InputDataRef | undefined;
}

export interface BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry {
  readonly key: string;
  readonly value: InputDataRef | undefined;
}

export interface BuildInfra_Buildbucket_Agent_Output {
  /**
   * Maps relative-to-root directory to the fully-resolved ref.
   *
   * This will always have 1:1 mapping to Agent.Input.data
   */
  readonly resolvedData: { [key: string]: ResolvedDataRef };
  readonly status: Status;
  readonly statusDetails:
    | StatusDetails
    | undefined;
  /**
   * Deprecated. Use summary_markdown instead.
   *
   * @deprecated
   */
  readonly summaryHtml: string;
  /**
   * The agent's resolved CIPD ${platform} (e.g. "linux-amd64",
   * "windows-386", etc.).
   *
   * This is trivial for bbagent to calculate (unlike trying to embed
   * its cipd package version inside or along with the executable).
   * Buildbucket is doing a full package -> instance ID resolution at
   * CreateBuild time anyway, so Agent.Source.resolved_instances
   * will give the mapping from `agent_platform` to a precise instance_id
   * which was used.
   */
  readonly agentPlatform: string;
  /**
   * Total installation duration for all input data. Currently only record
   * cipd packages installation time.
   */
  readonly totalDuration: Duration | undefined;
  readonly summaryMarkdown: string;
}

export interface BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry {
  readonly key: string;
  readonly value: ResolvedDataRef | undefined;
}

export interface BuildInfra_Buildbucket_Agent_PurposesEntry {
  readonly key: string;
  readonly value: BuildInfra_Buildbucket_Agent_Purpose;
}

export interface BuildInfra_Buildbucket_ExperimentReasonsEntry {
  readonly key: string;
  readonly value: BuildInfra_Buildbucket_ExperimentReason;
}

export interface BuildInfra_Buildbucket_AgentExecutableEntry {
  readonly key: string;
  readonly value: ResolvedDataRef | undefined;
}

/**
 * Swarming-specific information.
 *
 * Next ID: 10.
 */
export interface BuildInfra_Swarming {
  /**
   * Swarming hostname, e.g. "chromium-swarm.appspot.com".
   * Populated at the build creation time.
   */
  readonly hostname: string;
  /**
   * Swarming task id.
   * Not guaranteed to be populated at the build creation time.
   */
  readonly taskId: string;
  /**
   * Swarming run id of the parent task from which this build is triggered.
   * If set, swarming promises to ensure this build won't outlive its parent
   * swarming task (which may or may not itself be a Buildbucket build).
   * Populated at the build creation time.
   */
  readonly parentRunId: string;
  /**
   * Task service account email address.
   * This is the service account used for all authenticated requests by the
   * build.
   */
  readonly taskServiceAccount: string;
  /**
   * Priority of the task. The lower the more important.
   * Valid values are [20..255].
   */
  readonly priority: number;
  /** Swarming dimensions for the task. */
  readonly taskDimensions: readonly RequestedDimension[];
  /** Swarming dimensions of the bot used for the task. */
  readonly botDimensions: readonly StringPair[];
  /** Caches requested by this build. */
  readonly caches: readonly BuildInfra_Swarming_CacheEntry[];
}

/**
 * Describes a cache directory persisted on a bot.
 *
 * If a build requested a cache, the cache directory is available on build
 * startup. If the cache was present on the bot, the directory contains
 * files from the previous run on that bot.
 * The build can read/write to the cache directory while it runs.
 * After build completes, the cache directory is persisted.
 * The next time another build requests the same cache and runs on the same
 * bot, the files will still be there (unless the cache was evicted,
 * perhaps due to disk space reasons).
 *
 * One bot can keep multiple caches at the same time and one build can request
 * multiple different caches.
 * A cache is identified by its name and mapped to a path.
 *
 * If the bot is running out of space, caches are evicted in LRU manner
 * before the next build on this bot starts.
 *
 * Builder cache.
 *
 * Buildbucket implicitly declares cache
 *   {"name": "<hash(project/bucket/builder)>", "path": "builder"}.
 * This means that any LUCI builder has a "personal disk space" on the bot.
 * Builder cache is often a good start before customizing caching.
 * In recipes, it is available at api.buildbucket.builder_cache_path.
 */
export interface BuildInfra_Swarming_CacheEntry {
  /**
   * Identifier of the cache. Required. Length is limited to 128.
   * Must be unique in the build.
   *
   * If the pool of swarming bots is shared among multiple LUCI projects and
   * projects use same cache name, the cache will be shared across projects.
   * To avoid affecting and being affected by other projects, prefix the
   * cache name with something project-specific, e.g. "v8-".
   */
  readonly name: string;
  /**
   * Relative path where the cache in mapped into. Required.
   *
   * Must use POSIX format (forward slashes).
   * In most cases, it does not need slashes at all.
   *
   * In recipes, use api.path.cache_dir.join(path) to get absolute path.
   *
   * Must be unique in the build.
   */
  readonly path: string;
  /**
   * Duration to wait for a bot with a warm cache to pick up the
   * task, before falling back to a bot with a cold (non-existent) cache.
   *
   * The default is 0, which means that no preference will be chosen for a
   * bot with this or without this cache, and a bot without this cache may
   * be chosen instead.
   *
   * If no bot has this cache warm, the task will skip this wait and will
   * immediately fallback to a cold cache request.
   *
   * The value must be multiples of 60 seconds.
   */
  readonly waitForWarmCache:
    | Duration
    | undefined;
  /**
   * Environment variable with this name will be set to the path to the cache
   * directory.
   */
  readonly envVar: string;
}

/** LogDog-specific information. */
export interface BuildInfra_LogDog {
  /** LogDog hostname, e.g. "logs.chromium.org". */
  readonly hostname: string;
  /**
   * LogDog project, e.g. "chromium".
   * Typically matches Build.builder.project.
   */
  readonly project: string;
  /**
   * A slash-separated path prefix shared by all logs and artifacts of this
   * build.
   * No other build can have the same prefix.
   * Can be used to discover logs and/or load log contents.
   */
  readonly prefix: string;
}

/** Recipe-specific information. */
export interface BuildInfra_Recipe {
  /** CIPD package name containing the recipe used to run this build. */
  readonly cipdPackage: string;
  /** Name of the recipe used to run this build. */
  readonly name: string;
}

/** ResultDB-specific information. */
export interface BuildInfra_ResultDB {
  /** Hostname of the ResultDB instance, such as "results.api.cr.dev". */
  readonly hostname: string;
  /**
   * Name of the invocation for results of this build.
   * Typically "invocations/build:<build_id>".
   */
  readonly invocation: string;
  /** Whether to enable ResultDB:Buildbucket integration. */
  readonly enable: boolean;
  /**
   * Configuration for exporting test results to BigQuery.
   * This can have multiple values to export results to multiple BigQuery
   * tables, or to support multiple test result predicates.
   */
  readonly bqExports: readonly BigQueryExport[];
  /** Deprecated. Any values specified here are ignored. */
  readonly historyOptions: HistoryOptions | undefined;
}

/** Led specific information. */
export interface BuildInfra_Led {
  /** The original bucket this led build is shadowing. */
  readonly shadowedBucket: string;
}

/**
 * BBAgent-specific information.
 *
 * All paths are relateive to bbagent's working directory, and must be delimited
 * with slashes ("/"), regardless of the host OS.
 */
export interface BuildInfra_BBAgent {
  /**
   * Path to the base of the user executable package.
   *
   * Required.
   */
  readonly payloadPath: string;
  /**
   * Path to a directory where each subdirectory is a cache dir.
   *
   * Required.
   */
  readonly cacheDir: string;
  /**
   * List of Gerrit hosts to force git authentication for.
   *
   * By default public hosts are accessed anonymously, and the anonymous access
   * has very low quota. Context needs to know all such hostnames in advance to
   * be able to force authenticated access to them.
   *
   * @deprecated
   */
  readonly knownPublicGerritHosts: readonly string[];
  /**
   * DEPRECATED: Use build.Infra.Buildbucket.Agent.Input instead.
   *
   * @deprecated
   */
  readonly input: BuildInfra_BBAgent_Input | undefined;
}

/** BBAgent-specific input. */
export interface BuildInfra_BBAgent_Input {
  readonly cipdPackages: readonly BuildInfra_BBAgent_Input_CIPDPackage[];
}

/** CIPD Packages to make available for this build. */
export interface BuildInfra_BBAgent_Input_CIPDPackage {
  /**
   * Name of this CIPD package.
   *
   * Required.
   */
  readonly name: string;
  /**
   * CIPD package version.
   *
   * Required.
   */
  readonly version: string;
  /**
   * CIPD server to fetch this package from.
   *
   * Required.
   */
  readonly server: string;
  /**
   * Path where this CIPD package should be installed.
   *
   * Required.
   */
  readonly path: string;
}

/** Backend-specific information. */
export interface BuildInfra_Backend {
  /**
   * Configuration supplied to the backend at the time it was instructed to
   * run this build.
   */
  readonly config:
    | { readonly [key: string]: any }
    | undefined;
  /**
   * Current backend task status.
   * Updated as build runs.
   */
  readonly task:
    | Task
    | undefined;
  /** Caches requested by this build. */
  readonly caches: readonly CacheEntry[];
  /** Dimensions for the task. */
  readonly taskDimensions: readonly RequestedDimension[];
  /** Hostname is the hostname for the backend itself. */
  readonly hostname: string;
}

function createBaseBuild(): Build {
  return {
    id: "0",
    builder: undefined,
    builderInfo: undefined,
    number: 0,
    createdBy: "",
    viewUrl: "",
    canceledBy: "",
    createTime: undefined,
    startTime: undefined,
    endTime: undefined,
    updateTime: undefined,
    cancelTime: undefined,
    status: 0,
    summaryMarkdown: "",
    cancellationMarkdown: "",
    critical: 0,
    statusDetails: undefined,
    input: undefined,
    output: undefined,
    steps: [],
    infra: undefined,
    tags: [],
    exe: undefined,
    canary: false,
    schedulingTimeout: undefined,
    executionTimeout: undefined,
    gracePeriod: undefined,
    waitForCapacity: false,
    canOutliveParent: false,
    ancestorIds: [],
    retriable: 0,
  };
}

export const Build = {
  encode(message: Build, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "0") {
      writer.uint32(8).int64(message.id);
    }
    if (message.builder !== undefined) {
      BuilderID.encode(message.builder, writer.uint32(18).fork()).ldelim();
    }
    if (message.builderInfo !== undefined) {
      Build_BuilderInfo.encode(message.builderInfo, writer.uint32(274).fork()).ldelim();
    }
    if (message.number !== 0) {
      writer.uint32(24).int32(message.number);
    }
    if (message.createdBy !== "") {
      writer.uint32(34).string(message.createdBy);
    }
    if (message.viewUrl !== "") {
      writer.uint32(42).string(message.viewUrl);
    }
    if (message.canceledBy !== "") {
      writer.uint32(186).string(message.canceledBy);
    }
    if (message.createTime !== undefined) {
      Timestamp.encode(toTimestamp(message.createTime), writer.uint32(50).fork()).ldelim();
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(58).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(66).fork()).ldelim();
    }
    if (message.updateTime !== undefined) {
      Timestamp.encode(toTimestamp(message.updateTime), writer.uint32(74).fork()).ldelim();
    }
    if (message.cancelTime !== undefined) {
      Timestamp.encode(toTimestamp(message.cancelTime), writer.uint32(258).fork()).ldelim();
    }
    if (message.status !== 0) {
      writer.uint32(96).int32(message.status);
    }
    if (message.summaryMarkdown !== "") {
      writer.uint32(162).string(message.summaryMarkdown);
    }
    if (message.cancellationMarkdown !== "") {
      writer.uint32(266).string(message.cancellationMarkdown);
    }
    if (message.critical !== 0) {
      writer.uint32(168).int32(message.critical);
    }
    if (message.statusDetails !== undefined) {
      StatusDetails.encode(message.statusDetails, writer.uint32(178).fork()).ldelim();
    }
    if (message.input !== undefined) {
      Build_Input.encode(message.input, writer.uint32(122).fork()).ldelim();
    }
    if (message.output !== undefined) {
      Build_Output.encode(message.output, writer.uint32(130).fork()).ldelim();
    }
    for (const v of message.steps) {
      Step.encode(v!, writer.uint32(138).fork()).ldelim();
    }
    if (message.infra !== undefined) {
      BuildInfra.encode(message.infra, writer.uint32(146).fork()).ldelim();
    }
    for (const v of message.tags) {
      StringPair.encode(v!, writer.uint32(154).fork()).ldelim();
    }
    if (message.exe !== undefined) {
      Executable.encode(message.exe, writer.uint32(194).fork()).ldelim();
    }
    if (message.canary === true) {
      writer.uint32(200).bool(message.canary);
    }
    if (message.schedulingTimeout !== undefined) {
      Duration.encode(message.schedulingTimeout, writer.uint32(210).fork()).ldelim();
    }
    if (message.executionTimeout !== undefined) {
      Duration.encode(message.executionTimeout, writer.uint32(218).fork()).ldelim();
    }
    if (message.gracePeriod !== undefined) {
      Duration.encode(message.gracePeriod, writer.uint32(234).fork()).ldelim();
    }
    if (message.waitForCapacity === true) {
      writer.uint32(224).bool(message.waitForCapacity);
    }
    if (message.canOutliveParent === true) {
      writer.uint32(240).bool(message.canOutliveParent);
    }
    writer.uint32(250).fork();
    for (const v of message.ancestorIds) {
      writer.int64(v);
    }
    writer.ldelim();
    if (message.retriable !== 0) {
      writer.uint32(280).int32(message.retriable);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Build {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuild() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.id = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.builder = BuilderID.decode(reader, reader.uint32());
          continue;
        case 34:
          if (tag !== 274) {
            break;
          }

          message.builderInfo = Build_BuilderInfo.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.number = reader.int32();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.createdBy = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.viewUrl = reader.string();
          continue;
        case 23:
          if (tag !== 186) {
            break;
          }

          message.canceledBy = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.createTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.endTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.updateTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 32:
          if (tag !== 258) {
            break;
          }

          message.cancelTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 12:
          if (tag !== 96) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 20:
          if (tag !== 162) {
            break;
          }

          message.summaryMarkdown = reader.string();
          continue;
        case 33:
          if (tag !== 266) {
            break;
          }

          message.cancellationMarkdown = reader.string();
          continue;
        case 21:
          if (tag !== 168) {
            break;
          }

          message.critical = reader.int32() as any;
          continue;
        case 22:
          if (tag !== 178) {
            break;
          }

          message.statusDetails = StatusDetails.decode(reader, reader.uint32());
          continue;
        case 15:
          if (tag !== 122) {
            break;
          }

          message.input = Build_Input.decode(reader, reader.uint32());
          continue;
        case 16:
          if (tag !== 130) {
            break;
          }

          message.output = Build_Output.decode(reader, reader.uint32());
          continue;
        case 17:
          if (tag !== 138) {
            break;
          }

          message.steps.push(Step.decode(reader, reader.uint32()));
          continue;
        case 18:
          if (tag !== 146) {
            break;
          }

          message.infra = BuildInfra.decode(reader, reader.uint32());
          continue;
        case 19:
          if (tag !== 154) {
            break;
          }

          message.tags.push(StringPair.decode(reader, reader.uint32()));
          continue;
        case 24:
          if (tag !== 194) {
            break;
          }

          message.exe = Executable.decode(reader, reader.uint32());
          continue;
        case 25:
          if (tag !== 200) {
            break;
          }

          message.canary = reader.bool();
          continue;
        case 26:
          if (tag !== 210) {
            break;
          }

          message.schedulingTimeout = Duration.decode(reader, reader.uint32());
          continue;
        case 27:
          if (tag !== 218) {
            break;
          }

          message.executionTimeout = Duration.decode(reader, reader.uint32());
          continue;
        case 29:
          if (tag !== 234) {
            break;
          }

          message.gracePeriod = Duration.decode(reader, reader.uint32());
          continue;
        case 28:
          if (tag !== 224) {
            break;
          }

          message.waitForCapacity = reader.bool();
          continue;
        case 30:
          if (tag !== 240) {
            break;
          }

          message.canOutliveParent = reader.bool();
          continue;
        case 31:
          if (tag === 248) {
            message.ancestorIds.push(longToString(reader.int64() as Long));

            continue;
          }

          if (tag === 250) {
            const end2 = reader.uint32() + reader.pos;
            while (reader.pos < end2) {
              message.ancestorIds.push(longToString(reader.int64() as Long));
            }

            continue;
          }

          break;
        case 35:
          if (tag !== 280) {
            break;
          }

          message.retriable = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Build {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : "0",
      builder: isSet(object.builder) ? BuilderID.fromJSON(object.builder) : undefined,
      builderInfo: isSet(object.builderInfo) ? Build_BuilderInfo.fromJSON(object.builderInfo) : undefined,
      number: isSet(object.number) ? globalThis.Number(object.number) : 0,
      createdBy: isSet(object.createdBy) ? globalThis.String(object.createdBy) : "",
      viewUrl: isSet(object.viewUrl) ? globalThis.String(object.viewUrl) : "",
      canceledBy: isSet(object.canceledBy) ? globalThis.String(object.canceledBy) : "",
      createTime: isSet(object.createTime) ? globalThis.String(object.createTime) : undefined,
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      updateTime: isSet(object.updateTime) ? globalThis.String(object.updateTime) : undefined,
      cancelTime: isSet(object.cancelTime) ? globalThis.String(object.cancelTime) : undefined,
      status: isSet(object.status) ? statusFromJSON(object.status) : 0,
      summaryMarkdown: isSet(object.summaryMarkdown) ? globalThis.String(object.summaryMarkdown) : "",
      cancellationMarkdown: isSet(object.cancellationMarkdown) ? globalThis.String(object.cancellationMarkdown) : "",
      critical: isSet(object.critical) ? trinaryFromJSON(object.critical) : 0,
      statusDetails: isSet(object.statusDetails) ? StatusDetails.fromJSON(object.statusDetails) : undefined,
      input: isSet(object.input) ? Build_Input.fromJSON(object.input) : undefined,
      output: isSet(object.output) ? Build_Output.fromJSON(object.output) : undefined,
      steps: globalThis.Array.isArray(object?.steps) ? object.steps.map((e: any) => Step.fromJSON(e)) : [],
      infra: isSet(object.infra) ? BuildInfra.fromJSON(object.infra) : undefined,
      tags: globalThis.Array.isArray(object?.tags) ? object.tags.map((e: any) => StringPair.fromJSON(e)) : [],
      exe: isSet(object.exe) ? Executable.fromJSON(object.exe) : undefined,
      canary: isSet(object.canary) ? globalThis.Boolean(object.canary) : false,
      schedulingTimeout: isSet(object.schedulingTimeout) ? Duration.fromJSON(object.schedulingTimeout) : undefined,
      executionTimeout: isSet(object.executionTimeout) ? Duration.fromJSON(object.executionTimeout) : undefined,
      gracePeriod: isSet(object.gracePeriod) ? Duration.fromJSON(object.gracePeriod) : undefined,
      waitForCapacity: isSet(object.waitForCapacity) ? globalThis.Boolean(object.waitForCapacity) : false,
      canOutliveParent: isSet(object.canOutliveParent) ? globalThis.Boolean(object.canOutliveParent) : false,
      ancestorIds: globalThis.Array.isArray(object?.ancestorIds)
        ? object.ancestorIds.map((e: any) => globalThis.String(e))
        : [],
      retriable: isSet(object.retriable) ? trinaryFromJSON(object.retriable) : 0,
    };
  },

  toJSON(message: Build): unknown {
    const obj: any = {};
    if (message.id !== "0") {
      obj.id = message.id;
    }
    if (message.builder !== undefined) {
      obj.builder = BuilderID.toJSON(message.builder);
    }
    if (message.builderInfo !== undefined) {
      obj.builderInfo = Build_BuilderInfo.toJSON(message.builderInfo);
    }
    if (message.number !== 0) {
      obj.number = Math.round(message.number);
    }
    if (message.createdBy !== "") {
      obj.createdBy = message.createdBy;
    }
    if (message.viewUrl !== "") {
      obj.viewUrl = message.viewUrl;
    }
    if (message.canceledBy !== "") {
      obj.canceledBy = message.canceledBy;
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
    if (message.updateTime !== undefined) {
      obj.updateTime = message.updateTime;
    }
    if (message.cancelTime !== undefined) {
      obj.cancelTime = message.cancelTime;
    }
    if (message.status !== 0) {
      obj.status = statusToJSON(message.status);
    }
    if (message.summaryMarkdown !== "") {
      obj.summaryMarkdown = message.summaryMarkdown;
    }
    if (message.cancellationMarkdown !== "") {
      obj.cancellationMarkdown = message.cancellationMarkdown;
    }
    if (message.critical !== 0) {
      obj.critical = trinaryToJSON(message.critical);
    }
    if (message.statusDetails !== undefined) {
      obj.statusDetails = StatusDetails.toJSON(message.statusDetails);
    }
    if (message.input !== undefined) {
      obj.input = Build_Input.toJSON(message.input);
    }
    if (message.output !== undefined) {
      obj.output = Build_Output.toJSON(message.output);
    }
    if (message.steps?.length) {
      obj.steps = message.steps.map((e) => Step.toJSON(e));
    }
    if (message.infra !== undefined) {
      obj.infra = BuildInfra.toJSON(message.infra);
    }
    if (message.tags?.length) {
      obj.tags = message.tags.map((e) => StringPair.toJSON(e));
    }
    if (message.exe !== undefined) {
      obj.exe = Executable.toJSON(message.exe);
    }
    if (message.canary === true) {
      obj.canary = message.canary;
    }
    if (message.schedulingTimeout !== undefined) {
      obj.schedulingTimeout = Duration.toJSON(message.schedulingTimeout);
    }
    if (message.executionTimeout !== undefined) {
      obj.executionTimeout = Duration.toJSON(message.executionTimeout);
    }
    if (message.gracePeriod !== undefined) {
      obj.gracePeriod = Duration.toJSON(message.gracePeriod);
    }
    if (message.waitForCapacity === true) {
      obj.waitForCapacity = message.waitForCapacity;
    }
    if (message.canOutliveParent === true) {
      obj.canOutliveParent = message.canOutliveParent;
    }
    if (message.ancestorIds?.length) {
      obj.ancestorIds = message.ancestorIds;
    }
    if (message.retriable !== 0) {
      obj.retriable = trinaryToJSON(message.retriable);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Build>, I>>(base?: I): Build {
    return Build.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Build>, I>>(object: I): Build {
    const message = createBaseBuild() as any;
    message.id = object.id ?? "0";
    message.builder = (object.builder !== undefined && object.builder !== null)
      ? BuilderID.fromPartial(object.builder)
      : undefined;
    message.builderInfo = (object.builderInfo !== undefined && object.builderInfo !== null)
      ? Build_BuilderInfo.fromPartial(object.builderInfo)
      : undefined;
    message.number = object.number ?? 0;
    message.createdBy = object.createdBy ?? "";
    message.viewUrl = object.viewUrl ?? "";
    message.canceledBy = object.canceledBy ?? "";
    message.createTime = object.createTime ?? undefined;
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.updateTime = object.updateTime ?? undefined;
    message.cancelTime = object.cancelTime ?? undefined;
    message.status = object.status ?? 0;
    message.summaryMarkdown = object.summaryMarkdown ?? "";
    message.cancellationMarkdown = object.cancellationMarkdown ?? "";
    message.critical = object.critical ?? 0;
    message.statusDetails = (object.statusDetails !== undefined && object.statusDetails !== null)
      ? StatusDetails.fromPartial(object.statusDetails)
      : undefined;
    message.input = (object.input !== undefined && object.input !== null)
      ? Build_Input.fromPartial(object.input)
      : undefined;
    message.output = (object.output !== undefined && object.output !== null)
      ? Build_Output.fromPartial(object.output)
      : undefined;
    message.steps = object.steps?.map((e) => Step.fromPartial(e)) || [];
    message.infra = (object.infra !== undefined && object.infra !== null)
      ? BuildInfra.fromPartial(object.infra)
      : undefined;
    message.tags = object.tags?.map((e) => StringPair.fromPartial(e)) || [];
    message.exe = (object.exe !== undefined && object.exe !== null) ? Executable.fromPartial(object.exe) : undefined;
    message.canary = object.canary ?? false;
    message.schedulingTimeout = (object.schedulingTimeout !== undefined && object.schedulingTimeout !== null)
      ? Duration.fromPartial(object.schedulingTimeout)
      : undefined;
    message.executionTimeout = (object.executionTimeout !== undefined && object.executionTimeout !== null)
      ? Duration.fromPartial(object.executionTimeout)
      : undefined;
    message.gracePeriod = (object.gracePeriod !== undefined && object.gracePeriod !== null)
      ? Duration.fromPartial(object.gracePeriod)
      : undefined;
    message.waitForCapacity = object.waitForCapacity ?? false;
    message.canOutliveParent = object.canOutliveParent ?? false;
    message.ancestorIds = object.ancestorIds?.map((e) => e) || [];
    message.retriable = object.retriable ?? 0;
    return message;
  },
};

function createBaseBuild_Input(): Build_Input {
  return { properties: undefined, gitilesCommit: undefined, gerritChanges: [], experimental: false, experiments: [] };
}

export const Build_Input = {
  encode(message: Build_Input, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.properties !== undefined) {
      Struct.encode(Struct.wrap(message.properties), writer.uint32(10).fork()).ldelim();
    }
    if (message.gitilesCommit !== undefined) {
      GitilesCommit.encode(message.gitilesCommit, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.gerritChanges) {
      GerritChange.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    if (message.experimental === true) {
      writer.uint32(40).bool(message.experimental);
    }
    for (const v of message.experiments) {
      writer.uint32(50).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Build_Input {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuild_Input() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.properties = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.gitilesCommit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.gerritChanges.push(GerritChange.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.experimental = reader.bool();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.experiments.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Build_Input {
    return {
      properties: isObject(object.properties) ? object.properties : undefined,
      gitilesCommit: isSet(object.gitilesCommit) ? GitilesCommit.fromJSON(object.gitilesCommit) : undefined,
      gerritChanges: globalThis.Array.isArray(object?.gerritChanges)
        ? object.gerritChanges.map((e: any) => GerritChange.fromJSON(e))
        : [],
      experimental: isSet(object.experimental) ? globalThis.Boolean(object.experimental) : false,
      experiments: globalThis.Array.isArray(object?.experiments)
        ? object.experiments.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: Build_Input): unknown {
    const obj: any = {};
    if (message.properties !== undefined) {
      obj.properties = message.properties;
    }
    if (message.gitilesCommit !== undefined) {
      obj.gitilesCommit = GitilesCommit.toJSON(message.gitilesCommit);
    }
    if (message.gerritChanges?.length) {
      obj.gerritChanges = message.gerritChanges.map((e) => GerritChange.toJSON(e));
    }
    if (message.experimental === true) {
      obj.experimental = message.experimental;
    }
    if (message.experiments?.length) {
      obj.experiments = message.experiments;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Build_Input>, I>>(base?: I): Build_Input {
    return Build_Input.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Build_Input>, I>>(object: I): Build_Input {
    const message = createBaseBuild_Input() as any;
    message.properties = object.properties ?? undefined;
    message.gitilesCommit = (object.gitilesCommit !== undefined && object.gitilesCommit !== null)
      ? GitilesCommit.fromPartial(object.gitilesCommit)
      : undefined;
    message.gerritChanges = object.gerritChanges?.map((e) => GerritChange.fromPartial(e)) || [];
    message.experimental = object.experimental ?? false;
    message.experiments = object.experiments?.map((e) => e) || [];
    return message;
  },
};

function createBaseBuild_Output(): Build_Output {
  return {
    properties: undefined,
    gitilesCommit: undefined,
    logs: [],
    status: 0,
    statusDetails: undefined,
    summaryHtml: "",
    summaryMarkdown: "",
  };
}

export const Build_Output = {
  encode(message: Build_Output, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.properties !== undefined) {
      Struct.encode(Struct.wrap(message.properties), writer.uint32(10).fork()).ldelim();
    }
    if (message.gitilesCommit !== undefined) {
      GitilesCommit.encode(message.gitilesCommit, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.logs) {
      Log.encode(v!, writer.uint32(42).fork()).ldelim();
    }
    if (message.status !== 0) {
      writer.uint32(48).int32(message.status);
    }
    if (message.statusDetails !== undefined) {
      StatusDetails.encode(message.statusDetails, writer.uint32(58).fork()).ldelim();
    }
    if (message.summaryHtml !== "") {
      writer.uint32(66).string(message.summaryHtml);
    }
    if (message.summaryMarkdown !== "") {
      writer.uint32(18).string(message.summaryMarkdown);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Build_Output {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuild_Output() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.properties = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.gitilesCommit = GitilesCommit.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.logs.push(Log.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 48) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.statusDetails = StatusDetails.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.summaryHtml = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.summaryMarkdown = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Build_Output {
    return {
      properties: isObject(object.properties) ? object.properties : undefined,
      gitilesCommit: isSet(object.gitilesCommit) ? GitilesCommit.fromJSON(object.gitilesCommit) : undefined,
      logs: globalThis.Array.isArray(object?.logs) ? object.logs.map((e: any) => Log.fromJSON(e)) : [],
      status: isSet(object.status) ? statusFromJSON(object.status) : 0,
      statusDetails: isSet(object.statusDetails) ? StatusDetails.fromJSON(object.statusDetails) : undefined,
      summaryHtml: isSet(object.summaryHtml) ? globalThis.String(object.summaryHtml) : "",
      summaryMarkdown: isSet(object.summaryMarkdown) ? globalThis.String(object.summaryMarkdown) : "",
    };
  },

  toJSON(message: Build_Output): unknown {
    const obj: any = {};
    if (message.properties !== undefined) {
      obj.properties = message.properties;
    }
    if (message.gitilesCommit !== undefined) {
      obj.gitilesCommit = GitilesCommit.toJSON(message.gitilesCommit);
    }
    if (message.logs?.length) {
      obj.logs = message.logs.map((e) => Log.toJSON(e));
    }
    if (message.status !== 0) {
      obj.status = statusToJSON(message.status);
    }
    if (message.statusDetails !== undefined) {
      obj.statusDetails = StatusDetails.toJSON(message.statusDetails);
    }
    if (message.summaryHtml !== "") {
      obj.summaryHtml = message.summaryHtml;
    }
    if (message.summaryMarkdown !== "") {
      obj.summaryMarkdown = message.summaryMarkdown;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Build_Output>, I>>(base?: I): Build_Output {
    return Build_Output.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Build_Output>, I>>(object: I): Build_Output {
    const message = createBaseBuild_Output() as any;
    message.properties = object.properties ?? undefined;
    message.gitilesCommit = (object.gitilesCommit !== undefined && object.gitilesCommit !== null)
      ? GitilesCommit.fromPartial(object.gitilesCommit)
      : undefined;
    message.logs = object.logs?.map((e) => Log.fromPartial(e)) || [];
    message.status = object.status ?? 0;
    message.statusDetails = (object.statusDetails !== undefined && object.statusDetails !== null)
      ? StatusDetails.fromPartial(object.statusDetails)
      : undefined;
    message.summaryHtml = object.summaryHtml ?? "";
    message.summaryMarkdown = object.summaryMarkdown ?? "";
    return message;
  },
};

function createBaseBuild_BuilderInfo(): Build_BuilderInfo {
  return { description: "" };
}

export const Build_BuilderInfo = {
  encode(message: Build_BuilderInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.description !== "") {
      writer.uint32(10).string(message.description);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Build_BuilderInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuild_BuilderInfo() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.description = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Build_BuilderInfo {
    return { description: isSet(object.description) ? globalThis.String(object.description) : "" };
  },

  toJSON(message: Build_BuilderInfo): unknown {
    const obj: any = {};
    if (message.description !== "") {
      obj.description = message.description;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Build_BuilderInfo>, I>>(base?: I): Build_BuilderInfo {
    return Build_BuilderInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Build_BuilderInfo>, I>>(object: I): Build_BuilderInfo {
    const message = createBaseBuild_BuilderInfo() as any;
    message.description = object.description ?? "";
    return message;
  },
};

function createBaseInputDataRef(): InputDataRef {
  return { cas: undefined, cipd: undefined, onPath: [] };
}

export const InputDataRef = {
  encode(message: InputDataRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.cas !== undefined) {
      InputDataRef_CAS.encode(message.cas, writer.uint32(10).fork()).ldelim();
    }
    if (message.cipd !== undefined) {
      InputDataRef_CIPD.encode(message.cipd, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.onPath) {
      writer.uint32(26).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputDataRef {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputDataRef() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.cas = InputDataRef_CAS.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.cipd = InputDataRef_CIPD.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.onPath.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InputDataRef {
    return {
      cas: isSet(object.cas) ? InputDataRef_CAS.fromJSON(object.cas) : undefined,
      cipd: isSet(object.cipd) ? InputDataRef_CIPD.fromJSON(object.cipd) : undefined,
      onPath: globalThis.Array.isArray(object?.onPath) ? object.onPath.map((e: any) => globalThis.String(e)) : [],
    };
  },

  toJSON(message: InputDataRef): unknown {
    const obj: any = {};
    if (message.cas !== undefined) {
      obj.cas = InputDataRef_CAS.toJSON(message.cas);
    }
    if (message.cipd !== undefined) {
      obj.cipd = InputDataRef_CIPD.toJSON(message.cipd);
    }
    if (message.onPath?.length) {
      obj.onPath = message.onPath;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputDataRef>, I>>(base?: I): InputDataRef {
    return InputDataRef.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputDataRef>, I>>(object: I): InputDataRef {
    const message = createBaseInputDataRef() as any;
    message.cas = (object.cas !== undefined && object.cas !== null)
      ? InputDataRef_CAS.fromPartial(object.cas)
      : undefined;
    message.cipd = (object.cipd !== undefined && object.cipd !== null)
      ? InputDataRef_CIPD.fromPartial(object.cipd)
      : undefined;
    message.onPath = object.onPath?.map((e) => e) || [];
    return message;
  },
};

function createBaseInputDataRef_CAS(): InputDataRef_CAS {
  return { casInstance: "", digest: undefined };
}

export const InputDataRef_CAS = {
  encode(message: InputDataRef_CAS, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.casInstance !== "") {
      writer.uint32(10).string(message.casInstance);
    }
    if (message.digest !== undefined) {
      InputDataRef_CAS_Digest.encode(message.digest, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputDataRef_CAS {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputDataRef_CAS() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.casInstance = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.digest = InputDataRef_CAS_Digest.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InputDataRef_CAS {
    return {
      casInstance: isSet(object.casInstance) ? globalThis.String(object.casInstance) : "",
      digest: isSet(object.digest) ? InputDataRef_CAS_Digest.fromJSON(object.digest) : undefined,
    };
  },

  toJSON(message: InputDataRef_CAS): unknown {
    const obj: any = {};
    if (message.casInstance !== "") {
      obj.casInstance = message.casInstance;
    }
    if (message.digest !== undefined) {
      obj.digest = InputDataRef_CAS_Digest.toJSON(message.digest);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputDataRef_CAS>, I>>(base?: I): InputDataRef_CAS {
    return InputDataRef_CAS.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputDataRef_CAS>, I>>(object: I): InputDataRef_CAS {
    const message = createBaseInputDataRef_CAS() as any;
    message.casInstance = object.casInstance ?? "";
    message.digest = (object.digest !== undefined && object.digest !== null)
      ? InputDataRef_CAS_Digest.fromPartial(object.digest)
      : undefined;
    return message;
  },
};

function createBaseInputDataRef_CAS_Digest(): InputDataRef_CAS_Digest {
  return { hash: "", sizeBytes: "0" };
}

export const InputDataRef_CAS_Digest = {
  encode(message: InputDataRef_CAS_Digest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hash !== "") {
      writer.uint32(10).string(message.hash);
    }
    if (message.sizeBytes !== "0") {
      writer.uint32(16).int64(message.sizeBytes);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputDataRef_CAS_Digest {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputDataRef_CAS_Digest() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.hash = reader.string();
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.sizeBytes = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InputDataRef_CAS_Digest {
    return {
      hash: isSet(object.hash) ? globalThis.String(object.hash) : "",
      sizeBytes: isSet(object.sizeBytes) ? globalThis.String(object.sizeBytes) : "0",
    };
  },

  toJSON(message: InputDataRef_CAS_Digest): unknown {
    const obj: any = {};
    if (message.hash !== "") {
      obj.hash = message.hash;
    }
    if (message.sizeBytes !== "0") {
      obj.sizeBytes = message.sizeBytes;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputDataRef_CAS_Digest>, I>>(base?: I): InputDataRef_CAS_Digest {
    return InputDataRef_CAS_Digest.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputDataRef_CAS_Digest>, I>>(object: I): InputDataRef_CAS_Digest {
    const message = createBaseInputDataRef_CAS_Digest() as any;
    message.hash = object.hash ?? "";
    message.sizeBytes = object.sizeBytes ?? "0";
    return message;
  },
};

function createBaseInputDataRef_CIPD(): InputDataRef_CIPD {
  return { server: "", specs: [] };
}

export const InputDataRef_CIPD = {
  encode(message: InputDataRef_CIPD, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.server !== "") {
      writer.uint32(10).string(message.server);
    }
    for (const v of message.specs) {
      InputDataRef_CIPD_PkgSpec.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputDataRef_CIPD {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputDataRef_CIPD() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.server = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.specs.push(InputDataRef_CIPD_PkgSpec.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InputDataRef_CIPD {
    return {
      server: isSet(object.server) ? globalThis.String(object.server) : "",
      specs: globalThis.Array.isArray(object?.specs)
        ? object.specs.map((e: any) => InputDataRef_CIPD_PkgSpec.fromJSON(e))
        : [],
    };
  },

  toJSON(message: InputDataRef_CIPD): unknown {
    const obj: any = {};
    if (message.server !== "") {
      obj.server = message.server;
    }
    if (message.specs?.length) {
      obj.specs = message.specs.map((e) => InputDataRef_CIPD_PkgSpec.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputDataRef_CIPD>, I>>(base?: I): InputDataRef_CIPD {
    return InputDataRef_CIPD.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputDataRef_CIPD>, I>>(object: I): InputDataRef_CIPD {
    const message = createBaseInputDataRef_CIPD() as any;
    message.server = object.server ?? "";
    message.specs = object.specs?.map((e) => InputDataRef_CIPD_PkgSpec.fromPartial(e)) || [];
    return message;
  },
};

function createBaseInputDataRef_CIPD_PkgSpec(): InputDataRef_CIPD_PkgSpec {
  return { package: "", version: "" };
}

export const InputDataRef_CIPD_PkgSpec = {
  encode(message: InputDataRef_CIPD_PkgSpec, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.package !== "") {
      writer.uint32(10).string(message.package);
    }
    if (message.version !== "") {
      writer.uint32(18).string(message.version);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InputDataRef_CIPD_PkgSpec {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInputDataRef_CIPD_PkgSpec() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.package = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.version = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InputDataRef_CIPD_PkgSpec {
    return {
      package: isSet(object.package) ? globalThis.String(object.package) : "",
      version: isSet(object.version) ? globalThis.String(object.version) : "",
    };
  },

  toJSON(message: InputDataRef_CIPD_PkgSpec): unknown {
    const obj: any = {};
    if (message.package !== "") {
      obj.package = message.package;
    }
    if (message.version !== "") {
      obj.version = message.version;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InputDataRef_CIPD_PkgSpec>, I>>(base?: I): InputDataRef_CIPD_PkgSpec {
    return InputDataRef_CIPD_PkgSpec.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InputDataRef_CIPD_PkgSpec>, I>>(object: I): InputDataRef_CIPD_PkgSpec {
    const message = createBaseInputDataRef_CIPD_PkgSpec() as any;
    message.package = object.package ?? "";
    message.version = object.version ?? "";
    return message;
  },
};

function createBaseResolvedDataRef(): ResolvedDataRef {
  return { cas: undefined, cipd: undefined };
}

export const ResolvedDataRef = {
  encode(message: ResolvedDataRef, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.cas !== undefined) {
      ResolvedDataRef_CAS.encode(message.cas, writer.uint32(10).fork()).ldelim();
    }
    if (message.cipd !== undefined) {
      ResolvedDataRef_CIPD.encode(message.cipd, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolvedDataRef {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseResolvedDataRef() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.cas = ResolvedDataRef_CAS.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.cipd = ResolvedDataRef_CIPD.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ResolvedDataRef {
    return {
      cas: isSet(object.cas) ? ResolvedDataRef_CAS.fromJSON(object.cas) : undefined,
      cipd: isSet(object.cipd) ? ResolvedDataRef_CIPD.fromJSON(object.cipd) : undefined,
    };
  },

  toJSON(message: ResolvedDataRef): unknown {
    const obj: any = {};
    if (message.cas !== undefined) {
      obj.cas = ResolvedDataRef_CAS.toJSON(message.cas);
    }
    if (message.cipd !== undefined) {
      obj.cipd = ResolvedDataRef_CIPD.toJSON(message.cipd);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ResolvedDataRef>, I>>(base?: I): ResolvedDataRef {
    return ResolvedDataRef.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ResolvedDataRef>, I>>(object: I): ResolvedDataRef {
    const message = createBaseResolvedDataRef() as any;
    message.cas = (object.cas !== undefined && object.cas !== null)
      ? ResolvedDataRef_CAS.fromPartial(object.cas)
      : undefined;
    message.cipd = (object.cipd !== undefined && object.cipd !== null)
      ? ResolvedDataRef_CIPD.fromPartial(object.cipd)
      : undefined;
    return message;
  },
};

function createBaseResolvedDataRef_Timing(): ResolvedDataRef_Timing {
  return { fetchDuration: undefined, installDuration: undefined };
}

export const ResolvedDataRef_Timing = {
  encode(message: ResolvedDataRef_Timing, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.fetchDuration !== undefined) {
      Duration.encode(message.fetchDuration, writer.uint32(10).fork()).ldelim();
    }
    if (message.installDuration !== undefined) {
      Duration.encode(message.installDuration, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolvedDataRef_Timing {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseResolvedDataRef_Timing() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.fetchDuration = Duration.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.installDuration = Duration.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ResolvedDataRef_Timing {
    return {
      fetchDuration: isSet(object.fetchDuration) ? Duration.fromJSON(object.fetchDuration) : undefined,
      installDuration: isSet(object.installDuration) ? Duration.fromJSON(object.installDuration) : undefined,
    };
  },

  toJSON(message: ResolvedDataRef_Timing): unknown {
    const obj: any = {};
    if (message.fetchDuration !== undefined) {
      obj.fetchDuration = Duration.toJSON(message.fetchDuration);
    }
    if (message.installDuration !== undefined) {
      obj.installDuration = Duration.toJSON(message.installDuration);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ResolvedDataRef_Timing>, I>>(base?: I): ResolvedDataRef_Timing {
    return ResolvedDataRef_Timing.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ResolvedDataRef_Timing>, I>>(object: I): ResolvedDataRef_Timing {
    const message = createBaseResolvedDataRef_Timing() as any;
    message.fetchDuration = (object.fetchDuration !== undefined && object.fetchDuration !== null)
      ? Duration.fromPartial(object.fetchDuration)
      : undefined;
    message.installDuration = (object.installDuration !== undefined && object.installDuration !== null)
      ? Duration.fromPartial(object.installDuration)
      : undefined;
    return message;
  },
};

function createBaseResolvedDataRef_CAS(): ResolvedDataRef_CAS {
  return { timing: undefined };
}

export const ResolvedDataRef_CAS = {
  encode(message: ResolvedDataRef_CAS, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.timing !== undefined) {
      ResolvedDataRef_Timing.encode(message.timing, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolvedDataRef_CAS {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseResolvedDataRef_CAS() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.timing = ResolvedDataRef_Timing.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ResolvedDataRef_CAS {
    return { timing: isSet(object.timing) ? ResolvedDataRef_Timing.fromJSON(object.timing) : undefined };
  },

  toJSON(message: ResolvedDataRef_CAS): unknown {
    const obj: any = {};
    if (message.timing !== undefined) {
      obj.timing = ResolvedDataRef_Timing.toJSON(message.timing);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ResolvedDataRef_CAS>, I>>(base?: I): ResolvedDataRef_CAS {
    return ResolvedDataRef_CAS.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ResolvedDataRef_CAS>, I>>(object: I): ResolvedDataRef_CAS {
    const message = createBaseResolvedDataRef_CAS() as any;
    message.timing = (object.timing !== undefined && object.timing !== null)
      ? ResolvedDataRef_Timing.fromPartial(object.timing)
      : undefined;
    return message;
  },
};

function createBaseResolvedDataRef_CIPD(): ResolvedDataRef_CIPD {
  return { specs: [] };
}

export const ResolvedDataRef_CIPD = {
  encode(message: ResolvedDataRef_CIPD, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.specs) {
      ResolvedDataRef_CIPD_PkgSpec.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolvedDataRef_CIPD {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseResolvedDataRef_CIPD() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 2:
          if (tag !== 18) {
            break;
          }

          message.specs.push(ResolvedDataRef_CIPD_PkgSpec.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ResolvedDataRef_CIPD {
    return {
      specs: globalThis.Array.isArray(object?.specs)
        ? object.specs.map((e: any) => ResolvedDataRef_CIPD_PkgSpec.fromJSON(e))
        : [],
    };
  },

  toJSON(message: ResolvedDataRef_CIPD): unknown {
    const obj: any = {};
    if (message.specs?.length) {
      obj.specs = message.specs.map((e) => ResolvedDataRef_CIPD_PkgSpec.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ResolvedDataRef_CIPD>, I>>(base?: I): ResolvedDataRef_CIPD {
    return ResolvedDataRef_CIPD.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ResolvedDataRef_CIPD>, I>>(object: I): ResolvedDataRef_CIPD {
    const message = createBaseResolvedDataRef_CIPD() as any;
    message.specs = object.specs?.map((e) => ResolvedDataRef_CIPD_PkgSpec.fromPartial(e)) || [];
    return message;
  },
};

function createBaseResolvedDataRef_CIPD_PkgSpec(): ResolvedDataRef_CIPD_PkgSpec {
  return { skipped: false, package: "", version: "", wasCached: 0, timing: undefined };
}

export const ResolvedDataRef_CIPD_PkgSpec = {
  encode(message: ResolvedDataRef_CIPD_PkgSpec, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.skipped === true) {
      writer.uint32(8).bool(message.skipped);
    }
    if (message.package !== "") {
      writer.uint32(18).string(message.package);
    }
    if (message.version !== "") {
      writer.uint32(26).string(message.version);
    }
    if (message.wasCached !== 0) {
      writer.uint32(32).int32(message.wasCached);
    }
    if (message.timing !== undefined) {
      ResolvedDataRef_Timing.encode(message.timing, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResolvedDataRef_CIPD_PkgSpec {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseResolvedDataRef_CIPD_PkgSpec() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.skipped = reader.bool();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.package = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.version = reader.string();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.wasCached = reader.int32() as any;
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.timing = ResolvedDataRef_Timing.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): ResolvedDataRef_CIPD_PkgSpec {
    return {
      skipped: isSet(object.skipped) ? globalThis.Boolean(object.skipped) : false,
      package: isSet(object.package) ? globalThis.String(object.package) : "",
      version: isSet(object.version) ? globalThis.String(object.version) : "",
      wasCached: isSet(object.wasCached) ? trinaryFromJSON(object.wasCached) : 0,
      timing: isSet(object.timing) ? ResolvedDataRef_Timing.fromJSON(object.timing) : undefined,
    };
  },

  toJSON(message: ResolvedDataRef_CIPD_PkgSpec): unknown {
    const obj: any = {};
    if (message.skipped === true) {
      obj.skipped = message.skipped;
    }
    if (message.package !== "") {
      obj.package = message.package;
    }
    if (message.version !== "") {
      obj.version = message.version;
    }
    if (message.wasCached !== 0) {
      obj.wasCached = trinaryToJSON(message.wasCached);
    }
    if (message.timing !== undefined) {
      obj.timing = ResolvedDataRef_Timing.toJSON(message.timing);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<ResolvedDataRef_CIPD_PkgSpec>, I>>(base?: I): ResolvedDataRef_CIPD_PkgSpec {
    return ResolvedDataRef_CIPD_PkgSpec.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<ResolvedDataRef_CIPD_PkgSpec>, I>>(object: I): ResolvedDataRef_CIPD_PkgSpec {
    const message = createBaseResolvedDataRef_CIPD_PkgSpec() as any;
    message.skipped = object.skipped ?? false;
    message.package = object.package ?? "";
    message.version = object.version ?? "";
    message.wasCached = object.wasCached ?? 0;
    message.timing = (object.timing !== undefined && object.timing !== null)
      ? ResolvedDataRef_Timing.fromPartial(object.timing)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra(): BuildInfra {
  return {
    buildbucket: undefined,
    swarming: undefined,
    logdog: undefined,
    recipe: undefined,
    resultdb: undefined,
    bbagent: undefined,
    backend: undefined,
    led: undefined,
  };
}

export const BuildInfra = {
  encode(message: BuildInfra, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.buildbucket !== undefined) {
      BuildInfra_Buildbucket.encode(message.buildbucket, writer.uint32(10).fork()).ldelim();
    }
    if (message.swarming !== undefined) {
      BuildInfra_Swarming.encode(message.swarming, writer.uint32(18).fork()).ldelim();
    }
    if (message.logdog !== undefined) {
      BuildInfra_LogDog.encode(message.logdog, writer.uint32(26).fork()).ldelim();
    }
    if (message.recipe !== undefined) {
      BuildInfra_Recipe.encode(message.recipe, writer.uint32(34).fork()).ldelim();
    }
    if (message.resultdb !== undefined) {
      BuildInfra_ResultDB.encode(message.resultdb, writer.uint32(42).fork()).ldelim();
    }
    if (message.bbagent !== undefined) {
      BuildInfra_BBAgent.encode(message.bbagent, writer.uint32(50).fork()).ldelim();
    }
    if (message.backend !== undefined) {
      BuildInfra_Backend.encode(message.backend, writer.uint32(58).fork()).ldelim();
    }
    if (message.led !== undefined) {
      BuildInfra_Led.encode(message.led, writer.uint32(66).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buildbucket = BuildInfra_Buildbucket.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.swarming = BuildInfra_Swarming.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.logdog = BuildInfra_LogDog.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.recipe = BuildInfra_Recipe.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.resultdb = BuildInfra_ResultDB.decode(reader, reader.uint32());
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.bbagent = BuildInfra_BBAgent.decode(reader, reader.uint32());
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.backend = BuildInfra_Backend.decode(reader, reader.uint32());
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.led = BuildInfra_Led.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra {
    return {
      buildbucket: isSet(object.buildbucket) ? BuildInfra_Buildbucket.fromJSON(object.buildbucket) : undefined,
      swarming: isSet(object.swarming) ? BuildInfra_Swarming.fromJSON(object.swarming) : undefined,
      logdog: isSet(object.logdog) ? BuildInfra_LogDog.fromJSON(object.logdog) : undefined,
      recipe: isSet(object.recipe) ? BuildInfra_Recipe.fromJSON(object.recipe) : undefined,
      resultdb: isSet(object.resultdb) ? BuildInfra_ResultDB.fromJSON(object.resultdb) : undefined,
      bbagent: isSet(object.bbagent) ? BuildInfra_BBAgent.fromJSON(object.bbagent) : undefined,
      backend: isSet(object.backend) ? BuildInfra_Backend.fromJSON(object.backend) : undefined,
      led: isSet(object.led) ? BuildInfra_Led.fromJSON(object.led) : undefined,
    };
  },

  toJSON(message: BuildInfra): unknown {
    const obj: any = {};
    if (message.buildbucket !== undefined) {
      obj.buildbucket = BuildInfra_Buildbucket.toJSON(message.buildbucket);
    }
    if (message.swarming !== undefined) {
      obj.swarming = BuildInfra_Swarming.toJSON(message.swarming);
    }
    if (message.logdog !== undefined) {
      obj.logdog = BuildInfra_LogDog.toJSON(message.logdog);
    }
    if (message.recipe !== undefined) {
      obj.recipe = BuildInfra_Recipe.toJSON(message.recipe);
    }
    if (message.resultdb !== undefined) {
      obj.resultdb = BuildInfra_ResultDB.toJSON(message.resultdb);
    }
    if (message.bbagent !== undefined) {
      obj.bbagent = BuildInfra_BBAgent.toJSON(message.bbagent);
    }
    if (message.backend !== undefined) {
      obj.backend = BuildInfra_Backend.toJSON(message.backend);
    }
    if (message.led !== undefined) {
      obj.led = BuildInfra_Led.toJSON(message.led);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra>, I>>(base?: I): BuildInfra {
    return BuildInfra.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra>, I>>(object: I): BuildInfra {
    const message = createBaseBuildInfra() as any;
    message.buildbucket = (object.buildbucket !== undefined && object.buildbucket !== null)
      ? BuildInfra_Buildbucket.fromPartial(object.buildbucket)
      : undefined;
    message.swarming = (object.swarming !== undefined && object.swarming !== null)
      ? BuildInfra_Swarming.fromPartial(object.swarming)
      : undefined;
    message.logdog = (object.logdog !== undefined && object.logdog !== null)
      ? BuildInfra_LogDog.fromPartial(object.logdog)
      : undefined;
    message.recipe = (object.recipe !== undefined && object.recipe !== null)
      ? BuildInfra_Recipe.fromPartial(object.recipe)
      : undefined;
    message.resultdb = (object.resultdb !== undefined && object.resultdb !== null)
      ? BuildInfra_ResultDB.fromPartial(object.resultdb)
      : undefined;
    message.bbagent = (object.bbagent !== undefined && object.bbagent !== null)
      ? BuildInfra_BBAgent.fromPartial(object.bbagent)
      : undefined;
    message.backend = (object.backend !== undefined && object.backend !== null)
      ? BuildInfra_Backend.fromPartial(object.backend)
      : undefined;
    message.led = (object.led !== undefined && object.led !== null)
      ? BuildInfra_Led.fromPartial(object.led)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket(): BuildInfra_Buildbucket {
  return {
    serviceConfigRevision: "",
    requestedProperties: undefined,
    requestedDimensions: [],
    hostname: "",
    experimentReasons: {},
    agentExecutable: {},
    agent: undefined,
    knownPublicGerritHosts: [],
    buildNumber: false,
  };
}

export const BuildInfra_Buildbucket = {
  encode(message: BuildInfra_Buildbucket, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.serviceConfigRevision !== "") {
      writer.uint32(18).string(message.serviceConfigRevision);
    }
    if (message.requestedProperties !== undefined) {
      Struct.encode(Struct.wrap(message.requestedProperties), writer.uint32(42).fork()).ldelim();
    }
    for (const v of message.requestedDimensions) {
      RequestedDimension.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    if (message.hostname !== "") {
      writer.uint32(58).string(message.hostname);
    }
    Object.entries(message.experimentReasons).forEach(([key, value]) => {
      BuildInfra_Buildbucket_ExperimentReasonsEntry.encode({ key: key as any, value }, writer.uint32(66).fork())
        .ldelim();
    });
    Object.entries(message.agentExecutable).forEach(([key, value]) => {
      BuildInfra_Buildbucket_AgentExecutableEntry.encode({ key: key as any, value }, writer.uint32(74).fork()).ldelim();
    });
    if (message.agent !== undefined) {
      BuildInfra_Buildbucket_Agent.encode(message.agent, writer.uint32(82).fork()).ldelim();
    }
    for (const v of message.knownPublicGerritHosts) {
      writer.uint32(90).string(v!);
    }
    if (message.buildNumber === true) {
      writer.uint32(96).bool(message.buildNumber);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 2:
          if (tag !== 18) {
            break;
          }

          message.serviceConfigRevision = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.requestedProperties = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.requestedDimensions.push(RequestedDimension.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.hostname = reader.string();
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          const entry8 = BuildInfra_Buildbucket_ExperimentReasonsEntry.decode(reader, reader.uint32());
          if (entry8.value !== undefined) {
            message.experimentReasons[entry8.key] = entry8.value;
          }
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          const entry9 = BuildInfra_Buildbucket_AgentExecutableEntry.decode(reader, reader.uint32());
          if (entry9.value !== undefined) {
            message.agentExecutable[entry9.key] = entry9.value;
          }
          continue;
        case 10:
          if (tag !== 82) {
            break;
          }

          message.agent = BuildInfra_Buildbucket_Agent.decode(reader, reader.uint32());
          continue;
        case 11:
          if (tag !== 90) {
            break;
          }

          message.knownPublicGerritHosts.push(reader.string());
          continue;
        case 12:
          if (tag !== 96) {
            break;
          }

          message.buildNumber = reader.bool();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket {
    return {
      serviceConfigRevision: isSet(object.serviceConfigRevision) ? globalThis.String(object.serviceConfigRevision) : "",
      requestedProperties: isObject(object.requestedProperties) ? object.requestedProperties : undefined,
      requestedDimensions: globalThis.Array.isArray(object?.requestedDimensions)
        ? object.requestedDimensions.map((e: any) => RequestedDimension.fromJSON(e))
        : [],
      hostname: isSet(object.hostname) ? globalThis.String(object.hostname) : "",
      experimentReasons: isObject(object.experimentReasons)
        ? Object.entries(object.experimentReasons).reduce<{ [key: string]: BuildInfra_Buildbucket_ExperimentReason }>(
          (acc, [key, value]) => {
            acc[key] = buildInfra_Buildbucket_ExperimentReasonFromJSON(value);
            return acc;
          },
          {},
        )
        : {},
      agentExecutable: isObject(object.agentExecutable)
        ? Object.entries(object.agentExecutable).reduce<{ [key: string]: ResolvedDataRef }>((acc, [key, value]) => {
          acc[key] = ResolvedDataRef.fromJSON(value);
          return acc;
        }, {})
        : {},
      agent: isSet(object.agent) ? BuildInfra_Buildbucket_Agent.fromJSON(object.agent) : undefined,
      knownPublicGerritHosts: globalThis.Array.isArray(object?.knownPublicGerritHosts)
        ? object.knownPublicGerritHosts.map((e: any) => globalThis.String(e))
        : [],
      buildNumber: isSet(object.buildNumber) ? globalThis.Boolean(object.buildNumber) : false,
    };
  },

  toJSON(message: BuildInfra_Buildbucket): unknown {
    const obj: any = {};
    if (message.serviceConfigRevision !== "") {
      obj.serviceConfigRevision = message.serviceConfigRevision;
    }
    if (message.requestedProperties !== undefined) {
      obj.requestedProperties = message.requestedProperties;
    }
    if (message.requestedDimensions?.length) {
      obj.requestedDimensions = message.requestedDimensions.map((e) => RequestedDimension.toJSON(e));
    }
    if (message.hostname !== "") {
      obj.hostname = message.hostname;
    }
    if (message.experimentReasons) {
      const entries = Object.entries(message.experimentReasons);
      if (entries.length > 0) {
        obj.experimentReasons = {};
        entries.forEach(([k, v]) => {
          obj.experimentReasons[k] = buildInfra_Buildbucket_ExperimentReasonToJSON(v);
        });
      }
    }
    if (message.agentExecutable) {
      const entries = Object.entries(message.agentExecutable);
      if (entries.length > 0) {
        obj.agentExecutable = {};
        entries.forEach(([k, v]) => {
          obj.agentExecutable[k] = ResolvedDataRef.toJSON(v);
        });
      }
    }
    if (message.agent !== undefined) {
      obj.agent = BuildInfra_Buildbucket_Agent.toJSON(message.agent);
    }
    if (message.knownPublicGerritHosts?.length) {
      obj.knownPublicGerritHosts = message.knownPublicGerritHosts;
    }
    if (message.buildNumber === true) {
      obj.buildNumber = message.buildNumber;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket>, I>>(base?: I): BuildInfra_Buildbucket {
    return BuildInfra_Buildbucket.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket>, I>>(object: I): BuildInfra_Buildbucket {
    const message = createBaseBuildInfra_Buildbucket() as any;
    message.serviceConfigRevision = object.serviceConfigRevision ?? "";
    message.requestedProperties = object.requestedProperties ?? undefined;
    message.requestedDimensions = object.requestedDimensions?.map((e) => RequestedDimension.fromPartial(e)) || [];
    message.hostname = object.hostname ?? "";
    message.experimentReasons = Object.entries(object.experimentReasons ?? {}).reduce<
      { [key: string]: BuildInfra_Buildbucket_ExperimentReason }
    >((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = value as BuildInfra_Buildbucket_ExperimentReason;
      }
      return acc;
    }, {});
    message.agentExecutable = Object.entries(object.agentExecutable ?? {}).reduce<{ [key: string]: ResolvedDataRef }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ResolvedDataRef.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.agent = (object.agent !== undefined && object.agent !== null)
      ? BuildInfra_Buildbucket_Agent.fromPartial(object.agent)
      : undefined;
    message.knownPublicGerritHosts = object.knownPublicGerritHosts?.map((e) => e) || [];
    message.buildNumber = object.buildNumber ?? false;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent(): BuildInfra_Buildbucket_Agent {
  return {
    input: undefined,
    output: undefined,
    source: undefined,
    purposes: {},
    cipdClientCache: undefined,
    cipdPackagesCache: undefined,
  };
}

export const BuildInfra_Buildbucket_Agent = {
  encode(message: BuildInfra_Buildbucket_Agent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.input !== undefined) {
      BuildInfra_Buildbucket_Agent_Input.encode(message.input, writer.uint32(10).fork()).ldelim();
    }
    if (message.output !== undefined) {
      BuildInfra_Buildbucket_Agent_Output.encode(message.output, writer.uint32(18).fork()).ldelim();
    }
    if (message.source !== undefined) {
      BuildInfra_Buildbucket_Agent_Source.encode(message.source, writer.uint32(26).fork()).ldelim();
    }
    Object.entries(message.purposes).forEach(([key, value]) => {
      BuildInfra_Buildbucket_Agent_PurposesEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
    });
    if (message.cipdClientCache !== undefined) {
      CacheEntry.encode(message.cipdClientCache, writer.uint32(42).fork()).ldelim();
    }
    if (message.cipdPackagesCache !== undefined) {
      CacheEntry.encode(message.cipdPackagesCache, writer.uint32(50).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.input = BuildInfra_Buildbucket_Agent_Input.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.output = BuildInfra_Buildbucket_Agent_Output.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.source = BuildInfra_Buildbucket_Agent_Source.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          const entry4 = BuildInfra_Buildbucket_Agent_PurposesEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.purposes[entry4.key] = entry4.value;
          }
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.cipdClientCache = CacheEntry.decode(reader, reader.uint32());
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.cipdPackagesCache = CacheEntry.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent {
    return {
      input: isSet(object.input) ? BuildInfra_Buildbucket_Agent_Input.fromJSON(object.input) : undefined,
      output: isSet(object.output) ? BuildInfra_Buildbucket_Agent_Output.fromJSON(object.output) : undefined,
      source: isSet(object.source) ? BuildInfra_Buildbucket_Agent_Source.fromJSON(object.source) : undefined,
      purposes: isObject(object.purposes)
        ? Object.entries(object.purposes).reduce<{ [key: string]: BuildInfra_Buildbucket_Agent_Purpose }>(
          (acc, [key, value]) => {
            acc[key] = buildInfra_Buildbucket_Agent_PurposeFromJSON(value);
            return acc;
          },
          {},
        )
        : {},
      cipdClientCache: isSet(object.cipdClientCache) ? CacheEntry.fromJSON(object.cipdClientCache) : undefined,
      cipdPackagesCache: isSet(object.cipdPackagesCache) ? CacheEntry.fromJSON(object.cipdPackagesCache) : undefined,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent): unknown {
    const obj: any = {};
    if (message.input !== undefined) {
      obj.input = BuildInfra_Buildbucket_Agent_Input.toJSON(message.input);
    }
    if (message.output !== undefined) {
      obj.output = BuildInfra_Buildbucket_Agent_Output.toJSON(message.output);
    }
    if (message.source !== undefined) {
      obj.source = BuildInfra_Buildbucket_Agent_Source.toJSON(message.source);
    }
    if (message.purposes) {
      const entries = Object.entries(message.purposes);
      if (entries.length > 0) {
        obj.purposes = {};
        entries.forEach(([k, v]) => {
          obj.purposes[k] = buildInfra_Buildbucket_Agent_PurposeToJSON(v);
        });
      }
    }
    if (message.cipdClientCache !== undefined) {
      obj.cipdClientCache = CacheEntry.toJSON(message.cipdClientCache);
    }
    if (message.cipdPackagesCache !== undefined) {
      obj.cipdPackagesCache = CacheEntry.toJSON(message.cipdPackagesCache);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent>, I>>(base?: I): BuildInfra_Buildbucket_Agent {
    return BuildInfra_Buildbucket_Agent.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent>, I>>(object: I): BuildInfra_Buildbucket_Agent {
    const message = createBaseBuildInfra_Buildbucket_Agent() as any;
    message.input = (object.input !== undefined && object.input !== null)
      ? BuildInfra_Buildbucket_Agent_Input.fromPartial(object.input)
      : undefined;
    message.output = (object.output !== undefined && object.output !== null)
      ? BuildInfra_Buildbucket_Agent_Output.fromPartial(object.output)
      : undefined;
    message.source = (object.source !== undefined && object.source !== null)
      ? BuildInfra_Buildbucket_Agent_Source.fromPartial(object.source)
      : undefined;
    message.purposes = Object.entries(object.purposes ?? {}).reduce<
      { [key: string]: BuildInfra_Buildbucket_Agent_Purpose }
    >((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = value as BuildInfra_Buildbucket_Agent_Purpose;
      }
      return acc;
    }, {});
    message.cipdClientCache = (object.cipdClientCache !== undefined && object.cipdClientCache !== null)
      ? CacheEntry.fromPartial(object.cipdClientCache)
      : undefined;
    message.cipdPackagesCache = (object.cipdPackagesCache !== undefined && object.cipdPackagesCache !== null)
      ? CacheEntry.fromPartial(object.cipdPackagesCache)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Source(): BuildInfra_Buildbucket_Agent_Source {
  return { cipd: undefined };
}

export const BuildInfra_Buildbucket_Agent_Source = {
  encode(message: BuildInfra_Buildbucket_Agent_Source, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.cipd !== undefined) {
      BuildInfra_Buildbucket_Agent_Source_CIPD.encode(message.cipd, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Source {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Source() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.cipd = BuildInfra_Buildbucket_Agent_Source_CIPD.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Source {
    return { cipd: isSet(object.cipd) ? BuildInfra_Buildbucket_Agent_Source_CIPD.fromJSON(object.cipd) : undefined };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Source): unknown {
    const obj: any = {};
    if (message.cipd !== undefined) {
      obj.cipd = BuildInfra_Buildbucket_Agent_Source_CIPD.toJSON(message.cipd);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Source>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Source {
    return BuildInfra_Buildbucket_Agent_Source.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Source>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Source {
    const message = createBaseBuildInfra_Buildbucket_Agent_Source() as any;
    message.cipd = (object.cipd !== undefined && object.cipd !== null)
      ? BuildInfra_Buildbucket_Agent_Source_CIPD.fromPartial(object.cipd)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Source_CIPD(): BuildInfra_Buildbucket_Agent_Source_CIPD {
  return { package: "", version: "", server: "", resolvedInstances: {} };
}

export const BuildInfra_Buildbucket_Agent_Source_CIPD = {
  encode(message: BuildInfra_Buildbucket_Agent_Source_CIPD, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.package !== "") {
      writer.uint32(10).string(message.package);
    }
    if (message.version !== "") {
      writer.uint32(18).string(message.version);
    }
    if (message.server !== "") {
      writer.uint32(26).string(message.server);
    }
    Object.entries(message.resolvedInstances).forEach(([key, value]) => {
      BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry.encode(
        { key: key as any, value },
        writer.uint32(34).fork(),
      ).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Source_CIPD {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Source_CIPD() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.package = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.version = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.server = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          const entry4 = BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry.decode(
            reader,
            reader.uint32(),
          );
          if (entry4.value !== undefined) {
            message.resolvedInstances[entry4.key] = entry4.value;
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

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Source_CIPD {
    return {
      package: isSet(object.package) ? globalThis.String(object.package) : "",
      version: isSet(object.version) ? globalThis.String(object.version) : "",
      server: isSet(object.server) ? globalThis.String(object.server) : "",
      resolvedInstances: isObject(object.resolvedInstances)
        ? Object.entries(object.resolvedInstances).reduce<{ [key: string]: string }>((acc, [key, value]) => {
          acc[key] = String(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Source_CIPD): unknown {
    const obj: any = {};
    if (message.package !== "") {
      obj.package = message.package;
    }
    if (message.version !== "") {
      obj.version = message.version;
    }
    if (message.server !== "") {
      obj.server = message.server;
    }
    if (message.resolvedInstances) {
      const entries = Object.entries(message.resolvedInstances);
      if (entries.length > 0) {
        obj.resolvedInstances = {};
        entries.forEach(([k, v]) => {
          obj.resolvedInstances[k] = v;
        });
      }
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Source_CIPD>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Source_CIPD {
    return BuildInfra_Buildbucket_Agent_Source_CIPD.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Source_CIPD>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Source_CIPD {
    const message = createBaseBuildInfra_Buildbucket_Agent_Source_CIPD() as any;
    message.package = object.package ?? "";
    message.version = object.version ?? "";
    message.server = object.server ?? "";
    message.resolvedInstances = Object.entries(object.resolvedInstances ?? {}).reduce<{ [key: string]: string }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = globalThis.String(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry(): BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry {
  return { key: "", value: "" };
}

export const BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry = {
  encode(
    message: BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry() as any;
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

          message.value = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry {
    return BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry {
    const message = createBaseBuildInfra_Buildbucket_Agent_Source_CIPD_ResolvedInstancesEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Input(): BuildInfra_Buildbucket_Agent_Input {
  return { data: {}, cipdSource: {} };
}

export const BuildInfra_Buildbucket_Agent_Input = {
  encode(message: BuildInfra_Buildbucket_Agent_Input, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.data).forEach(([key, value]) => {
      BuildInfra_Buildbucket_Agent_Input_DataEntry.encode({ key: key as any, value }, writer.uint32(10).fork())
        .ldelim();
    });
    Object.entries(message.cipdSource).forEach(([key, value]) => {
      BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry.encode({ key: key as any, value }, writer.uint32(18).fork())
        .ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Input {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Input() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          const entry1 = BuildInfra_Buildbucket_Agent_Input_DataEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.data[entry1.key] = entry1.value;
          }
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          const entry2 = BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.cipdSource[entry2.key] = entry2.value;
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

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Input {
    return {
      data: isObject(object.data)
        ? Object.entries(object.data).reduce<{ [key: string]: InputDataRef }>((acc, [key, value]) => {
          acc[key] = InputDataRef.fromJSON(value);
          return acc;
        }, {})
        : {},
      cipdSource: isObject(object.cipdSource)
        ? Object.entries(object.cipdSource).reduce<{ [key: string]: InputDataRef }>((acc, [key, value]) => {
          acc[key] = InputDataRef.fromJSON(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Input): unknown {
    const obj: any = {};
    if (message.data) {
      const entries = Object.entries(message.data);
      if (entries.length > 0) {
        obj.data = {};
        entries.forEach(([k, v]) => {
          obj.data[k] = InputDataRef.toJSON(v);
        });
      }
    }
    if (message.cipdSource) {
      const entries = Object.entries(message.cipdSource);
      if (entries.length > 0) {
        obj.cipdSource = {};
        entries.forEach(([k, v]) => {
          obj.cipdSource[k] = InputDataRef.toJSON(v);
        });
      }
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Input>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Input {
    return BuildInfra_Buildbucket_Agent_Input.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Input>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Input {
    const message = createBaseBuildInfra_Buildbucket_Agent_Input() as any;
    message.data = Object.entries(object.data ?? {}).reduce<{ [key: string]: InputDataRef }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = InputDataRef.fromPartial(value);
      }
      return acc;
    }, {});
    message.cipdSource = Object.entries(object.cipdSource ?? {}).reduce<{ [key: string]: InputDataRef }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = InputDataRef.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Input_DataEntry(): BuildInfra_Buildbucket_Agent_Input_DataEntry {
  return { key: "", value: undefined };
}

export const BuildInfra_Buildbucket_Agent_Input_DataEntry = {
  encode(message: BuildInfra_Buildbucket_Agent_Input_DataEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      InputDataRef.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Input_DataEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Input_DataEntry() as any;
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

          message.value = InputDataRef.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Input_DataEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? InputDataRef.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Input_DataEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = InputDataRef.toJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Input_DataEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Input_DataEntry {
    return BuildInfra_Buildbucket_Agent_Input_DataEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Input_DataEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Input_DataEntry {
    const message = createBaseBuildInfra_Buildbucket_Agent_Input_DataEntry() as any;
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? InputDataRef.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Input_CipdSourceEntry(): BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry {
  return { key: "", value: undefined };
}

export const BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry = {
  encode(
    message: BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      InputDataRef.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Input_CipdSourceEntry() as any;
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

          message.value = InputDataRef.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? InputDataRef.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = InputDataRef.toJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry {
    return BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Input_CipdSourceEntry {
    const message = createBaseBuildInfra_Buildbucket_Agent_Input_CipdSourceEntry() as any;
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? InputDataRef.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Output(): BuildInfra_Buildbucket_Agent_Output {
  return {
    resolvedData: {},
    status: 0,
    statusDetails: undefined,
    summaryHtml: "",
    agentPlatform: "",
    totalDuration: undefined,
    summaryMarkdown: "",
  };
}

export const BuildInfra_Buildbucket_Agent_Output = {
  encode(message: BuildInfra_Buildbucket_Agent_Output, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.resolvedData).forEach(([key, value]) => {
      BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry.encode({ key: key as any, value }, writer.uint32(10).fork())
        .ldelim();
    });
    if (message.status !== 0) {
      writer.uint32(16).int32(message.status);
    }
    if (message.statusDetails !== undefined) {
      StatusDetails.encode(message.statusDetails, writer.uint32(26).fork()).ldelim();
    }
    if (message.summaryHtml !== "") {
      writer.uint32(34).string(message.summaryHtml);
    }
    if (message.agentPlatform !== "") {
      writer.uint32(42).string(message.agentPlatform);
    }
    if (message.totalDuration !== undefined) {
      Duration.encode(message.totalDuration, writer.uint32(50).fork()).ldelim();
    }
    if (message.summaryMarkdown !== "") {
      writer.uint32(58).string(message.summaryMarkdown);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Output {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Output() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          const entry1 = BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.resolvedData[entry1.key] = entry1.value;
          }
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.statusDetails = StatusDetails.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.summaryHtml = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.agentPlatform = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.totalDuration = Duration.decode(reader, reader.uint32());
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.summaryMarkdown = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Output {
    return {
      resolvedData: isObject(object.resolvedData)
        ? Object.entries(object.resolvedData).reduce<{ [key: string]: ResolvedDataRef }>((acc, [key, value]) => {
          acc[key] = ResolvedDataRef.fromJSON(value);
          return acc;
        }, {})
        : {},
      status: isSet(object.status) ? statusFromJSON(object.status) : 0,
      statusDetails: isSet(object.statusDetails) ? StatusDetails.fromJSON(object.statusDetails) : undefined,
      summaryHtml: isSet(object.summaryHtml) ? globalThis.String(object.summaryHtml) : "",
      agentPlatform: isSet(object.agentPlatform) ? globalThis.String(object.agentPlatform) : "",
      totalDuration: isSet(object.totalDuration) ? Duration.fromJSON(object.totalDuration) : undefined,
      summaryMarkdown: isSet(object.summaryMarkdown) ? globalThis.String(object.summaryMarkdown) : "",
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Output): unknown {
    const obj: any = {};
    if (message.resolvedData) {
      const entries = Object.entries(message.resolvedData);
      if (entries.length > 0) {
        obj.resolvedData = {};
        entries.forEach(([k, v]) => {
          obj.resolvedData[k] = ResolvedDataRef.toJSON(v);
        });
      }
    }
    if (message.status !== 0) {
      obj.status = statusToJSON(message.status);
    }
    if (message.statusDetails !== undefined) {
      obj.statusDetails = StatusDetails.toJSON(message.statusDetails);
    }
    if (message.summaryHtml !== "") {
      obj.summaryHtml = message.summaryHtml;
    }
    if (message.agentPlatform !== "") {
      obj.agentPlatform = message.agentPlatform;
    }
    if (message.totalDuration !== undefined) {
      obj.totalDuration = Duration.toJSON(message.totalDuration);
    }
    if (message.summaryMarkdown !== "") {
      obj.summaryMarkdown = message.summaryMarkdown;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Output>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Output {
    return BuildInfra_Buildbucket_Agent_Output.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Output>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Output {
    const message = createBaseBuildInfra_Buildbucket_Agent_Output() as any;
    message.resolvedData = Object.entries(object.resolvedData ?? {}).reduce<{ [key: string]: ResolvedDataRef }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = ResolvedDataRef.fromPartial(value);
        }
        return acc;
      },
      {},
    );
    message.status = object.status ?? 0;
    message.statusDetails = (object.statusDetails !== undefined && object.statusDetails !== null)
      ? StatusDetails.fromPartial(object.statusDetails)
      : undefined;
    message.summaryHtml = object.summaryHtml ?? "";
    message.agentPlatform = object.agentPlatform ?? "";
    message.totalDuration = (object.totalDuration !== undefined && object.totalDuration !== null)
      ? Duration.fromPartial(object.totalDuration)
      : undefined;
    message.summaryMarkdown = object.summaryMarkdown ?? "";
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry(): BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry {
  return { key: "", value: undefined };
}

export const BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry = {
  encode(
    message: BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ResolvedDataRef.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry() as any;
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

          message.value = ResolvedDataRef.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? ResolvedDataRef.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = ResolvedDataRef.toJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry {
    return BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry {
    const message = createBaseBuildInfra_Buildbucket_Agent_Output_ResolvedDataEntry() as any;
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ResolvedDataRef.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_Agent_PurposesEntry(): BuildInfra_Buildbucket_Agent_PurposesEntry {
  return { key: "", value: 0 };
}

export const BuildInfra_Buildbucket_Agent_PurposesEntry = {
  encode(message: BuildInfra_Buildbucket_Agent_PurposesEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== 0) {
      writer.uint32(16).int32(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_Agent_PurposesEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_Agent_PurposesEntry() as any;
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
          if (tag !== 16) {
            break;
          }

          message.value = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_Agent_PurposesEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? buildInfra_Buildbucket_Agent_PurposeFromJSON(object.value) : 0,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_Agent_PurposesEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== 0) {
      obj.value = buildInfra_Buildbucket_Agent_PurposeToJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_PurposesEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_Agent_PurposesEntry {
    return BuildInfra_Buildbucket_Agent_PurposesEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_Agent_PurposesEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_Agent_PurposesEntry {
    const message = createBaseBuildInfra_Buildbucket_Agent_PurposesEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? 0;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_ExperimentReasonsEntry(): BuildInfra_Buildbucket_ExperimentReasonsEntry {
  return { key: "", value: 0 };
}

export const BuildInfra_Buildbucket_ExperimentReasonsEntry = {
  encode(message: BuildInfra_Buildbucket_ExperimentReasonsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== 0) {
      writer.uint32(16).int32(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_ExperimentReasonsEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_ExperimentReasonsEntry() as any;
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
          if (tag !== 16) {
            break;
          }

          message.value = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_ExperimentReasonsEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? buildInfra_Buildbucket_ExperimentReasonFromJSON(object.value) : 0,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_ExperimentReasonsEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== 0) {
      obj.value = buildInfra_Buildbucket_ExperimentReasonToJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_ExperimentReasonsEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_ExperimentReasonsEntry {
    return BuildInfra_Buildbucket_ExperimentReasonsEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_ExperimentReasonsEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_ExperimentReasonsEntry {
    const message = createBaseBuildInfra_Buildbucket_ExperimentReasonsEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? 0;
    return message;
  },
};

function createBaseBuildInfra_Buildbucket_AgentExecutableEntry(): BuildInfra_Buildbucket_AgentExecutableEntry {
  return { key: "", value: undefined };
}

export const BuildInfra_Buildbucket_AgentExecutableEntry = {
  encode(message: BuildInfra_Buildbucket_AgentExecutableEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== undefined) {
      ResolvedDataRef.encode(message.value, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Buildbucket_AgentExecutableEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Buildbucket_AgentExecutableEntry() as any;
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

          message.value = ResolvedDataRef.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Buildbucket_AgentExecutableEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? ResolvedDataRef.fromJSON(object.value) : undefined,
    };
  },

  toJSON(message: BuildInfra_Buildbucket_AgentExecutableEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== undefined) {
      obj.value = ResolvedDataRef.toJSON(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Buildbucket_AgentExecutableEntry>, I>>(
    base?: I,
  ): BuildInfra_Buildbucket_AgentExecutableEntry {
    return BuildInfra_Buildbucket_AgentExecutableEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Buildbucket_AgentExecutableEntry>, I>>(
    object: I,
  ): BuildInfra_Buildbucket_AgentExecutableEntry {
    const message = createBaseBuildInfra_Buildbucket_AgentExecutableEntry() as any;
    message.key = object.key ?? "";
    message.value = (object.value !== undefined && object.value !== null)
      ? ResolvedDataRef.fromPartial(object.value)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Swarming(): BuildInfra_Swarming {
  return {
    hostname: "",
    taskId: "",
    parentRunId: "",
    taskServiceAccount: "",
    priority: 0,
    taskDimensions: [],
    botDimensions: [],
    caches: [],
  };
}

export const BuildInfra_Swarming = {
  encode(message: BuildInfra_Swarming, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hostname !== "") {
      writer.uint32(10).string(message.hostname);
    }
    if (message.taskId !== "") {
      writer.uint32(18).string(message.taskId);
    }
    if (message.parentRunId !== "") {
      writer.uint32(74).string(message.parentRunId);
    }
    if (message.taskServiceAccount !== "") {
      writer.uint32(26).string(message.taskServiceAccount);
    }
    if (message.priority !== 0) {
      writer.uint32(32).int32(message.priority);
    }
    for (const v of message.taskDimensions) {
      RequestedDimension.encode(v!, writer.uint32(42).fork()).ldelim();
    }
    for (const v of message.botDimensions) {
      StringPair.encode(v!, writer.uint32(50).fork()).ldelim();
    }
    for (const v of message.caches) {
      BuildInfra_Swarming_CacheEntry.encode(v!, writer.uint32(58).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Swarming {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Swarming() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.hostname = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.taskId = reader.string();
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.parentRunId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.taskServiceAccount = reader.string();
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.priority = reader.int32();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.taskDimensions.push(RequestedDimension.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.botDimensions.push(StringPair.decode(reader, reader.uint32()));
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.caches.push(BuildInfra_Swarming_CacheEntry.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Swarming {
    return {
      hostname: isSet(object.hostname) ? globalThis.String(object.hostname) : "",
      taskId: isSet(object.taskId) ? globalThis.String(object.taskId) : "",
      parentRunId: isSet(object.parentRunId) ? globalThis.String(object.parentRunId) : "",
      taskServiceAccount: isSet(object.taskServiceAccount) ? globalThis.String(object.taskServiceAccount) : "",
      priority: isSet(object.priority) ? globalThis.Number(object.priority) : 0,
      taskDimensions: globalThis.Array.isArray(object?.taskDimensions)
        ? object.taskDimensions.map((e: any) => RequestedDimension.fromJSON(e))
        : [],
      botDimensions: globalThis.Array.isArray(object?.botDimensions)
        ? object.botDimensions.map((e: any) => StringPair.fromJSON(e))
        : [],
      caches: globalThis.Array.isArray(object?.caches)
        ? object.caches.map((e: any) => BuildInfra_Swarming_CacheEntry.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BuildInfra_Swarming): unknown {
    const obj: any = {};
    if (message.hostname !== "") {
      obj.hostname = message.hostname;
    }
    if (message.taskId !== "") {
      obj.taskId = message.taskId;
    }
    if (message.parentRunId !== "") {
      obj.parentRunId = message.parentRunId;
    }
    if (message.taskServiceAccount !== "") {
      obj.taskServiceAccount = message.taskServiceAccount;
    }
    if (message.priority !== 0) {
      obj.priority = Math.round(message.priority);
    }
    if (message.taskDimensions?.length) {
      obj.taskDimensions = message.taskDimensions.map((e) => RequestedDimension.toJSON(e));
    }
    if (message.botDimensions?.length) {
      obj.botDimensions = message.botDimensions.map((e) => StringPair.toJSON(e));
    }
    if (message.caches?.length) {
      obj.caches = message.caches.map((e) => BuildInfra_Swarming_CacheEntry.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Swarming>, I>>(base?: I): BuildInfra_Swarming {
    return BuildInfra_Swarming.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Swarming>, I>>(object: I): BuildInfra_Swarming {
    const message = createBaseBuildInfra_Swarming() as any;
    message.hostname = object.hostname ?? "";
    message.taskId = object.taskId ?? "";
    message.parentRunId = object.parentRunId ?? "";
    message.taskServiceAccount = object.taskServiceAccount ?? "";
    message.priority = object.priority ?? 0;
    message.taskDimensions = object.taskDimensions?.map((e) => RequestedDimension.fromPartial(e)) || [];
    message.botDimensions = object.botDimensions?.map((e) => StringPair.fromPartial(e)) || [];
    message.caches = object.caches?.map((e) => BuildInfra_Swarming_CacheEntry.fromPartial(e)) || [];
    return message;
  },
};

function createBaseBuildInfra_Swarming_CacheEntry(): BuildInfra_Swarming_CacheEntry {
  return { name: "", path: "", waitForWarmCache: undefined, envVar: "" };
}

export const BuildInfra_Swarming_CacheEntry = {
  encode(message: BuildInfra_Swarming_CacheEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.path !== "") {
      writer.uint32(18).string(message.path);
    }
    if (message.waitForWarmCache !== undefined) {
      Duration.encode(message.waitForWarmCache, writer.uint32(26).fork()).ldelim();
    }
    if (message.envVar !== "") {
      writer.uint32(34).string(message.envVar);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Swarming_CacheEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Swarming_CacheEntry() as any;
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

          message.path = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.waitForWarmCache = Duration.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.envVar = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Swarming_CacheEntry {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      path: isSet(object.path) ? globalThis.String(object.path) : "",
      waitForWarmCache: isSet(object.waitForWarmCache) ? Duration.fromJSON(object.waitForWarmCache) : undefined,
      envVar: isSet(object.envVar) ? globalThis.String(object.envVar) : "",
    };
  },

  toJSON(message: BuildInfra_Swarming_CacheEntry): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.path !== "") {
      obj.path = message.path;
    }
    if (message.waitForWarmCache !== undefined) {
      obj.waitForWarmCache = Duration.toJSON(message.waitForWarmCache);
    }
    if (message.envVar !== "") {
      obj.envVar = message.envVar;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Swarming_CacheEntry>, I>>(base?: I): BuildInfra_Swarming_CacheEntry {
    return BuildInfra_Swarming_CacheEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Swarming_CacheEntry>, I>>(
    object: I,
  ): BuildInfra_Swarming_CacheEntry {
    const message = createBaseBuildInfra_Swarming_CacheEntry() as any;
    message.name = object.name ?? "";
    message.path = object.path ?? "";
    message.waitForWarmCache = (object.waitForWarmCache !== undefined && object.waitForWarmCache !== null)
      ? Duration.fromPartial(object.waitForWarmCache)
      : undefined;
    message.envVar = object.envVar ?? "";
    return message;
  },
};

function createBaseBuildInfra_LogDog(): BuildInfra_LogDog {
  return { hostname: "", project: "", prefix: "" };
}

export const BuildInfra_LogDog = {
  encode(message: BuildInfra_LogDog, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hostname !== "") {
      writer.uint32(10).string(message.hostname);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.prefix !== "") {
      writer.uint32(26).string(message.prefix);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_LogDog {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_LogDog() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.hostname = reader.string();
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

          message.prefix = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_LogDog {
    return {
      hostname: isSet(object.hostname) ? globalThis.String(object.hostname) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      prefix: isSet(object.prefix) ? globalThis.String(object.prefix) : "",
    };
  },

  toJSON(message: BuildInfra_LogDog): unknown {
    const obj: any = {};
    if (message.hostname !== "") {
      obj.hostname = message.hostname;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.prefix !== "") {
      obj.prefix = message.prefix;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_LogDog>, I>>(base?: I): BuildInfra_LogDog {
    return BuildInfra_LogDog.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_LogDog>, I>>(object: I): BuildInfra_LogDog {
    const message = createBaseBuildInfra_LogDog() as any;
    message.hostname = object.hostname ?? "";
    message.project = object.project ?? "";
    message.prefix = object.prefix ?? "";
    return message;
  },
};

function createBaseBuildInfra_Recipe(): BuildInfra_Recipe {
  return { cipdPackage: "", name: "" };
}

export const BuildInfra_Recipe = {
  encode(message: BuildInfra_Recipe, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.cipdPackage !== "") {
      writer.uint32(10).string(message.cipdPackage);
    }
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Recipe {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Recipe() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.cipdPackage = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
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

  fromJSON(object: any): BuildInfra_Recipe {
    return {
      cipdPackage: isSet(object.cipdPackage) ? globalThis.String(object.cipdPackage) : "",
      name: isSet(object.name) ? globalThis.String(object.name) : "",
    };
  },

  toJSON(message: BuildInfra_Recipe): unknown {
    const obj: any = {};
    if (message.cipdPackage !== "") {
      obj.cipdPackage = message.cipdPackage;
    }
    if (message.name !== "") {
      obj.name = message.name;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Recipe>, I>>(base?: I): BuildInfra_Recipe {
    return BuildInfra_Recipe.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Recipe>, I>>(object: I): BuildInfra_Recipe {
    const message = createBaseBuildInfra_Recipe() as any;
    message.cipdPackage = object.cipdPackage ?? "";
    message.name = object.name ?? "";
    return message;
  },
};

function createBaseBuildInfra_ResultDB(): BuildInfra_ResultDB {
  return { hostname: "", invocation: "", enable: false, bqExports: [], historyOptions: undefined };
}

export const BuildInfra_ResultDB = {
  encode(message: BuildInfra_ResultDB, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.hostname !== "") {
      writer.uint32(10).string(message.hostname);
    }
    if (message.invocation !== "") {
      writer.uint32(18).string(message.invocation);
    }
    if (message.enable === true) {
      writer.uint32(24).bool(message.enable);
    }
    for (const v of message.bqExports) {
      BigQueryExport.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    if (message.historyOptions !== undefined) {
      HistoryOptions.encode(message.historyOptions, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_ResultDB {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_ResultDB() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.hostname = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.invocation = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.enable = reader.bool();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.bqExports.push(BigQueryExport.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.historyOptions = HistoryOptions.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_ResultDB {
    return {
      hostname: isSet(object.hostname) ? globalThis.String(object.hostname) : "",
      invocation: isSet(object.invocation) ? globalThis.String(object.invocation) : "",
      enable: isSet(object.enable) ? globalThis.Boolean(object.enable) : false,
      bqExports: globalThis.Array.isArray(object?.bqExports)
        ? object.bqExports.map((e: any) => BigQueryExport.fromJSON(e))
        : [],
      historyOptions: isSet(object.historyOptions) ? HistoryOptions.fromJSON(object.historyOptions) : undefined,
    };
  },

  toJSON(message: BuildInfra_ResultDB): unknown {
    const obj: any = {};
    if (message.hostname !== "") {
      obj.hostname = message.hostname;
    }
    if (message.invocation !== "") {
      obj.invocation = message.invocation;
    }
    if (message.enable === true) {
      obj.enable = message.enable;
    }
    if (message.bqExports?.length) {
      obj.bqExports = message.bqExports.map((e) => BigQueryExport.toJSON(e));
    }
    if (message.historyOptions !== undefined) {
      obj.historyOptions = HistoryOptions.toJSON(message.historyOptions);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_ResultDB>, I>>(base?: I): BuildInfra_ResultDB {
    return BuildInfra_ResultDB.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_ResultDB>, I>>(object: I): BuildInfra_ResultDB {
    const message = createBaseBuildInfra_ResultDB() as any;
    message.hostname = object.hostname ?? "";
    message.invocation = object.invocation ?? "";
    message.enable = object.enable ?? false;
    message.bqExports = object.bqExports?.map((e) => BigQueryExport.fromPartial(e)) || [];
    message.historyOptions = (object.historyOptions !== undefined && object.historyOptions !== null)
      ? HistoryOptions.fromPartial(object.historyOptions)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_Led(): BuildInfra_Led {
  return { shadowedBucket: "" };
}

export const BuildInfra_Led = {
  encode(message: BuildInfra_Led, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.shadowedBucket !== "") {
      writer.uint32(10).string(message.shadowedBucket);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Led {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Led() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.shadowedBucket = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Led {
    return { shadowedBucket: isSet(object.shadowedBucket) ? globalThis.String(object.shadowedBucket) : "" };
  },

  toJSON(message: BuildInfra_Led): unknown {
    const obj: any = {};
    if (message.shadowedBucket !== "") {
      obj.shadowedBucket = message.shadowedBucket;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Led>, I>>(base?: I): BuildInfra_Led {
    return BuildInfra_Led.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Led>, I>>(object: I): BuildInfra_Led {
    const message = createBaseBuildInfra_Led() as any;
    message.shadowedBucket = object.shadowedBucket ?? "";
    return message;
  },
};

function createBaseBuildInfra_BBAgent(): BuildInfra_BBAgent {
  return { payloadPath: "", cacheDir: "", knownPublicGerritHosts: [], input: undefined };
}

export const BuildInfra_BBAgent = {
  encode(message: BuildInfra_BBAgent, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.payloadPath !== "") {
      writer.uint32(10).string(message.payloadPath);
    }
    if (message.cacheDir !== "") {
      writer.uint32(18).string(message.cacheDir);
    }
    for (const v of message.knownPublicGerritHosts) {
      writer.uint32(26).string(v!);
    }
    if (message.input !== undefined) {
      BuildInfra_BBAgent_Input.encode(message.input, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_BBAgent {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_BBAgent() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.payloadPath = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.cacheDir = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.knownPublicGerritHosts.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.input = BuildInfra_BBAgent_Input.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_BBAgent {
    return {
      payloadPath: isSet(object.payloadPath) ? globalThis.String(object.payloadPath) : "",
      cacheDir: isSet(object.cacheDir) ? globalThis.String(object.cacheDir) : "",
      knownPublicGerritHosts: globalThis.Array.isArray(object?.knownPublicGerritHosts)
        ? object.knownPublicGerritHosts.map((e: any) => globalThis.String(e))
        : [],
      input: isSet(object.input) ? BuildInfra_BBAgent_Input.fromJSON(object.input) : undefined,
    };
  },

  toJSON(message: BuildInfra_BBAgent): unknown {
    const obj: any = {};
    if (message.payloadPath !== "") {
      obj.payloadPath = message.payloadPath;
    }
    if (message.cacheDir !== "") {
      obj.cacheDir = message.cacheDir;
    }
    if (message.knownPublicGerritHosts?.length) {
      obj.knownPublicGerritHosts = message.knownPublicGerritHosts;
    }
    if (message.input !== undefined) {
      obj.input = BuildInfra_BBAgent_Input.toJSON(message.input);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_BBAgent>, I>>(base?: I): BuildInfra_BBAgent {
    return BuildInfra_BBAgent.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_BBAgent>, I>>(object: I): BuildInfra_BBAgent {
    const message = createBaseBuildInfra_BBAgent() as any;
    message.payloadPath = object.payloadPath ?? "";
    message.cacheDir = object.cacheDir ?? "";
    message.knownPublicGerritHosts = object.knownPublicGerritHosts?.map((e) => e) || [];
    message.input = (object.input !== undefined && object.input !== null)
      ? BuildInfra_BBAgent_Input.fromPartial(object.input)
      : undefined;
    return message;
  },
};

function createBaseBuildInfra_BBAgent_Input(): BuildInfra_BBAgent_Input {
  return { cipdPackages: [] };
}

export const BuildInfra_BBAgent_Input = {
  encode(message: BuildInfra_BBAgent_Input, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.cipdPackages) {
      BuildInfra_BBAgent_Input_CIPDPackage.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_BBAgent_Input {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_BBAgent_Input() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.cipdPackages.push(BuildInfra_BBAgent_Input_CIPDPackage.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_BBAgent_Input {
    return {
      cipdPackages: globalThis.Array.isArray(object?.cipdPackages)
        ? object.cipdPackages.map((e: any) => BuildInfra_BBAgent_Input_CIPDPackage.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BuildInfra_BBAgent_Input): unknown {
    const obj: any = {};
    if (message.cipdPackages?.length) {
      obj.cipdPackages = message.cipdPackages.map((e) => BuildInfra_BBAgent_Input_CIPDPackage.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_BBAgent_Input>, I>>(base?: I): BuildInfra_BBAgent_Input {
    return BuildInfra_BBAgent_Input.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_BBAgent_Input>, I>>(object: I): BuildInfra_BBAgent_Input {
    const message = createBaseBuildInfra_BBAgent_Input() as any;
    message.cipdPackages = object.cipdPackages?.map((e) => BuildInfra_BBAgent_Input_CIPDPackage.fromPartial(e)) || [];
    return message;
  },
};

function createBaseBuildInfra_BBAgent_Input_CIPDPackage(): BuildInfra_BBAgent_Input_CIPDPackage {
  return { name: "", version: "", server: "", path: "" };
}

export const BuildInfra_BBAgent_Input_CIPDPackage = {
  encode(message: BuildInfra_BBAgent_Input_CIPDPackage, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.version !== "") {
      writer.uint32(18).string(message.version);
    }
    if (message.server !== "") {
      writer.uint32(26).string(message.server);
    }
    if (message.path !== "") {
      writer.uint32(34).string(message.path);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_BBAgent_Input_CIPDPackage {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_BBAgent_Input_CIPDPackage() as any;
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

          message.version = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.server = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.path = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_BBAgent_Input_CIPDPackage {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      version: isSet(object.version) ? globalThis.String(object.version) : "",
      server: isSet(object.server) ? globalThis.String(object.server) : "",
      path: isSet(object.path) ? globalThis.String(object.path) : "",
    };
  },

  toJSON(message: BuildInfra_BBAgent_Input_CIPDPackage): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.version !== "") {
      obj.version = message.version;
    }
    if (message.server !== "") {
      obj.server = message.server;
    }
    if (message.path !== "") {
      obj.path = message.path;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_BBAgent_Input_CIPDPackage>, I>>(
    base?: I,
  ): BuildInfra_BBAgent_Input_CIPDPackage {
    return BuildInfra_BBAgent_Input_CIPDPackage.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_BBAgent_Input_CIPDPackage>, I>>(
    object: I,
  ): BuildInfra_BBAgent_Input_CIPDPackage {
    const message = createBaseBuildInfra_BBAgent_Input_CIPDPackage() as any;
    message.name = object.name ?? "";
    message.version = object.version ?? "";
    message.server = object.server ?? "";
    message.path = object.path ?? "";
    return message;
  },
};

function createBaseBuildInfra_Backend(): BuildInfra_Backend {
  return { config: undefined, task: undefined, caches: [], taskDimensions: [], hostname: "" };
}

export const BuildInfra_Backend = {
  encode(message: BuildInfra_Backend, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.config !== undefined) {
      Struct.encode(Struct.wrap(message.config), writer.uint32(10).fork()).ldelim();
    }
    if (message.task !== undefined) {
      Task.encode(message.task, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.caches) {
      CacheEntry.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.taskDimensions) {
      RequestedDimension.encode(v!, writer.uint32(42).fork()).ldelim();
    }
    if (message.hostname !== "") {
      writer.uint32(50).string(message.hostname);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildInfra_Backend {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildInfra_Backend() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.config = Struct.unwrap(Struct.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.task = Task.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.caches.push(CacheEntry.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.taskDimensions.push(RequestedDimension.decode(reader, reader.uint32()));
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.hostname = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildInfra_Backend {
    return {
      config: isObject(object.config) ? object.config : undefined,
      task: isSet(object.task) ? Task.fromJSON(object.task) : undefined,
      caches: globalThis.Array.isArray(object?.caches) ? object.caches.map((e: any) => CacheEntry.fromJSON(e)) : [],
      taskDimensions: globalThis.Array.isArray(object?.taskDimensions)
        ? object.taskDimensions.map((e: any) => RequestedDimension.fromJSON(e))
        : [],
      hostname: isSet(object.hostname) ? globalThis.String(object.hostname) : "",
    };
  },

  toJSON(message: BuildInfra_Backend): unknown {
    const obj: any = {};
    if (message.config !== undefined) {
      obj.config = message.config;
    }
    if (message.task !== undefined) {
      obj.task = Task.toJSON(message.task);
    }
    if (message.caches?.length) {
      obj.caches = message.caches.map((e) => CacheEntry.toJSON(e));
    }
    if (message.taskDimensions?.length) {
      obj.taskDimensions = message.taskDimensions.map((e) => RequestedDimension.toJSON(e));
    }
    if (message.hostname !== "") {
      obj.hostname = message.hostname;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildInfra_Backend>, I>>(base?: I): BuildInfra_Backend {
    return BuildInfra_Backend.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildInfra_Backend>, I>>(object: I): BuildInfra_Backend {
    const message = createBaseBuildInfra_Backend() as any;
    message.config = object.config ?? undefined;
    message.task = (object.task !== undefined && object.task !== null) ? Task.fromPartial(object.task) : undefined;
    message.caches = object.caches?.map((e) => CacheEntry.fromPartial(e)) || [];
    message.taskDimensions = object.taskDimensions?.map((e) => RequestedDimension.fromPartial(e)) || [];
    message.hostname = object.hostname ?? "";
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
