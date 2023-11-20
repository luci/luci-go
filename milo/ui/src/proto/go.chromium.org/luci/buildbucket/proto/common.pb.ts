/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal";
import { Duration } from "../../../../google/protobuf/duration.pb";
import { Timestamp } from "../../../../google/protobuf/timestamp.pb";

export const protobufPackage = "buildbucket.v2";

/** Status of a build or a step. */
export enum Status {
  /** UNSPECIFIED - Unspecified state. Meaning depends on the context. */
  UNSPECIFIED = 0,
  /** SCHEDULED - Build was scheduled, but did not start or end yet. */
  SCHEDULED = 1,
  /** STARTED - Build/step has started. */
  STARTED = 2,
  /**
   * ENDED_MASK - A union of all terminal statuses.
   * Can be used in BuildPredicate.status.
   * A concrete build/step cannot have this status.
   * Can be used as a bitmask to check that a build/step ended.
   */
  ENDED_MASK = 4,
  /**
   * SUCCESS - A build/step ended successfully.
   * This is a terminal status. It may not transition to another status.
   */
  SUCCESS = 12,
  /**
   * FAILURE - A build/step ended unsuccessfully due to its Build.Input,
   * e.g. tests failed, and NOT due to a build infrastructure failure.
   * This is a terminal status. It may not transition to another status.
   */
  FAILURE = 20,
  /**
   * INFRA_FAILURE - A build/step ended unsuccessfully due to a failure independent of the
   * input, e.g. swarming failed, not enough capacity or the recipe was unable
   * to read the patch from gerrit.
   * start_time is not required for this status.
   * This is a terminal status. It may not transition to another status.
   */
  INFRA_FAILURE = 36,
  /**
   * CANCELED - A build was cancelled explicitly, e.g. via an RPC.
   * This is a terminal status. It may not transition to another status.
   */
  CANCELED = 68,
}

export function statusFromJSON(object: any): Status {
  switch (object) {
    case 0:
    case "STATUS_UNSPECIFIED":
      return Status.UNSPECIFIED;
    case 1:
    case "SCHEDULED":
      return Status.SCHEDULED;
    case 2:
    case "STARTED":
      return Status.STARTED;
    case 4:
    case "ENDED_MASK":
      return Status.ENDED_MASK;
    case 12:
    case "SUCCESS":
      return Status.SUCCESS;
    case 20:
    case "FAILURE":
      return Status.FAILURE;
    case 36:
    case "INFRA_FAILURE":
      return Status.INFRA_FAILURE;
    case 68:
    case "CANCELED":
      return Status.CANCELED;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Status");
  }
}

export function statusToJSON(object: Status): string {
  switch (object) {
    case Status.UNSPECIFIED:
      return "STATUS_UNSPECIFIED";
    case Status.SCHEDULED:
      return "SCHEDULED";
    case Status.STARTED:
      return "STARTED";
    case Status.ENDED_MASK:
      return "ENDED_MASK";
    case Status.SUCCESS:
      return "SUCCESS";
    case Status.FAILURE:
      return "FAILURE";
    case Status.INFRA_FAILURE:
      return "INFRA_FAILURE";
    case Status.CANCELED:
      return "CANCELED";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Status");
  }
}

/** A boolean with an undefined value. */
export enum Trinary {
  UNSET = 0,
  YES = 1,
  NO = 2,
}

export function trinaryFromJSON(object: any): Trinary {
  switch (object) {
    case 0:
    case "UNSET":
      return Trinary.UNSET;
    case 1:
    case "YES":
      return Trinary.YES;
    case 2:
    case "NO":
      return Trinary.NO;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Trinary");
  }
}

export function trinaryToJSON(object: Trinary): string {
  switch (object) {
    case Trinary.UNSET:
      return "UNSET";
    case Trinary.YES:
      return "YES";
    case Trinary.NO:
      return "NO";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Trinary");
  }
}

/** Compression method used in the corresponding data. */
export enum Compression {
  ZLIB = 0,
  ZSTD = 1,
}

export function compressionFromJSON(object: any): Compression {
  switch (object) {
    case 0:
    case "ZLIB":
      return Compression.ZLIB;
    case 1:
    case "ZSTD":
      return Compression.ZSTD;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Compression");
  }
}

export function compressionToJSON(object: Compression): string {
  switch (object) {
    case Compression.ZLIB:
      return "ZLIB";
    case Compression.ZSTD:
      return "ZSTD";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Compression");
  }
}

/**
 * An executable to run when the build is ready to start.
 *
 * Please refer to go.chromium.org/luci/luciexe for the protocol this executable
 * is expected to implement.
 *
 * In addition to the "Host Application" responsibilities listed there,
 * buildbucket will also ensure that $CWD points to an empty directory when it
 * starts the build.
 */
export interface Executable {
  /**
   * The CIPD package containing the executable.
   *
   * See the `cmd` field below for how the executable will be located within the
   * package.
   */
  readonly cipdPackage: string;
  /**
   * The CIPD version to fetch.
   *
   * Optional. If omitted, this defaults to `latest`.
   */
  readonly cipdVersion: string;
  /**
   * The command to invoke within the package.
   *
   * The 0th argument is taken as relative to the cipd_package root (a.k.a.
   * BBAgentArgs.payload_path), so "foo" would invoke the binary called "foo" in
   * the root of the package. On Windows, this will automatically look
   * first for ".exe" and ".bat" variants. Similarly, "subdir/foo" would
   * look for "foo" in "subdir" of the CIPD package.
   *
   * The other arguments are passed verbatim to the executable.
   *
   * The 'build.proto' binary message will always be passed to stdin, even when
   * this command has arguments (see go.chromium.org/luci/luciexe).
   *
   * RECOMMENDATION: It's advised to rely on the build.proto's Input.Properties
   * field for passing task-specific data. Properties are JSON-typed and can be
   * modeled with a protobuf (using JSONPB). However, supplying additional args
   * can be useful to, e.g., increase logging verbosity, or similar
   * 'system level' settings within the binary.
   *
   * Optional. If omitted, defaults to `['luciexe']`.
   */
  readonly cmd: readonly string[];
  /**
   * Wrapper is a command and its args which will be used to 'wrap' the
   * execution of `cmd`.
   * Given:
   *  wrapper = ['/some/exe', '--arg']
   *  cmd = ['my_exe', '--other-arg']
   * Buildbucket's agent will invoke
   *  /some/exe --arg -- /path/to/task/root/dir/my_exe --other-arg
   * Note that '--' is always inserted between the wrapper and the target
   * cmd
   *
   * The wrapper program MUST maintain all the invariants specified in
   * go.chromium.org/luci/luciexe (likely by passing-through
   * most of this responsibility to `cmd`).
   *
   * wrapper[0] MAY be an absolute path. If https://pkg.go.dev/path/filepath#IsAbs
   * returns `true` for wrapper[0], it will be interpreted as an absolute
   * path. In this case, it is your responsibility to ensure that the target
   * binary is correctly deployed an any machine where the Build might run
   * (by whatever means you use to prepare/adjust your system image). Failure to do
   * so will cause the build to terminate with INFRA_FAILURE.
   *
   * If wrapper[0] is non-absolute, but does not contain a path separator,
   * it will be looked for in $PATH (and the same rules apply for
   * pre-distribution as in the absolute path case).
   *
   * If wrapper[0] begins with a "./" (or ".\") or contains a path separator
   * anywhere, it will be considered relative to the task root.
   *
   * Example wrapper[0]:
   *
   * Absolute path (*nix): /some/prog
   * Absolute path (Windows): C:\some\prog.exe
   * $PATH or %PATH% lookup: prog
   * task-relative (*nix): ./prog ($taskRoot/prog)
   * task-relative (*nix): dir/prog ($taskRoot/dir/prog)
   * task-relative (Windows): .\prog.exe ($taskRoot\\prog.exe)
   * task-relative (Windows): dir\prog.exe ($taskRoot\\dir\\prog.exe)
   */
  readonly wrapper: readonly string[];
}

/**
 * Machine-readable details of a status.
 * Human-readble details are present in a sibling summary_markdown field.
 */
export interface StatusDetails {
  /**
   * If set, indicates that the failure was due to a resource exhaustion / quota
   * denial.
   * Applicable in FAILURE and INFRA_FAILURE statuses.
   */
  readonly resourceExhaustion:
    | StatusDetails_ResourceExhaustion
    | undefined;
  /**
   * If set, indicates that the build ended due to the expiration_timeout or
   * scheduling_timeout set for the build.
   *
   * Applicable in all final statuses.
   *
   * SUCCESS+timeout would indicate a successful recovery from a timeout signal
   * during the build's grace_period.
   */
  readonly timeout: StatusDetails_Timeout | undefined;
}

export interface StatusDetails_ResourceExhaustion {
}

export interface StatusDetails_Timeout {
}

/** A named log of a step or build. */
export interface Log {
  /**
   * Log name, standard ("stdout", "stderr") or custom (e.g. "json.output").
   * Unique within the containing message (step or build).
   */
  readonly name: string;
  /** URL of a Human-readable page that displays log contents. */
  readonly viewUrl: string;
  /**
   * URL of the log content.
   * As of 2018-09-06, the only supported scheme is "logdog".
   * Typically it has form
   * "logdog://<host>/<project>/<prefix>/+/<stream_name>".
   * See also
   * https://godoc.org/go.chromium.org/luci/logdog/common/types#ParseURL
   */
  readonly url: string;
}

/** A Gerrit patchset. */
export interface GerritChange {
  /** Gerrit hostname, e.g. "chromium-review.googlesource.com". */
  readonly host: string;
  /** Gerrit project, e.g. "chromium/src". */
  readonly project: string;
  /** Change number, e.g. 12345. */
  readonly change: string;
  /** Patch set number, e.g. 1. */
  readonly patchset: string;
}

/** A landed Git commit hosted on Gitiles. */
export interface GitilesCommit {
  /** Gitiles hostname, e.g. "chromium.googlesource.com". */
  readonly host: string;
  /** Repository name on the host, e.g. "chromium/src". */
  readonly project: string;
  /** Commit HEX SHA1. */
  readonly id: string;
  /**
   * Commit ref, e.g. "refs/heads/master".
   * NOT a branch name: if specified, must start with "refs/".
   * If id is set, ref SHOULD also be set, so that git clients can
   * know how to obtain the commit by id.
   */
  readonly ref: string;
  /**
   * Defines a total order of commits on the ref. Requires ref field.
   * Typically 1-based, monotonically increasing, contiguous integer
   * defined by a Gerrit plugin, goto.google.com/git-numberer.
   * TODO(tandrii): make it a public doc.
   */
  readonly position: number;
}

/** A key-value pair of strings. */
export interface StringPair {
  readonly key: string;
  readonly value: string;
}

/** Half-open time range. */
export interface TimeRange {
  /** Inclusive lower boundary. Optional. */
  readonly startTime:
    | string
    | undefined;
  /** Exclusive upper boundary. Optional. */
  readonly endTime: string | undefined;
}

/** A requested dimension. Looks like StringPair, but also has an expiration. */
export interface RequestedDimension {
  readonly key: string;
  readonly value: string;
  /** If set, ignore this dimension after this duration. */
  readonly expiration: Duration | undefined;
}

/**
 * This message is a duplicate of Build.Infra.Swarming.CacheEntry,
 * however we will be moving from hardcoded swarming -> task backends.
 * This message will remain as the desired CacheEntry and eventually
 * Build.Infra.Swarming will be deprecated, so this will remain.
 *
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
 * Buildbucket implicitly declares cache
 *   {"name": "<hash(project/bucket/builder)>", "path": "builder"}.
 * This means that any LUCI builder has a "personal disk space" on the bot.
 * Builder cache is often a good start before customizing caching.
 * In recipes, it is available at api.buildbucket.builder_cache_path.
 *
 * To share a builder cache among multiple builders, it can be overridden:
 *
 *   builders {
 *     name: "a"
 *     caches {
 *       path: "builder"
 *       name: "my_shared_cache"
 *     }
 *   }
 *   builders {
 *     name: "b"
 *     caches {
 *       path: "builder"
 *       name: "my_shared_cache"
 *     }
 *   }
 *
 * Builders "a" and "b" share their builder cache. If an "a" build ran on a
 * bot and left some files in the builder cache and then a "b" build runs on
 * the same bot, the same files will be available in the builder cache.
 */
export interface CacheEntry {
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
   * In recipes, use api.path['cache'].join(path) to get absolute path.
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

export interface HealthStatus {
  /**
   * A numeric score for a builder's health.
   * The scores must respect the following:
   *   - 0: Unknown status
   *   - 1: The worst possible health
   *         e.g.
   *           - all bots are dead.
   *           - every single build has ended in INFRA_FAILURE in the configured
   *             time period.
   *   - 10: Completely healthy.
   *           e.g. Every single build has ended in SUCCESS or CANCELLED in the
   *                configured time period.
   *
   * Reasoning for scores from 2 to 9 are to be configured by the builder owner.
   * Since each set of metrics used to calculate the health score can vary, the
   * builder owners must provide the score and reasoning (using the description
   * field). This allows for complicated metric calculation while preserving a
   * binary solution for less complex forms of metric calculation.
   */
  readonly healthScore: string;
  /**
   * A map of metric label to value. This will allow milo to display the metrics
   * used to construct the health score. There is no generic set of metrics for
   * this since each set of metrics can vary from team to team.
   *
   * Buildbucket will not use this information to calculate the health score.
   * These metrics are for display only.
   */
  readonly healthMetrics: { [key: string]: number };
  /**
   * A human readable summary of why the health is the way it is, without
   * the user having to go to the dashboard to find it themselves.
   *
   * E.g.
   *   "the p90 pending time has been greater than 50 minutes for at least 3
   *    of the last 7 days"
   */
  readonly description: string;
  /**
   * Mapping of username domain to clickable link for documentation on the health
   * metrics and how they were calculated.
   *
   * The empty domain value will be used as a fallback for anonymous users, or
   * if the user identity domain doesn't have a matching entry in this map.
   *
   * If linking an internal google link (say g3doc), use a go-link instead of a
   * raw url.
   */
  readonly docLinks: { [key: string]: string };
  /**
   * Mapping of username domain to clickable link for data visualization or
   * dashboards for the health metrics.
   *
   * Similar to doc_links, the empty domain value will be used as a fallback for
   * anonymous users, or if the user identity domain doesn't have a matching
   * entry in this map.
   *
   * If linking an internal google link (say g3doc), use a go-link instead of a
   * raw url.
   */
  readonly dataLinks: { [key: string]: string };
  /**
   * Entity that reported the health status, A luci-auth identity.
   * E.g.
   *    anonymous:anonymous, user:someuser@example.com, project:chromeos
   *
   * Set by Buildbucket. Output only.
   */
  readonly reporter: string;
  /** Set by Buildbucket. Output only. */
  readonly reportedTime:
    | string
    | undefined;
  /**
   * A contact email for the builder's owning team, for the purpose of fixing builder health issues
   * See contact_team_email field in project_config.BuilderConfig
   */
  readonly contactTeamEmail: string;
}

export interface HealthStatus_HealthMetricsEntry {
  readonly key: string;
  readonly value: number;
}

export interface HealthStatus_DocLinksEntry {
  readonly key: string;
  readonly value: string;
}

export interface HealthStatus_DataLinksEntry {
  readonly key: string;
  readonly value: string;
}

function createBaseExecutable(): Executable {
  return { cipdPackage: "", cipdVersion: "", cmd: [], wrapper: [] };
}

export const Executable = {
  encode(message: Executable, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.cipdPackage !== "") {
      writer.uint32(10).string(message.cipdPackage);
    }
    if (message.cipdVersion !== "") {
      writer.uint32(18).string(message.cipdVersion);
    }
    for (const v of message.cmd) {
      writer.uint32(26).string(v!);
    }
    for (const v of message.wrapper) {
      writer.uint32(34).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Executable {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseExecutable() as any;
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

          message.cipdVersion = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.cmd.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.wrapper.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Executable {
    return {
      cipdPackage: isSet(object.cipdPackage) ? globalThis.String(object.cipdPackage) : "",
      cipdVersion: isSet(object.cipdVersion) ? globalThis.String(object.cipdVersion) : "",
      cmd: globalThis.Array.isArray(object?.cmd) ? object.cmd.map((e: any) => globalThis.String(e)) : [],
      wrapper: globalThis.Array.isArray(object?.wrapper) ? object.wrapper.map((e: any) => globalThis.String(e)) : [],
    };
  },

  toJSON(message: Executable): unknown {
    const obj: any = {};
    if (message.cipdPackage !== "") {
      obj.cipdPackage = message.cipdPackage;
    }
    if (message.cipdVersion !== "") {
      obj.cipdVersion = message.cipdVersion;
    }
    if (message.cmd?.length) {
      obj.cmd = message.cmd;
    }
    if (message.wrapper?.length) {
      obj.wrapper = message.wrapper;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Executable>, I>>(base?: I): Executable {
    return Executable.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Executable>, I>>(object: I): Executable {
    const message = createBaseExecutable() as any;
    message.cipdPackage = object.cipdPackage ?? "";
    message.cipdVersion = object.cipdVersion ?? "";
    message.cmd = object.cmd?.map((e) => e) || [];
    message.wrapper = object.wrapper?.map((e) => e) || [];
    return message;
  },
};

function createBaseStatusDetails(): StatusDetails {
  return { resourceExhaustion: undefined, timeout: undefined };
}

export const StatusDetails = {
  encode(message: StatusDetails, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.resourceExhaustion !== undefined) {
      StatusDetails_ResourceExhaustion.encode(message.resourceExhaustion, writer.uint32(26).fork()).ldelim();
    }
    if (message.timeout !== undefined) {
      StatusDetails_Timeout.encode(message.timeout, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StatusDetails {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStatusDetails() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 3:
          if (tag !== 26) {
            break;
          }

          message.resourceExhaustion = StatusDetails_ResourceExhaustion.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.timeout = StatusDetails_Timeout.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): StatusDetails {
    return {
      resourceExhaustion: isSet(object.resourceExhaustion)
        ? StatusDetails_ResourceExhaustion.fromJSON(object.resourceExhaustion)
        : undefined,
      timeout: isSet(object.timeout) ? StatusDetails_Timeout.fromJSON(object.timeout) : undefined,
    };
  },

  toJSON(message: StatusDetails): unknown {
    const obj: any = {};
    if (message.resourceExhaustion !== undefined) {
      obj.resourceExhaustion = StatusDetails_ResourceExhaustion.toJSON(message.resourceExhaustion);
    }
    if (message.timeout !== undefined) {
      obj.timeout = StatusDetails_Timeout.toJSON(message.timeout);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<StatusDetails>, I>>(base?: I): StatusDetails {
    return StatusDetails.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<StatusDetails>, I>>(object: I): StatusDetails {
    const message = createBaseStatusDetails() as any;
    message.resourceExhaustion = (object.resourceExhaustion !== undefined && object.resourceExhaustion !== null)
      ? StatusDetails_ResourceExhaustion.fromPartial(object.resourceExhaustion)
      : undefined;
    message.timeout = (object.timeout !== undefined && object.timeout !== null)
      ? StatusDetails_Timeout.fromPartial(object.timeout)
      : undefined;
    return message;
  },
};

function createBaseStatusDetails_ResourceExhaustion(): StatusDetails_ResourceExhaustion {
  return {};
}

export const StatusDetails_ResourceExhaustion = {
  encode(_: StatusDetails_ResourceExhaustion, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StatusDetails_ResourceExhaustion {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStatusDetails_ResourceExhaustion() as any;
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

  fromJSON(_: any): StatusDetails_ResourceExhaustion {
    return {};
  },

  toJSON(_: StatusDetails_ResourceExhaustion): unknown {
    const obj: any = {};
    return obj;
  },

  create<I extends Exact<DeepPartial<StatusDetails_ResourceExhaustion>, I>>(
    base?: I,
  ): StatusDetails_ResourceExhaustion {
    return StatusDetails_ResourceExhaustion.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<StatusDetails_ResourceExhaustion>, I>>(
    _: I,
  ): StatusDetails_ResourceExhaustion {
    const message = createBaseStatusDetails_ResourceExhaustion() as any;
    return message;
  },
};

function createBaseStatusDetails_Timeout(): StatusDetails_Timeout {
  return {};
}

export const StatusDetails_Timeout = {
  encode(_: StatusDetails_Timeout, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StatusDetails_Timeout {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStatusDetails_Timeout() as any;
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

  fromJSON(_: any): StatusDetails_Timeout {
    return {};
  },

  toJSON(_: StatusDetails_Timeout): unknown {
    const obj: any = {};
    return obj;
  },

  create<I extends Exact<DeepPartial<StatusDetails_Timeout>, I>>(base?: I): StatusDetails_Timeout {
    return StatusDetails_Timeout.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<StatusDetails_Timeout>, I>>(_: I): StatusDetails_Timeout {
    const message = createBaseStatusDetails_Timeout() as any;
    return message;
  },
};

function createBaseLog(): Log {
  return { name: "", viewUrl: "", url: "" };
}

export const Log = {
  encode(message: Log, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.viewUrl !== "") {
      writer.uint32(18).string(message.viewUrl);
    }
    if (message.url !== "") {
      writer.uint32(26).string(message.url);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Log {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseLog() as any;
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

          message.viewUrl = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.url = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Log {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      viewUrl: isSet(object.viewUrl) ? globalThis.String(object.viewUrl) : "",
      url: isSet(object.url) ? globalThis.String(object.url) : "",
    };
  },

  toJSON(message: Log): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.viewUrl !== "") {
      obj.viewUrl = message.viewUrl;
    }
    if (message.url !== "") {
      obj.url = message.url;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Log>, I>>(base?: I): Log {
    return Log.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Log>, I>>(object: I): Log {
    const message = createBaseLog() as any;
    message.name = object.name ?? "";
    message.viewUrl = object.viewUrl ?? "";
    message.url = object.url ?? "";
    return message;
  },
};

function createBaseGerritChange(): GerritChange {
  return { host: "", project: "", change: "0", patchset: "0" };
}

export const GerritChange = {
  encode(message: GerritChange, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.host !== "") {
      writer.uint32(10).string(message.host);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.change !== "0") {
      writer.uint32(24).int64(message.change);
    }
    if (message.patchset !== "0") {
      writer.uint32(32).int64(message.patchset);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GerritChange {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGerritChange() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.host = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.project = reader.string();
          continue;
        case 3:
          if (tag !== 24) {
            break;
          }

          message.change = longToString(reader.int64() as Long);
          continue;
        case 4:
          if (tag !== 32) {
            break;
          }

          message.patchset = longToString(reader.int64() as Long);
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GerritChange {
    return {
      host: isSet(object.host) ? globalThis.String(object.host) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      change: isSet(object.change) ? globalThis.String(object.change) : "0",
      patchset: isSet(object.patchset) ? globalThis.String(object.patchset) : "0",
    };
  },

  toJSON(message: GerritChange): unknown {
    const obj: any = {};
    if (message.host !== "") {
      obj.host = message.host;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.change !== "0") {
      obj.change = message.change;
    }
    if (message.patchset !== "0") {
      obj.patchset = message.patchset;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GerritChange>, I>>(base?: I): GerritChange {
    return GerritChange.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GerritChange>, I>>(object: I): GerritChange {
    const message = createBaseGerritChange() as any;
    message.host = object.host ?? "";
    message.project = object.project ?? "";
    message.change = object.change ?? "0";
    message.patchset = object.patchset ?? "0";
    return message;
  },
};

function createBaseGitilesCommit(): GitilesCommit {
  return { host: "", project: "", id: "", ref: "", position: 0 };
}

export const GitilesCommit = {
  encode(message: GitilesCommit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.host !== "") {
      writer.uint32(10).string(message.host);
    }
    if (message.project !== "") {
      writer.uint32(18).string(message.project);
    }
    if (message.id !== "") {
      writer.uint32(26).string(message.id);
    }
    if (message.ref !== "") {
      writer.uint32(34).string(message.ref);
    }
    if (message.position !== 0) {
      writer.uint32(40).uint32(message.position);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): GitilesCommit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseGitilesCommit() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.host = reader.string();
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

          message.id = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.ref = reader.string();
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.position = reader.uint32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): GitilesCommit {
    return {
      host: isSet(object.host) ? globalThis.String(object.host) : "",
      project: isSet(object.project) ? globalThis.String(object.project) : "",
      id: isSet(object.id) ? globalThis.String(object.id) : "",
      ref: isSet(object.ref) ? globalThis.String(object.ref) : "",
      position: isSet(object.position) ? globalThis.Number(object.position) : 0,
    };
  },

  toJSON(message: GitilesCommit): unknown {
    const obj: any = {};
    if (message.host !== "") {
      obj.host = message.host;
    }
    if (message.project !== "") {
      obj.project = message.project;
    }
    if (message.id !== "") {
      obj.id = message.id;
    }
    if (message.ref !== "") {
      obj.ref = message.ref;
    }
    if (message.position !== 0) {
      obj.position = Math.round(message.position);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<GitilesCommit>, I>>(base?: I): GitilesCommit {
    return GitilesCommit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<GitilesCommit>, I>>(object: I): GitilesCommit {
    const message = createBaseGitilesCommit() as any;
    message.host = object.host ?? "";
    message.project = object.project ?? "";
    message.id = object.id ?? "";
    message.ref = object.ref ?? "";
    message.position = object.position ?? 0;
    return message;
  },
};

function createBaseStringPair(): StringPair {
  return { key: "", value: "" };
}

export const StringPair = {
  encode(message: StringPair, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): StringPair {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStringPair() as any;
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

  fromJSON(object: any): StringPair {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: StringPair): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<StringPair>, I>>(base?: I): StringPair {
    return StringPair.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<StringPair>, I>>(object: I): StringPair {
    const message = createBaseStringPair() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

function createBaseTimeRange(): TimeRange {
  return { startTime: undefined, endTime: undefined };
}

export const TimeRange = {
  encode(message: TimeRange, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(10).fork()).ldelim();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TimeRange {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTimeRange() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 2:
          if (tag !== 18) {
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

  fromJSON(object: any): TimeRange {
    return {
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
    };
  },

  toJSON(message: TimeRange): unknown {
    const obj: any = {};
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TimeRange>, I>>(base?: I): TimeRange {
    return TimeRange.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TimeRange>, I>>(object: I): TimeRange {
    const message = createBaseTimeRange() as any;
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    return message;
  },
};

function createBaseRequestedDimension(): RequestedDimension {
  return { key: "", value: "", expiration: undefined };
}

export const RequestedDimension = {
  encode(message: RequestedDimension, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    if (message.expiration !== undefined) {
      Duration.encode(message.expiration, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RequestedDimension {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRequestedDimension() as any;
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
        case 3:
          if (tag !== 26) {
            break;
          }

          message.expiration = Duration.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): RequestedDimension {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
      expiration: isSet(object.expiration) ? Duration.fromJSON(object.expiration) : undefined,
    };
  },

  toJSON(message: RequestedDimension): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    if (message.expiration !== undefined) {
      obj.expiration = Duration.toJSON(message.expiration);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<RequestedDimension>, I>>(base?: I): RequestedDimension {
    return RequestedDimension.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<RequestedDimension>, I>>(object: I): RequestedDimension {
    const message = createBaseRequestedDimension() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    message.expiration = (object.expiration !== undefined && object.expiration !== null)
      ? Duration.fromPartial(object.expiration)
      : undefined;
    return message;
  },
};

function createBaseCacheEntry(): CacheEntry {
  return { name: "", path: "", waitForWarmCache: undefined, envVar: "" };
}

export const CacheEntry = {
  encode(message: CacheEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
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

  decode(input: _m0.Reader | Uint8Array, length?: number): CacheEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseCacheEntry() as any;
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

  fromJSON(object: any): CacheEntry {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      path: isSet(object.path) ? globalThis.String(object.path) : "",
      waitForWarmCache: isSet(object.waitForWarmCache) ? Duration.fromJSON(object.waitForWarmCache) : undefined,
      envVar: isSet(object.envVar) ? globalThis.String(object.envVar) : "",
    };
  },

  toJSON(message: CacheEntry): unknown {
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

  create<I extends Exact<DeepPartial<CacheEntry>, I>>(base?: I): CacheEntry {
    return CacheEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<CacheEntry>, I>>(object: I): CacheEntry {
    const message = createBaseCacheEntry() as any;
    message.name = object.name ?? "";
    message.path = object.path ?? "";
    message.waitForWarmCache = (object.waitForWarmCache !== undefined && object.waitForWarmCache !== null)
      ? Duration.fromPartial(object.waitForWarmCache)
      : undefined;
    message.envVar = object.envVar ?? "";
    return message;
  },
};

function createBaseHealthStatus(): HealthStatus {
  return {
    healthScore: "0",
    healthMetrics: {},
    description: "",
    docLinks: {},
    dataLinks: {},
    reporter: "",
    reportedTime: undefined,
    contactTeamEmail: "",
  };
}

export const HealthStatus = {
  encode(message: HealthStatus, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.healthScore !== "0") {
      writer.uint32(8).int64(message.healthScore);
    }
    Object.entries(message.healthMetrics).forEach(([key, value]) => {
      HealthStatus_HealthMetricsEntry.encode({ key: key as any, value }, writer.uint32(18).fork()).ldelim();
    });
    if (message.description !== "") {
      writer.uint32(26).string(message.description);
    }
    Object.entries(message.docLinks).forEach(([key, value]) => {
      HealthStatus_DocLinksEntry.encode({ key: key as any, value }, writer.uint32(34).fork()).ldelim();
    });
    Object.entries(message.dataLinks).forEach(([key, value]) => {
      HealthStatus_DataLinksEntry.encode({ key: key as any, value }, writer.uint32(42).fork()).ldelim();
    });
    if (message.reporter !== "") {
      writer.uint32(50).string(message.reporter);
    }
    if (message.reportedTime !== undefined) {
      Timestamp.encode(toTimestamp(message.reportedTime), writer.uint32(58).fork()).ldelim();
    }
    if (message.contactTeamEmail !== "") {
      writer.uint32(66).string(message.contactTeamEmail);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HealthStatus {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHealthStatus() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.healthScore = longToString(reader.int64() as Long);
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          const entry2 = HealthStatus_HealthMetricsEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.healthMetrics[entry2.key] = entry2.value;
          }
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.description = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          const entry4 = HealthStatus_DocLinksEntry.decode(reader, reader.uint32());
          if (entry4.value !== undefined) {
            message.docLinks[entry4.key] = entry4.value;
          }
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          const entry5 = HealthStatus_DataLinksEntry.decode(reader, reader.uint32());
          if (entry5.value !== undefined) {
            message.dataLinks[entry5.key] = entry5.value;
          }
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.reporter = reader.string();
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.reportedTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        case 8:
          if (tag !== 66) {
            break;
          }

          message.contactTeamEmail = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): HealthStatus {
    return {
      healthScore: isSet(object.healthScore) ? globalThis.String(object.healthScore) : "0",
      healthMetrics: isObject(object.healthMetrics)
        ? Object.entries(object.healthMetrics).reduce<{ [key: string]: number }>((acc, [key, value]) => {
          acc[key] = Number(value);
          return acc;
        }, {})
        : {},
      description: isSet(object.description) ? globalThis.String(object.description) : "",
      docLinks: isObject(object.docLinks)
        ? Object.entries(object.docLinks).reduce<{ [key: string]: string }>((acc, [key, value]) => {
          acc[key] = String(value);
          return acc;
        }, {})
        : {},
      dataLinks: isObject(object.dataLinks)
        ? Object.entries(object.dataLinks).reduce<{ [key: string]: string }>((acc, [key, value]) => {
          acc[key] = String(value);
          return acc;
        }, {})
        : {},
      reporter: isSet(object.reporter) ? globalThis.String(object.reporter) : "",
      reportedTime: isSet(object.reportedTime) ? globalThis.String(object.reportedTime) : undefined,
      contactTeamEmail: isSet(object.contactTeamEmail) ? globalThis.String(object.contactTeamEmail) : "",
    };
  },

  toJSON(message: HealthStatus): unknown {
    const obj: any = {};
    if (message.healthScore !== "0") {
      obj.healthScore = message.healthScore;
    }
    if (message.healthMetrics) {
      const entries = Object.entries(message.healthMetrics);
      if (entries.length > 0) {
        obj.healthMetrics = {};
        entries.forEach(([k, v]) => {
          obj.healthMetrics[k] = v;
        });
      }
    }
    if (message.description !== "") {
      obj.description = message.description;
    }
    if (message.docLinks) {
      const entries = Object.entries(message.docLinks);
      if (entries.length > 0) {
        obj.docLinks = {};
        entries.forEach(([k, v]) => {
          obj.docLinks[k] = v;
        });
      }
    }
    if (message.dataLinks) {
      const entries = Object.entries(message.dataLinks);
      if (entries.length > 0) {
        obj.dataLinks = {};
        entries.forEach(([k, v]) => {
          obj.dataLinks[k] = v;
        });
      }
    }
    if (message.reporter !== "") {
      obj.reporter = message.reporter;
    }
    if (message.reportedTime !== undefined) {
      obj.reportedTime = message.reportedTime;
    }
    if (message.contactTeamEmail !== "") {
      obj.contactTeamEmail = message.contactTeamEmail;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<HealthStatus>, I>>(base?: I): HealthStatus {
    return HealthStatus.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<HealthStatus>, I>>(object: I): HealthStatus {
    const message = createBaseHealthStatus() as any;
    message.healthScore = object.healthScore ?? "0";
    message.healthMetrics = Object.entries(object.healthMetrics ?? {}).reduce<{ [key: string]: number }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = globalThis.Number(value);
        }
        return acc;
      },
      {},
    );
    message.description = object.description ?? "";
    message.docLinks = Object.entries(object.docLinks ?? {}).reduce<{ [key: string]: string }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = globalThis.String(value);
      }
      return acc;
    }, {});
    message.dataLinks = Object.entries(object.dataLinks ?? {}).reduce<{ [key: string]: string }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = globalThis.String(value);
        }
        return acc;
      },
      {},
    );
    message.reporter = object.reporter ?? "";
    message.reportedTime = object.reportedTime ?? undefined;
    message.contactTeamEmail = object.contactTeamEmail ?? "";
    return message;
  },
};

function createBaseHealthStatus_HealthMetricsEntry(): HealthStatus_HealthMetricsEntry {
  return { key: "", value: 0 };
}

export const HealthStatus_HealthMetricsEntry = {
  encode(message: HealthStatus_HealthMetricsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== 0) {
      writer.uint32(21).float(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HealthStatus_HealthMetricsEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHealthStatus_HealthMetricsEntry() as any;
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
          if (tag !== 21) {
            break;
          }

          message.value = reader.float();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): HealthStatus_HealthMetricsEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.Number(object.value) : 0,
    };
  },

  toJSON(message: HealthStatus_HealthMetricsEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== 0) {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<HealthStatus_HealthMetricsEntry>, I>>(base?: I): HealthStatus_HealthMetricsEntry {
    return HealthStatus_HealthMetricsEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<HealthStatus_HealthMetricsEntry>, I>>(
    object: I,
  ): HealthStatus_HealthMetricsEntry {
    const message = createBaseHealthStatus_HealthMetricsEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? 0;
    return message;
  },
};

function createBaseHealthStatus_DocLinksEntry(): HealthStatus_DocLinksEntry {
  return { key: "", value: "" };
}

export const HealthStatus_DocLinksEntry = {
  encode(message: HealthStatus_DocLinksEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HealthStatus_DocLinksEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHealthStatus_DocLinksEntry() as any;
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

  fromJSON(object: any): HealthStatus_DocLinksEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: HealthStatus_DocLinksEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<HealthStatus_DocLinksEntry>, I>>(base?: I): HealthStatus_DocLinksEntry {
    return HealthStatus_DocLinksEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<HealthStatus_DocLinksEntry>, I>>(object: I): HealthStatus_DocLinksEntry {
    const message = createBaseHealthStatus_DocLinksEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

function createBaseHealthStatus_DataLinksEntry(): HealthStatus_DataLinksEntry {
  return { key: "", value: "" };
}

export const HealthStatus_DataLinksEntry = {
  encode(message: HealthStatus_DataLinksEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HealthStatus_DataLinksEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseHealthStatus_DataLinksEntry() as any;
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

  fromJSON(object: any): HealthStatus_DataLinksEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: HealthStatus_DataLinksEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<HealthStatus_DataLinksEntry>, I>>(base?: I): HealthStatus_DataLinksEntry {
    return HealthStatus_DataLinksEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<HealthStatus_DataLinksEntry>, I>>(object: I): HealthStatus_DataLinksEntry {
    const message = createBaseHealthStatus_DataLinksEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
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
