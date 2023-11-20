/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { Duration } from "../../../../google/protobuf/duration.pb";
import { UInt32Value } from "../../../../google/protobuf/wrappers.pb";
import { BigQueryExport, HistoryOptions } from "../../resultdb/proto/v1/invocation.pb";
import {
  Compression,
  compressionFromJSON,
  compressionToJSON,
  Executable,
  Trinary,
  trinaryFromJSON,
  trinaryToJSON,
} from "./common.pb";

export const protobufPackage = "buildbucket";

/**
 * Toggle is a boolean with an extra state UNSET.
 * When protobuf messages are merged, UNSET does not overwrite an existing
 * value.
 * TODO(nodir): replace with Trinary in ../common.proto.
 */
export enum Toggle {
  UNSET = 0,
  YES = 1,
  NO = 2,
}

export function toggleFromJSON(object: any): Toggle {
  switch (object) {
    case 0:
    case "UNSET":
      return Toggle.UNSET;
    case 1:
    case "YES":
      return Toggle.YES;
    case 2:
    case "NO":
      return Toggle.NO;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Toggle");
  }
}

export function toggleToJSON(object: Toggle): string {
  switch (object) {
    case Toggle.UNSET:
      return "UNSET";
    case Toggle.YES:
      return "YES";
    case Toggle.NO:
      return "NO";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Toggle");
  }
}

/**
 * Deprecated in favor of LUCI Realms. This proto is totally unused now, exists
 * only to not break older configs that still may have deprecated fields
 * populated.
 */
export interface Acl {
  /** @deprecated */
  readonly role: Acl_Role;
  /** @deprecated */
  readonly group: string;
  /** @deprecated */
  readonly identity: string;
}

export enum Acl_Role {
  READER = 0,
  SCHEDULER = 1,
  WRITER = 2,
}

export function acl_RoleFromJSON(object: any): Acl_Role {
  switch (object) {
    case 0:
    case "READER":
      return Acl_Role.READER;
    case 1:
    case "SCHEDULER":
      return Acl_Role.SCHEDULER;
    case 2:
    case "WRITER":
      return Acl_Role.WRITER;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Acl_Role");
  }
}

export function acl_RoleToJSON(object: Acl_Role): string {
  switch (object) {
    case Acl_Role.READER:
      return "READER";
    case Acl_Role.SCHEDULER:
      return "SCHEDULER";
    case Acl_Role.WRITER:
      return "WRITER";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum Acl_Role");
  }
}

/**
 * Defines a swarmbucket builder. A builder has a name, a category and specifies
 * what should happen if a build is scheduled to that builder.
 *
 * SECURITY WARNING: if adding more fields to this message, keep in mind that
 * a user that has permissions to schedule a build to the bucket, can override
 * this config.
 *
 * Next tag: 38.
 */
export interface BuilderConfig {
  /**
   * Name of the builder.
   *
   * If a builder name, will be propagated to "builder" build tag and
   * "buildername" recipe property.
   *
   * A builder name must be unique within the bucket, and match regex
   * ^[a-zA-Z0-9\-_.\(\) ]{1,128}$.
   */
  readonly name: string;
  /**
   * Backend for this builder.
   * If unset, builds are scheduled using the legacy pipeline.
   */
  readonly backend:
    | BuilderConfig_Backend
    | undefined;
  /**
   * Alternate backend to use for this builder when the
   * "luci.buildbucket.backend_alt" experiment is enabled. Works even when
   * `backend` is empty. Useful for migrations to new backends.
   */
  readonly backendAlt:
    | BuilderConfig_Backend
    | undefined;
  /**
   * Hostname of the swarming instance, e.g. "chromium-swarm.appspot.com".
   * Required, but defaults to deprecated Swarming.hostname.
   */
  readonly swarmingHost: string;
  /** Builder category. Will be used for visual grouping, for example in Code Review. */
  readonly category: string;
  /**
   * DEPRECATED.
   * Used only to enable "vpython:native-python-wrapper"
   * Does NOT actually propagate to swarming.
   */
  readonly swarmingTags: readonly string[];
  /**
   * A requirement for a bot to execute the build.
   *
   * Supports 2 forms:
   * - "<key>:<value>" - require a bot with this dimension.
   *   This is a shortcut for "0:<key>:<value>", see below.
   * - "<expiration_secs>:<key>:<value>" - wait for up to expiration_secs.
   *   for a bot with the dimension.
   *   Supports multiple values for different keys and expiration_secs.
   *   expiration_secs must be a multiple of 60.
   *
   * If this builder is defined in a bucket, dimension "pool" is defaulted
   * to the name of the bucket. See Bucket message below.
   */
  readonly dimensions: readonly string[];
  /**
   * Specifies that a recipe to run.
   * DEPRECATED: use exe.
   */
  readonly recipe:
    | BuilderConfig_Recipe
    | undefined;
  /** What to run when a build is ready to start. */
  readonly exe:
    | Executable
    | undefined;
  /**
   * A JSON object representing Build.input.properties.
   * Individual object properties can be overridden with
   * ScheduleBuildRequest.properties.
   */
  readonly properties: string;
  /**
   * A list of top-level property names which can be overridden in
   * ScheduleBuildRequest.
   *
   * If this field is the EXACT value `["*"]` then all properties are permitted
   * to be overridden.
   *
   * NOTE: Some executables (such as the recipe engine) can have drastic
   * behavior differences based on some properties (for example, the "recipe"
   * property). If you allow the "recipe" property to be overridden, then anyone
   * with the 'buildbucket.builds.add' permission could create a Build for this
   * Builder running a different recipe (from the same recipe repo).
   */
  readonly allowedPropertyOverrides: readonly string[];
  /**
   * Swarming task priority.
   * A value between 20 and 255, inclusive.
   * Lower means more important.
   *
   * The default value is configured in
   * https://chrome-internal.googlesource.com/infradata/config/+/master/configs/cr-buildbucket/swarming_task_template.json
   *
   * See also https://chromium.googlesource.com/infra/luci/luci-py.git/+/master/appengine/swarming/doc/User-Guide.md#request
   */
  readonly priority: number;
  /**
   * Maximum build execution time.
   *
   * Not to be confused with pending time.
   *
   * If the timeout is reached, the task will be signaled according to the
   * `deadline` section of
   * https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md
   * and status_details.timeout is set.
   *
   * The task will have `grace_period` amount of time to handle cleanup
   * before being forcefully terminated.
   *
   * NOTE: This corresponds with Build.execution_timeout and
   * ScheduleBuildRequest.execution_timeout; The name `execution_timeout_secs` and
   * uint32 type are relics of the past.
   */
  readonly executionTimeoutSecs: number;
  /**
   * Maximum build pending time.
   *
   * If the timeout is reached, the build is marked as INFRA_FAILURE status
   * and both status_details.{timeout, resource_exhaustion} are set.
   *
   * NOTE: This corresponds with Build.scheduling_timeout and
   * ScheduleBuildRequest.scheduling_timeout; The name `expiration_secs` and
   * uint32 type are relics of the past.
   */
  readonly expirationSecs: number;
  /**
   * Amount of cleanup time after execution_timeout_secs.
   *
   * After being signaled according to execution_timeout_secs, the task will
   * have this many seconds to clean up before being forcefully terminated.
   *
   * The signalling process is explained in the `deadline` section of
   * https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md.
   *
   * Defaults to 30s if unspecified or 0.
   */
  readonly gracePeriod:
    | Duration
    | undefined;
  /**
   * If YES, will request that swarming wait until it sees at least one bot
   * report a superset of the requested dimensions.
   *
   * If UNSET/NO (the default), swarming will immediately reject a build which
   * specifies a dimension set that it's never seen before.
   *
   * Usually you want this to be UNSET/NO, unless you know that some external
   * system is working to add bots to swarming which will match the requested
   * dimensions within expiration_secs. Otherwise you'll have to wait for all of
   * `expiration_secs` until swarming tells you "Sorry, nothing has dimension
   * `os:MadeUpOS`".
   */
  readonly waitForCapacity: Trinary;
  /** Caches that should be present on the bot. */
  readonly caches: readonly BuilderConfig_CacheEntry[];
  /**
   * If YES, generate monotonically increasing contiguous numbers for each
   * build, unique within the builder.
   * Note: this limits the build creation rate in this builder to 5 per second.
   */
  readonly buildNumbers: Toggle;
  /**
   * Email of a service account to run the build as or literal 'bot' string to
   * use Swarming bot's account (if available). Passed directly to Swarming.
   * Subject to Swarming's ACLs.
   */
  readonly serviceAccount: string;
  /**
   * If YES, each builder will get extra dimension "builder:<builder name>"
   * added. Default is UNSET.
   *
   * For example, this config
   *
   *   builder {
   *     name: "linux-compiler"
   *     dimension: "builder:linux-compiler"
   *   }
   *
   * is equivalent to this:
   *
   *   builders {
   *     name: "linux-compiler"
   *     auto_builder_dimension: YES
   *   }
   *
   * (see also http://docs.buildbot.net/0.8.9/manual/cfg-properties.html#interpolate)
   * but are currently against complicating config with this.
   */
  readonly autoBuilderDimension: Toggle;
  /**
   * DEPRECATED
   *
   * Set the "luci.non_production" experiment in the 'experiments' field below,
   * instead.
   *
   * If YES, sets the "luci.non_production" experiment to 100% for
   * builds on this builder.
   *
   * See the documentation on `experiments` for more details about the
   * "luci.non_production" experiment.
   */
  readonly experimental: Toggle;
  /**
   * DEPRECATED
   *
   * Set the "luci.buildbucket.canary_software" experiment in the 'experiments'
   * field below, instead.
   *
   * Percentage of builds that should use a canary swarming task template.
   * A value from 0 to 100.
   * If omitted, a global server-defined default percentage is used.
   */
  readonly taskTemplateCanaryPercentage:
    | number
    | undefined;
  /**
   * A mapping of experiment name to the percentage chance (0-100) that it will
   * apply to builds generated from this builder. Experiments are simply strings
   * which various parts of the stack (from LUCI services down to your build
   * scripts) may react to in order to enable certain functionality.
   *
   * You may set any experiments you like, but experiments beginning with
   * "luci." are reserved. Experiment names must conform to
   *
   *    [a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*
   *
   * Any experiments which are selected for a build show up in
   * `Build.input.experiments`.
   *
   * Its recommended that you confine your experiments to smaller, more explicit
   * targets. For example, prefer the experiment named
   * "my_project.use_mysoftware_v2_crbug_999999" rather than "use_next_gen".
   *
   * It is NOT recommended to 'piggy-back' on top of existing experiment names
   * for a different purpose. However if you want to, you can have your build
   * treat the presence of ANY experiment as equivalent to "luci.non_production"
   * being set for your build (i.e. "if any experiment is set, don't affect
   * production"). This is ulimately up to you, however.
   *
   * Well-known experiments
   *
   * Buildbucket has a number of 'global' experiments which are in various
   * states of deployment at any given time. For the current state, see
   * go/buildbucket-settings.cfg.
   */
  readonly experiments: { [key: string]: number };
  /**
   * This field will set the default value of the "critical" field of
   * all the builds of this builder. Please refer to build.proto for
   * the meaning of this field.
   *
   * This value can be overridden by ScheduleBuildRequest.critical
   */
  readonly critical: Trinary;
  /** Used to enable and configure ResultDB integration. */
  readonly resultdb:
    | BuilderConfig_ResultDB
    | undefined;
  /**
   * Description that helps users understand the purpose of the builder, in
   * HTML.
   */
  readonly descriptionHtml: string;
  readonly shadowBuilderAdjustments:
    | BuilderConfig_ShadowBuilderAdjustments
    | undefined;
  /**
   * This field will set the default value of the "retriable" field of
   * all the builds of this builder. Please refer to build.proto for
   * the meaning of this field.
   *
   * This value can be overridden by ScheduleBuildRequest.retriable
   */
  readonly retriable: Trinary;
  readonly builderHealthMetricsLinks:
    | BuilderConfig_BuilderHealthLinks
    | undefined;
  /**
   * The owning team's contact email. This team is responsible for fixing
   * any builder health issues (see Builder.Metadata.HealthSpec).
   * Will be validated as an email address, but nothing else.
   * It will display on milo and could be public facing, so please don't put anything sensitive.
   */
  readonly contactTeamEmail: string;
}

/**
 * Describes a cache directory persisted on a bot.
 * Prerequisite reading in BuildInfra.Swarming.CacheEntry message in
 * build.proto.
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
export interface BuilderConfig_CacheEntry {
  /**
   * Identifier of the cache. Length is limited to 128.
   * Defaults to path.
   * See also BuildInfra.Swarming.CacheEntry.name in build.proto.
   */
  readonly name: string;
  /**
   * Relative path where the cache in mapped into. Required.
   * See also BuildInfra.Swarming.CacheEntry.path in build.proto.
   */
  readonly path: string;
  /**
   * Number of seconds to wait for a bot with a warm cache to pick up the
   * task, before falling back to a bot with a cold (non-existent) cache.
   * See also BuildInfra.Swarming.CacheEntry.wait_for_warm_cache in build.proto.
   * The value must be multiples of 60 seconds.
   */
  readonly waitForWarmCacheSecs: number;
  /**
   * Environment variable with this name will be set to the path to the cache
   * directory.
   */
  readonly envVar: string;
}

/**
 * DEPRECATED. See BuilderConfig.executable and BuilderConfig.properties
 *
 * To specify a recipe name, pass "$recipe_engine" property which is a JSON
 * object having "recipe" property.
 */
export interface BuilderConfig_Recipe {
  /** Name of the recipe to run. */
  readonly name: string;
  /**
   * The CIPD package to fetch the recipes from.
   *
   * Typically the package will look like:
   *
   *   infra/recipe_bundles/chromium.googlesource.com/chromium/tools/build
   *
   * Recipes bundled from internal repositories are typically under
   * `infra_internal/recipe_bundles/...`.
   *
   * But if you're building your own recipe bundles, they could be located
   * elsewhere.
   */
  readonly cipdPackage: string;
  /**
   * The CIPD version to fetch. This can be a lower-cased git ref (like
   * `refs/heads/main` or `head`), or it can be a cipd tag (like
   * `git_revision:dead...beef`).
   *
   * The default is `head`, which corresponds to the git repo's HEAD ref. This
   * is typically (but not always) a symbolic ref for `refs/heads/master`.
   */
  readonly cipdVersion: string;
  /**
   * Colon-separated build properties to set.
   * Ignored if BuilderConfig.properties is set.
   *
   * Use this field for string properties and use properties_j for other
   * types.
   */
  readonly properties: readonly string[];
  /**
   * Same as properties, but the value must valid JSON. For example
   *   properties_j: "a:1"
   * means property a is a number 1, not string "1".
   *
   * If null, it means no property must be defined. In particular, it removes
   * a default value for the property, if any.
   *
   * Fields properties and properties_j can be used together, but cannot both
   * specify values for same property.
   */
  readonly propertiesJ: readonly string[];
}

/** ResultDB-specific information for a builder. */
export interface BuilderConfig_ResultDB {
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

/** Buildbucket backend-specific information for a builder. */
export interface BuilderConfig_Backend {
  /** URI for this backend, e.g. "swarming://chromium-swarm". */
  readonly target: string;
  /**
   * A string interpreted as JSON encapsulating configuration for this
   * backend.
   * TODO(crbug.com/1042991): Move priority, wait_for_capacity, etc. here.
   */
  readonly configJson: string;
}

export interface BuilderConfig_ExperimentsEntry {
  readonly key: string;
  readonly value: number;
}

/**
 * Configurations that need to be replaced when running a led build for this
 * Builder.
 */
export interface BuilderConfig_ShadowBuilderAdjustments {
  readonly serviceAccount: string;
  readonly pool: string;
  /**
   * A JSON object contains properties to override Build.input.properties
   * when creating the led build.
   * Same as ScheduleBuild, the top-level properties here will override the
   * ones in builder config, instead of deep merge.
   */
  readonly properties: string;
  /**
   * Overrides default dimensions defined by builder config.
   * Same as ScheduleBuild,
   * * dimensions with empty value will be excluded.
   * * same key dimensions with both empty and non-empty values are disallowed.
   *
   * Note: for historical reason, pool can be adjusted individually.
   * If pool is adjusted individually, the same change should be reflected in
   * dimensions, and vice versa.
   */
  readonly dimensions: readonly string[];
}

export interface BuilderConfig_BuilderHealthLinks {
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
}

export interface BuilderConfig_BuilderHealthLinks_DocLinksEntry {
  readonly key: string;
  readonly value: string;
}

export interface BuilderConfig_BuilderHealthLinks_DataLinksEntry {
  readonly key: string;
  readonly value: string;
}

/** Configuration of buildbucket-swarming integration for one bucket. */
export interface Swarming {
  /**
   * Configuration for each builder.
   * Swarming tasks are created only for builds for builders that are not
   * explicitly specified.
   */
  readonly builders: readonly BuilderConfig[];
  /**
   * DEPRECATED. Use builder_defaults.task_template_canary_percentage instead.
   * Setting this field sets builder_defaults.task_template_canary_percentage.
   */
  readonly taskTemplateCanaryPercentage: number | undefined;
}

/** Defines one bucket in buildbucket.cfg */
export interface Bucket {
  /**
   * Name of the bucket. Names are unique within one instance of buildbucket.
   * If another project already uses this name, a config will be rejected.
   * Name reservation is first-come first-serve.
   * Regex: ^[a-z0-9\-_.]{1,100}$
   */
  readonly name: string;
  /**
   * Deprecated and ignored. Use Realms ACLs instead.
   *
   * @deprecated
   */
  readonly acls: readonly Acl[];
  /**
   * Buildbucket-swarming integration.
   * Mutually exclusive with builder_template.
   */
  readonly swarming:
    | Swarming
    | undefined;
  /**
   * Name of this bucket's shadow bucket for the led builds to use.
   *
   * If omitted, it implies that led builds of this bucket reuse this bucket.
   * This is allowed, but note that it means the led builds will be in
   * the same bucket/builder with the real builds, which means Any users with
   * led access will be able to do ANYTHING that this bucket's bots and
   * service_accounts can do.
   *
   * It could also be noisy, such as:
   * * On the LUCI UI, led builds will show under the same builder as the real builds,
   * * Led builds will share the same ResultDB config as the real builds, so
   *   their test results will be exported to the same BigQuery tables.
   * * Subscribers of Buildbucket PubSub need to filter them out.
   */
  readonly shadow: string;
  /**
   * Security constraints of the bucket.
   *
   * This field is used by CreateBuild on this bucket to constrain proposed
   * Builds. If a build doesn't meet the constraints, it will be rejected.
   * For shadow buckets, this is what prevents the bucket from allowing
   * totally arbitrary Builds.
   *
   * `lucicfg` will automatically populate this for the "primary" bucket
   * when using `luci.builder`.
   *
   * Buildbuceket.CreateBuild will validate the incoming requests to make sure
   * they meet these constraints.
   */
  readonly constraints:
    | Bucket_Constraints
    | undefined;
  /**
   * Template of builders in a dynamic bucket.
   * Mutually exclusive with swarming.
   *
   * If is not nil, the bucket is a dynamic LUCI bucket.
   * If a bucket has both swarming and dynamic_builder_template as nil,
   * the bucket is a legacy one.
   */
  readonly dynamicBuilderTemplate: Bucket_DynamicBuilderTemplate | undefined;
}

/**
 * Constraints for a bucket.
 *
 * Buildbucket.CreateBuild will validate the incoming requests to make sure
 * they meet these constraints.
 */
export interface Bucket_Constraints {
  /**
   * Constraints allowed pools.
   * Builds in this bucket must have a "pool" dimension which matches an entry in this list.
   */
  readonly pools: readonly string[];
  /** Only service accounts in this list are allowed. */
  readonly serviceAccounts: readonly string[];
}

/** Template of builders in a dynamic bucket. */
export interface Bucket_DynamicBuilderTemplate {
}

/** Schema of buildbucket.cfg file, a project config. */
export interface BuildbucketCfg {
  /** All buckets defined for this project. */
  readonly buckets: readonly Bucket[];
  /**
   * Global configs are shared among all buckets and builders defined inside
   * this project.
   */
  readonly commonConfig: BuildbucketCfg_CommonConfig | undefined;
}

export interface BuildbucketCfg_Topic {
  /**
   * Topic name format should be like
   * "projects/<projid>/topics/<topicid>" and conforms to the guideline:
   * https://cloud.google.com/pubsub/docs/admin#resource_names.
   */
  readonly name: string;
  /**
   * The compression method that  `build_large_fields` uses in pubsub message
   * data. By default, it's ZLIB as this is the most common one and is the
   * built-in lib in many programming languages.
   */
  readonly compression: Compression;
}

export interface BuildbucketCfg_CommonConfig {
  /**
   * A list of PubSub topics that Buildbucket will publish notifications for
   * build status changes in this project.
   * The message data schema can be found in message `BuildsV2PubSub` in
   * https://chromium.googlesource.com/infra/luci/luci-go/+/main/buildbucket/proto/notification.proto
   * Attributes on the pubsub messages:
   * - "project"
   * - "bucket"
   * - "builder"
   * - "is_completed" (The value is either "true" or "false" in string.)
   *
   * Note: `pubsub.topics.publish` permission must be granted to the
   * corresponding luci-project-scoped accounts in the cloud project(s) hosting
   * the topics.
   */
  readonly buildsNotificationTopics: readonly BuildbucketCfg_Topic[];
}

function createBaseAcl(): Acl {
  return { role: 0, group: "", identity: "" };
}

export const Acl = {
  encode(message: Acl, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.role !== 0) {
      writer.uint32(8).int32(message.role);
    }
    if (message.group !== "") {
      writer.uint32(18).string(message.group);
    }
    if (message.identity !== "") {
      writer.uint32(26).string(message.identity);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Acl {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseAcl() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.role = reader.int32() as any;
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.group = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.identity = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Acl {
    return {
      role: isSet(object.role) ? acl_RoleFromJSON(object.role) : 0,
      group: isSet(object.group) ? globalThis.String(object.group) : "",
      identity: isSet(object.identity) ? globalThis.String(object.identity) : "",
    };
  },

  toJSON(message: Acl): unknown {
    const obj: any = {};
    if (message.role !== 0) {
      obj.role = acl_RoleToJSON(message.role);
    }
    if (message.group !== "") {
      obj.group = message.group;
    }
    if (message.identity !== "") {
      obj.identity = message.identity;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Acl>, I>>(base?: I): Acl {
    return Acl.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Acl>, I>>(object: I): Acl {
    const message = createBaseAcl() as any;
    message.role = object.role ?? 0;
    message.group = object.group ?? "";
    message.identity = object.identity ?? "";
    return message;
  },
};

function createBaseBuilderConfig(): BuilderConfig {
  return {
    name: "",
    backend: undefined,
    backendAlt: undefined,
    swarmingHost: "",
    category: "",
    swarmingTags: [],
    dimensions: [],
    recipe: undefined,
    exe: undefined,
    properties: "",
    allowedPropertyOverrides: [],
    priority: 0,
    executionTimeoutSecs: 0,
    expirationSecs: 0,
    gracePeriod: undefined,
    waitForCapacity: 0,
    caches: [],
    buildNumbers: 0,
    serviceAccount: "",
    autoBuilderDimension: 0,
    experimental: 0,
    taskTemplateCanaryPercentage: undefined,
    experiments: {},
    critical: 0,
    resultdb: undefined,
    descriptionHtml: "",
    shadowBuilderAdjustments: undefined,
    retriable: 0,
    builderHealthMetricsLinks: undefined,
    contactTeamEmail: "",
  };
}

export const BuilderConfig = {
  encode(message: BuilderConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.backend !== undefined) {
      BuilderConfig_Backend.encode(message.backend, writer.uint32(258).fork()).ldelim();
    }
    if (message.backendAlt !== undefined) {
      BuilderConfig_Backend.encode(message.backendAlt, writer.uint32(266).fork()).ldelim();
    }
    if (message.swarmingHost !== "") {
      writer.uint32(170).string(message.swarmingHost);
    }
    if (message.category !== "") {
      writer.uint32(50).string(message.category);
    }
    for (const v of message.swarmingTags) {
      writer.uint32(18).string(v!);
    }
    for (const v of message.dimensions) {
      writer.uint32(26).string(v!);
    }
    if (message.recipe !== undefined) {
      BuilderConfig_Recipe.encode(message.recipe, writer.uint32(34).fork()).ldelim();
    }
    if (message.exe !== undefined) {
      Executable.encode(message.exe, writer.uint32(186).fork()).ldelim();
    }
    if (message.properties !== "") {
      writer.uint32(194).string(message.properties);
    }
    for (const v of message.allowedPropertyOverrides) {
      writer.uint32(274).string(v!);
    }
    if (message.priority !== 0) {
      writer.uint32(40).uint32(message.priority);
    }
    if (message.executionTimeoutSecs !== 0) {
      writer.uint32(56).uint32(message.executionTimeoutSecs);
    }
    if (message.expirationSecs !== 0) {
      writer.uint32(160).uint32(message.expirationSecs);
    }
    if (message.gracePeriod !== undefined) {
      Duration.encode(message.gracePeriod, writer.uint32(250).fork()).ldelim();
    }
    if (message.waitForCapacity !== 0) {
      writer.uint32(232).int32(message.waitForCapacity);
    }
    for (const v of message.caches) {
      BuilderConfig_CacheEntry.encode(v!, writer.uint32(74).fork()).ldelim();
    }
    if (message.buildNumbers !== 0) {
      writer.uint32(128).int32(message.buildNumbers);
    }
    if (message.serviceAccount !== "") {
      writer.uint32(98).string(message.serviceAccount);
    }
    if (message.autoBuilderDimension !== 0) {
      writer.uint32(136).int32(message.autoBuilderDimension);
    }
    if (message.experimental !== 0) {
      writer.uint32(144).int32(message.experimental);
    }
    if (message.taskTemplateCanaryPercentage !== undefined) {
      UInt32Value.encode({ value: message.taskTemplateCanaryPercentage! }, writer.uint32(178).fork()).ldelim();
    }
    Object.entries(message.experiments).forEach(([key, value]) => {
      BuilderConfig_ExperimentsEntry.encode({ key: key as any, value }, writer.uint32(226).fork()).ldelim();
    });
    if (message.critical !== 0) {
      writer.uint32(200).int32(message.critical);
    }
    if (message.resultdb !== undefined) {
      BuilderConfig_ResultDB.encode(message.resultdb, writer.uint32(210).fork()).ldelim();
    }
    if (message.descriptionHtml !== "") {
      writer.uint32(242).string(message.descriptionHtml);
    }
    if (message.shadowBuilderAdjustments !== undefined) {
      BuilderConfig_ShadowBuilderAdjustments.encode(message.shadowBuilderAdjustments, writer.uint32(282).fork())
        .ldelim();
    }
    if (message.retriable !== 0) {
      writer.uint32(288).int32(message.retriable);
    }
    if (message.builderHealthMetricsLinks !== undefined) {
      BuilderConfig_BuilderHealthLinks.encode(message.builderHealthMetricsLinks, writer.uint32(298).fork()).ldelim();
    }
    if (message.contactTeamEmail !== "") {
      writer.uint32(306).string(message.contactTeamEmail);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        case 32:
          if (tag !== 258) {
            break;
          }

          message.backend = BuilderConfig_Backend.decode(reader, reader.uint32());
          continue;
        case 33:
          if (tag !== 266) {
            break;
          }

          message.backendAlt = BuilderConfig_Backend.decode(reader, reader.uint32());
          continue;
        case 21:
          if (tag !== 170) {
            break;
          }

          message.swarmingHost = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.category = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.swarmingTags.push(reader.string());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.dimensions.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.recipe = BuilderConfig_Recipe.decode(reader, reader.uint32());
          continue;
        case 23:
          if (tag !== 186) {
            break;
          }

          message.exe = Executable.decode(reader, reader.uint32());
          continue;
        case 24:
          if (tag !== 194) {
            break;
          }

          message.properties = reader.string();
          continue;
        case 34:
          if (tag !== 274) {
            break;
          }

          message.allowedPropertyOverrides.push(reader.string());
          continue;
        case 5:
          if (tag !== 40) {
            break;
          }

          message.priority = reader.uint32();
          continue;
        case 7:
          if (tag !== 56) {
            break;
          }

          message.executionTimeoutSecs = reader.uint32();
          continue;
        case 20:
          if (tag !== 160) {
            break;
          }

          message.expirationSecs = reader.uint32();
          continue;
        case 31:
          if (tag !== 250) {
            break;
          }

          message.gracePeriod = Duration.decode(reader, reader.uint32());
          continue;
        case 29:
          if (tag !== 232) {
            break;
          }

          message.waitForCapacity = reader.int32() as any;
          continue;
        case 9:
          if (tag !== 74) {
            break;
          }

          message.caches.push(BuilderConfig_CacheEntry.decode(reader, reader.uint32()));
          continue;
        case 16:
          if (tag !== 128) {
            break;
          }

          message.buildNumbers = reader.int32() as any;
          continue;
        case 12:
          if (tag !== 98) {
            break;
          }

          message.serviceAccount = reader.string();
          continue;
        case 17:
          if (tag !== 136) {
            break;
          }

          message.autoBuilderDimension = reader.int32() as any;
          continue;
        case 18:
          if (tag !== 144) {
            break;
          }

          message.experimental = reader.int32() as any;
          continue;
        case 22:
          if (tag !== 178) {
            break;
          }

          message.taskTemplateCanaryPercentage = UInt32Value.decode(reader, reader.uint32()).value;
          continue;
        case 28:
          if (tag !== 226) {
            break;
          }

          const entry28 = BuilderConfig_ExperimentsEntry.decode(reader, reader.uint32());
          if (entry28.value !== undefined) {
            message.experiments[entry28.key] = entry28.value;
          }
          continue;
        case 25:
          if (tag !== 200) {
            break;
          }

          message.critical = reader.int32() as any;
          continue;
        case 26:
          if (tag !== 210) {
            break;
          }

          message.resultdb = BuilderConfig_ResultDB.decode(reader, reader.uint32());
          continue;
        case 30:
          if (tag !== 242) {
            break;
          }

          message.descriptionHtml = reader.string();
          continue;
        case 35:
          if (tag !== 282) {
            break;
          }

          message.shadowBuilderAdjustments = BuilderConfig_ShadowBuilderAdjustments.decode(reader, reader.uint32());
          continue;
        case 36:
          if (tag !== 288) {
            break;
          }

          message.retriable = reader.int32() as any;
          continue;
        case 37:
          if (tag !== 298) {
            break;
          }

          message.builderHealthMetricsLinks = BuilderConfig_BuilderHealthLinks.decode(reader, reader.uint32());
          continue;
        case 38:
          if (tag !== 306) {
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

  fromJSON(object: any): BuilderConfig {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      backend: isSet(object.backend) ? BuilderConfig_Backend.fromJSON(object.backend) : undefined,
      backendAlt: isSet(object.backendAlt) ? BuilderConfig_Backend.fromJSON(object.backendAlt) : undefined,
      swarmingHost: isSet(object.swarmingHost) ? globalThis.String(object.swarmingHost) : "",
      category: isSet(object.category) ? globalThis.String(object.category) : "",
      swarmingTags: globalThis.Array.isArray(object?.swarmingTags)
        ? object.swarmingTags.map((e: any) => globalThis.String(e))
        : [],
      dimensions: globalThis.Array.isArray(object?.dimensions)
        ? object.dimensions.map((e: any) => globalThis.String(e))
        : [],
      recipe: isSet(object.recipe) ? BuilderConfig_Recipe.fromJSON(object.recipe) : undefined,
      exe: isSet(object.exe) ? Executable.fromJSON(object.exe) : undefined,
      properties: isSet(object.properties) ? globalThis.String(object.properties) : "",
      allowedPropertyOverrides: globalThis.Array.isArray(object?.allowedPropertyOverrides)
        ? object.allowedPropertyOverrides.map((e: any) => globalThis.String(e))
        : [],
      priority: isSet(object.priority) ? globalThis.Number(object.priority) : 0,
      executionTimeoutSecs: isSet(object.executionTimeoutSecs) ? globalThis.Number(object.executionTimeoutSecs) : 0,
      expirationSecs: isSet(object.expirationSecs) ? globalThis.Number(object.expirationSecs) : 0,
      gracePeriod: isSet(object.gracePeriod) ? Duration.fromJSON(object.gracePeriod) : undefined,
      waitForCapacity: isSet(object.waitForCapacity) ? trinaryFromJSON(object.waitForCapacity) : 0,
      caches: globalThis.Array.isArray(object?.caches)
        ? object.caches.map((e: any) => BuilderConfig_CacheEntry.fromJSON(e))
        : [],
      buildNumbers: isSet(object.buildNumbers) ? toggleFromJSON(object.buildNumbers) : 0,
      serviceAccount: isSet(object.serviceAccount) ? globalThis.String(object.serviceAccount) : "",
      autoBuilderDimension: isSet(object.autoBuilderDimension) ? toggleFromJSON(object.autoBuilderDimension) : 0,
      experimental: isSet(object.experimental) ? toggleFromJSON(object.experimental) : 0,
      taskTemplateCanaryPercentage: isSet(object.taskTemplateCanaryPercentage)
        ? Number(object.taskTemplateCanaryPercentage)
        : undefined,
      experiments: isObject(object.experiments)
        ? Object.entries(object.experiments).reduce<{ [key: string]: number }>((acc, [key, value]) => {
          acc[key] = Number(value);
          return acc;
        }, {})
        : {},
      critical: isSet(object.critical) ? trinaryFromJSON(object.critical) : 0,
      resultdb: isSet(object.resultdb) ? BuilderConfig_ResultDB.fromJSON(object.resultdb) : undefined,
      descriptionHtml: isSet(object.descriptionHtml) ? globalThis.String(object.descriptionHtml) : "",
      shadowBuilderAdjustments: isSet(object.shadowBuilderAdjustments)
        ? BuilderConfig_ShadowBuilderAdjustments.fromJSON(object.shadowBuilderAdjustments)
        : undefined,
      retriable: isSet(object.retriable) ? trinaryFromJSON(object.retriable) : 0,
      builderHealthMetricsLinks: isSet(object.builderHealthMetricsLinks)
        ? BuilderConfig_BuilderHealthLinks.fromJSON(object.builderHealthMetricsLinks)
        : undefined,
      contactTeamEmail: isSet(object.contactTeamEmail) ? globalThis.String(object.contactTeamEmail) : "",
    };
  },

  toJSON(message: BuilderConfig): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.backend !== undefined) {
      obj.backend = BuilderConfig_Backend.toJSON(message.backend);
    }
    if (message.backendAlt !== undefined) {
      obj.backendAlt = BuilderConfig_Backend.toJSON(message.backendAlt);
    }
    if (message.swarmingHost !== "") {
      obj.swarmingHost = message.swarmingHost;
    }
    if (message.category !== "") {
      obj.category = message.category;
    }
    if (message.swarmingTags?.length) {
      obj.swarmingTags = message.swarmingTags;
    }
    if (message.dimensions?.length) {
      obj.dimensions = message.dimensions;
    }
    if (message.recipe !== undefined) {
      obj.recipe = BuilderConfig_Recipe.toJSON(message.recipe);
    }
    if (message.exe !== undefined) {
      obj.exe = Executable.toJSON(message.exe);
    }
    if (message.properties !== "") {
      obj.properties = message.properties;
    }
    if (message.allowedPropertyOverrides?.length) {
      obj.allowedPropertyOverrides = message.allowedPropertyOverrides;
    }
    if (message.priority !== 0) {
      obj.priority = Math.round(message.priority);
    }
    if (message.executionTimeoutSecs !== 0) {
      obj.executionTimeoutSecs = Math.round(message.executionTimeoutSecs);
    }
    if (message.expirationSecs !== 0) {
      obj.expirationSecs = Math.round(message.expirationSecs);
    }
    if (message.gracePeriod !== undefined) {
      obj.gracePeriod = Duration.toJSON(message.gracePeriod);
    }
    if (message.waitForCapacity !== 0) {
      obj.waitForCapacity = trinaryToJSON(message.waitForCapacity);
    }
    if (message.caches?.length) {
      obj.caches = message.caches.map((e) => BuilderConfig_CacheEntry.toJSON(e));
    }
    if (message.buildNumbers !== 0) {
      obj.buildNumbers = toggleToJSON(message.buildNumbers);
    }
    if (message.serviceAccount !== "") {
      obj.serviceAccount = message.serviceAccount;
    }
    if (message.autoBuilderDimension !== 0) {
      obj.autoBuilderDimension = toggleToJSON(message.autoBuilderDimension);
    }
    if (message.experimental !== 0) {
      obj.experimental = toggleToJSON(message.experimental);
    }
    if (message.taskTemplateCanaryPercentage !== undefined) {
      obj.taskTemplateCanaryPercentage = message.taskTemplateCanaryPercentage;
    }
    if (message.experiments) {
      const entries = Object.entries(message.experiments);
      if (entries.length > 0) {
        obj.experiments = {};
        entries.forEach(([k, v]) => {
          obj.experiments[k] = Math.round(v);
        });
      }
    }
    if (message.critical !== 0) {
      obj.critical = trinaryToJSON(message.critical);
    }
    if (message.resultdb !== undefined) {
      obj.resultdb = BuilderConfig_ResultDB.toJSON(message.resultdb);
    }
    if (message.descriptionHtml !== "") {
      obj.descriptionHtml = message.descriptionHtml;
    }
    if (message.shadowBuilderAdjustments !== undefined) {
      obj.shadowBuilderAdjustments = BuilderConfig_ShadowBuilderAdjustments.toJSON(message.shadowBuilderAdjustments);
    }
    if (message.retriable !== 0) {
      obj.retriable = trinaryToJSON(message.retriable);
    }
    if (message.builderHealthMetricsLinks !== undefined) {
      obj.builderHealthMetricsLinks = BuilderConfig_BuilderHealthLinks.toJSON(message.builderHealthMetricsLinks);
    }
    if (message.contactTeamEmail !== "") {
      obj.contactTeamEmail = message.contactTeamEmail;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig>, I>>(base?: I): BuilderConfig {
    return BuilderConfig.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig>, I>>(object: I): BuilderConfig {
    const message = createBaseBuilderConfig() as any;
    message.name = object.name ?? "";
    message.backend = (object.backend !== undefined && object.backend !== null)
      ? BuilderConfig_Backend.fromPartial(object.backend)
      : undefined;
    message.backendAlt = (object.backendAlt !== undefined && object.backendAlt !== null)
      ? BuilderConfig_Backend.fromPartial(object.backendAlt)
      : undefined;
    message.swarmingHost = object.swarmingHost ?? "";
    message.category = object.category ?? "";
    message.swarmingTags = object.swarmingTags?.map((e) => e) || [];
    message.dimensions = object.dimensions?.map((e) => e) || [];
    message.recipe = (object.recipe !== undefined && object.recipe !== null)
      ? BuilderConfig_Recipe.fromPartial(object.recipe)
      : undefined;
    message.exe = (object.exe !== undefined && object.exe !== null) ? Executable.fromPartial(object.exe) : undefined;
    message.properties = object.properties ?? "";
    message.allowedPropertyOverrides = object.allowedPropertyOverrides?.map((e) => e) || [];
    message.priority = object.priority ?? 0;
    message.executionTimeoutSecs = object.executionTimeoutSecs ?? 0;
    message.expirationSecs = object.expirationSecs ?? 0;
    message.gracePeriod = (object.gracePeriod !== undefined && object.gracePeriod !== null)
      ? Duration.fromPartial(object.gracePeriod)
      : undefined;
    message.waitForCapacity = object.waitForCapacity ?? 0;
    message.caches = object.caches?.map((e) => BuilderConfig_CacheEntry.fromPartial(e)) || [];
    message.buildNumbers = object.buildNumbers ?? 0;
    message.serviceAccount = object.serviceAccount ?? "";
    message.autoBuilderDimension = object.autoBuilderDimension ?? 0;
    message.experimental = object.experimental ?? 0;
    message.taskTemplateCanaryPercentage = object.taskTemplateCanaryPercentage ?? undefined;
    message.experiments = Object.entries(object.experiments ?? {}).reduce<{ [key: string]: number }>(
      (acc, [key, value]) => {
        if (value !== undefined) {
          acc[key] = globalThis.Number(value);
        }
        return acc;
      },
      {},
    );
    message.critical = object.critical ?? 0;
    message.resultdb = (object.resultdb !== undefined && object.resultdb !== null)
      ? BuilderConfig_ResultDB.fromPartial(object.resultdb)
      : undefined;
    message.descriptionHtml = object.descriptionHtml ?? "";
    message.shadowBuilderAdjustments =
      (object.shadowBuilderAdjustments !== undefined && object.shadowBuilderAdjustments !== null)
        ? BuilderConfig_ShadowBuilderAdjustments.fromPartial(object.shadowBuilderAdjustments)
        : undefined;
    message.retriable = object.retriable ?? 0;
    message.builderHealthMetricsLinks =
      (object.builderHealthMetricsLinks !== undefined && object.builderHealthMetricsLinks !== null)
        ? BuilderConfig_BuilderHealthLinks.fromPartial(object.builderHealthMetricsLinks)
        : undefined;
    message.contactTeamEmail = object.contactTeamEmail ?? "";
    return message;
  },
};

function createBaseBuilderConfig_CacheEntry(): BuilderConfig_CacheEntry {
  return { name: "", path: "", waitForWarmCacheSecs: 0, envVar: "" };
}

export const BuilderConfig_CacheEntry = {
  encode(message: BuilderConfig_CacheEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.path !== "") {
      writer.uint32(18).string(message.path);
    }
    if (message.waitForWarmCacheSecs !== 0) {
      writer.uint32(24).int32(message.waitForWarmCacheSecs);
    }
    if (message.envVar !== "") {
      writer.uint32(34).string(message.envVar);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_CacheEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_CacheEntry() as any;
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
          if (tag !== 24) {
            break;
          }

          message.waitForWarmCacheSecs = reader.int32();
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

  fromJSON(object: any): BuilderConfig_CacheEntry {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      path: isSet(object.path) ? globalThis.String(object.path) : "",
      waitForWarmCacheSecs: isSet(object.waitForWarmCacheSecs) ? globalThis.Number(object.waitForWarmCacheSecs) : 0,
      envVar: isSet(object.envVar) ? globalThis.String(object.envVar) : "",
    };
  },

  toJSON(message: BuilderConfig_CacheEntry): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.path !== "") {
      obj.path = message.path;
    }
    if (message.waitForWarmCacheSecs !== 0) {
      obj.waitForWarmCacheSecs = Math.round(message.waitForWarmCacheSecs);
    }
    if (message.envVar !== "") {
      obj.envVar = message.envVar;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_CacheEntry>, I>>(base?: I): BuilderConfig_CacheEntry {
    return BuilderConfig_CacheEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_CacheEntry>, I>>(object: I): BuilderConfig_CacheEntry {
    const message = createBaseBuilderConfig_CacheEntry() as any;
    message.name = object.name ?? "";
    message.path = object.path ?? "";
    message.waitForWarmCacheSecs = object.waitForWarmCacheSecs ?? 0;
    message.envVar = object.envVar ?? "";
    return message;
  },
};

function createBaseBuilderConfig_Recipe(): BuilderConfig_Recipe {
  return { name: "", cipdPackage: "", cipdVersion: "", properties: [], propertiesJ: [] };
}

export const BuilderConfig_Recipe = {
  encode(message: BuilderConfig_Recipe, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(18).string(message.name);
    }
    if (message.cipdPackage !== "") {
      writer.uint32(50).string(message.cipdPackage);
    }
    if (message.cipdVersion !== "") {
      writer.uint32(42).string(message.cipdVersion);
    }
    for (const v of message.properties) {
      writer.uint32(26).string(v!);
    }
    for (const v of message.propertiesJ) {
      writer.uint32(34).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_Recipe {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_Recipe() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 2:
          if (tag !== 18) {
            break;
          }

          message.name = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.cipdPackage = reader.string();
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.cipdVersion = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.properties.push(reader.string());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.propertiesJ.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuilderConfig_Recipe {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      cipdPackage: isSet(object.cipdPackage) ? globalThis.String(object.cipdPackage) : "",
      cipdVersion: isSet(object.cipdVersion) ? globalThis.String(object.cipdVersion) : "",
      properties: globalThis.Array.isArray(object?.properties)
        ? object.properties.map((e: any) => globalThis.String(e))
        : [],
      propertiesJ: globalThis.Array.isArray(object?.propertiesJ)
        ? object.propertiesJ.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: BuilderConfig_Recipe): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.cipdPackage !== "") {
      obj.cipdPackage = message.cipdPackage;
    }
    if (message.cipdVersion !== "") {
      obj.cipdVersion = message.cipdVersion;
    }
    if (message.properties?.length) {
      obj.properties = message.properties;
    }
    if (message.propertiesJ?.length) {
      obj.propertiesJ = message.propertiesJ;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_Recipe>, I>>(base?: I): BuilderConfig_Recipe {
    return BuilderConfig_Recipe.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_Recipe>, I>>(object: I): BuilderConfig_Recipe {
    const message = createBaseBuilderConfig_Recipe() as any;
    message.name = object.name ?? "";
    message.cipdPackage = object.cipdPackage ?? "";
    message.cipdVersion = object.cipdVersion ?? "";
    message.properties = object.properties?.map((e) => e) || [];
    message.propertiesJ = object.propertiesJ?.map((e) => e) || [];
    return message;
  },
};

function createBaseBuilderConfig_ResultDB(): BuilderConfig_ResultDB {
  return { enable: false, bqExports: [], historyOptions: undefined };
}

export const BuilderConfig_ResultDB = {
  encode(message: BuilderConfig_ResultDB, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.enable === true) {
      writer.uint32(8).bool(message.enable);
    }
    for (const v of message.bqExports) {
      BigQueryExport.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.historyOptions !== undefined) {
      HistoryOptions.encode(message.historyOptions, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_ResultDB {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_ResultDB() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break;
          }

          message.enable = reader.bool();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.bqExports.push(BigQueryExport.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
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

  fromJSON(object: any): BuilderConfig_ResultDB {
    return {
      enable: isSet(object.enable) ? globalThis.Boolean(object.enable) : false,
      bqExports: globalThis.Array.isArray(object?.bqExports)
        ? object.bqExports.map((e: any) => BigQueryExport.fromJSON(e))
        : [],
      historyOptions: isSet(object.historyOptions) ? HistoryOptions.fromJSON(object.historyOptions) : undefined,
    };
  },

  toJSON(message: BuilderConfig_ResultDB): unknown {
    const obj: any = {};
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

  create<I extends Exact<DeepPartial<BuilderConfig_ResultDB>, I>>(base?: I): BuilderConfig_ResultDB {
    return BuilderConfig_ResultDB.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_ResultDB>, I>>(object: I): BuilderConfig_ResultDB {
    const message = createBaseBuilderConfig_ResultDB() as any;
    message.enable = object.enable ?? false;
    message.bqExports = object.bqExports?.map((e) => BigQueryExport.fromPartial(e)) || [];
    message.historyOptions = (object.historyOptions !== undefined && object.historyOptions !== null)
      ? HistoryOptions.fromPartial(object.historyOptions)
      : undefined;
    return message;
  },
};

function createBaseBuilderConfig_Backend(): BuilderConfig_Backend {
  return { target: "", configJson: "" };
}

export const BuilderConfig_Backend = {
  encode(message: BuilderConfig_Backend, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.target !== "") {
      writer.uint32(10).string(message.target);
    }
    if (message.configJson !== "") {
      writer.uint32(18).string(message.configJson);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_Backend {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_Backend() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.target = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.configJson = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuilderConfig_Backend {
    return {
      target: isSet(object.target) ? globalThis.String(object.target) : "",
      configJson: isSet(object.configJson) ? globalThis.String(object.configJson) : "",
    };
  },

  toJSON(message: BuilderConfig_Backend): unknown {
    const obj: any = {};
    if (message.target !== "") {
      obj.target = message.target;
    }
    if (message.configJson !== "") {
      obj.configJson = message.configJson;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_Backend>, I>>(base?: I): BuilderConfig_Backend {
    return BuilderConfig_Backend.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_Backend>, I>>(object: I): BuilderConfig_Backend {
    const message = createBaseBuilderConfig_Backend() as any;
    message.target = object.target ?? "";
    message.configJson = object.configJson ?? "";
    return message;
  },
};

function createBaseBuilderConfig_ExperimentsEntry(): BuilderConfig_ExperimentsEntry {
  return { key: "", value: 0 };
}

export const BuilderConfig_ExperimentsEntry = {
  encode(message: BuilderConfig_ExperimentsEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== 0) {
      writer.uint32(16).int32(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_ExperimentsEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_ExperimentsEntry() as any;
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

          message.value = reader.int32();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuilderConfig_ExperimentsEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.Number(object.value) : 0,
    };
  },

  toJSON(message: BuilderConfig_ExperimentsEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== 0) {
      obj.value = Math.round(message.value);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_ExperimentsEntry>, I>>(base?: I): BuilderConfig_ExperimentsEntry {
    return BuilderConfig_ExperimentsEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_ExperimentsEntry>, I>>(
    object: I,
  ): BuilderConfig_ExperimentsEntry {
    const message = createBaseBuilderConfig_ExperimentsEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? 0;
    return message;
  },
};

function createBaseBuilderConfig_ShadowBuilderAdjustments(): BuilderConfig_ShadowBuilderAdjustments {
  return { serviceAccount: "", pool: "", properties: "", dimensions: [] };
}

export const BuilderConfig_ShadowBuilderAdjustments = {
  encode(message: BuilderConfig_ShadowBuilderAdjustments, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.serviceAccount !== "") {
      writer.uint32(10).string(message.serviceAccount);
    }
    if (message.pool !== "") {
      writer.uint32(18).string(message.pool);
    }
    if (message.properties !== "") {
      writer.uint32(26).string(message.properties);
    }
    for (const v of message.dimensions) {
      writer.uint32(34).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_ShadowBuilderAdjustments {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_ShadowBuilderAdjustments() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.serviceAccount = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.pool = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.properties = reader.string();
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.dimensions.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuilderConfig_ShadowBuilderAdjustments {
    return {
      serviceAccount: isSet(object.serviceAccount) ? globalThis.String(object.serviceAccount) : "",
      pool: isSet(object.pool) ? globalThis.String(object.pool) : "",
      properties: isSet(object.properties) ? globalThis.String(object.properties) : "",
      dimensions: globalThis.Array.isArray(object?.dimensions)
        ? object.dimensions.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: BuilderConfig_ShadowBuilderAdjustments): unknown {
    const obj: any = {};
    if (message.serviceAccount !== "") {
      obj.serviceAccount = message.serviceAccount;
    }
    if (message.pool !== "") {
      obj.pool = message.pool;
    }
    if (message.properties !== "") {
      obj.properties = message.properties;
    }
    if (message.dimensions?.length) {
      obj.dimensions = message.dimensions;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_ShadowBuilderAdjustments>, I>>(
    base?: I,
  ): BuilderConfig_ShadowBuilderAdjustments {
    return BuilderConfig_ShadowBuilderAdjustments.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_ShadowBuilderAdjustments>, I>>(
    object: I,
  ): BuilderConfig_ShadowBuilderAdjustments {
    const message = createBaseBuilderConfig_ShadowBuilderAdjustments() as any;
    message.serviceAccount = object.serviceAccount ?? "";
    message.pool = object.pool ?? "";
    message.properties = object.properties ?? "";
    message.dimensions = object.dimensions?.map((e) => e) || [];
    return message;
  },
};

function createBaseBuilderConfig_BuilderHealthLinks(): BuilderConfig_BuilderHealthLinks {
  return { docLinks: {}, dataLinks: {} };
}

export const BuilderConfig_BuilderHealthLinks = {
  encode(message: BuilderConfig_BuilderHealthLinks, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    Object.entries(message.docLinks).forEach(([key, value]) => {
      BuilderConfig_BuilderHealthLinks_DocLinksEntry.encode({ key: key as any, value }, writer.uint32(10).fork())
        .ldelim();
    });
    Object.entries(message.dataLinks).forEach(([key, value]) => {
      BuilderConfig_BuilderHealthLinks_DataLinksEntry.encode({ key: key as any, value }, writer.uint32(18).fork())
        .ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_BuilderHealthLinks {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_BuilderHealthLinks() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          const entry1 = BuilderConfig_BuilderHealthLinks_DocLinksEntry.decode(reader, reader.uint32());
          if (entry1.value !== undefined) {
            message.docLinks[entry1.key] = entry1.value;
          }
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          const entry2 = BuilderConfig_BuilderHealthLinks_DataLinksEntry.decode(reader, reader.uint32());
          if (entry2.value !== undefined) {
            message.dataLinks[entry2.key] = entry2.value;
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

  fromJSON(object: any): BuilderConfig_BuilderHealthLinks {
    return {
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
    };
  },

  toJSON(message: BuilderConfig_BuilderHealthLinks): unknown {
    const obj: any = {};
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
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_BuilderHealthLinks>, I>>(
    base?: I,
  ): BuilderConfig_BuilderHealthLinks {
    return BuilderConfig_BuilderHealthLinks.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_BuilderHealthLinks>, I>>(
    object: I,
  ): BuilderConfig_BuilderHealthLinks {
    const message = createBaseBuilderConfig_BuilderHealthLinks() as any;
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
    return message;
  },
};

function createBaseBuilderConfig_BuilderHealthLinks_DocLinksEntry(): BuilderConfig_BuilderHealthLinks_DocLinksEntry {
  return { key: "", value: "" };
}

export const BuilderConfig_BuilderHealthLinks_DocLinksEntry = {
  encode(
    message: BuilderConfig_BuilderHealthLinks_DocLinksEntry,
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

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_BuilderHealthLinks_DocLinksEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_BuilderHealthLinks_DocLinksEntry() as any;
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

  fromJSON(object: any): BuilderConfig_BuilderHealthLinks_DocLinksEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: BuilderConfig_BuilderHealthLinks_DocLinksEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_BuilderHealthLinks_DocLinksEntry>, I>>(
    base?: I,
  ): BuilderConfig_BuilderHealthLinks_DocLinksEntry {
    return BuilderConfig_BuilderHealthLinks_DocLinksEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_BuilderHealthLinks_DocLinksEntry>, I>>(
    object: I,
  ): BuilderConfig_BuilderHealthLinks_DocLinksEntry {
    const message = createBaseBuilderConfig_BuilderHealthLinks_DocLinksEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

function createBaseBuilderConfig_BuilderHealthLinks_DataLinksEntry(): BuilderConfig_BuilderHealthLinks_DataLinksEntry {
  return { key: "", value: "" };
}

export const BuilderConfig_BuilderHealthLinks_DataLinksEntry = {
  encode(
    message: BuilderConfig_BuilderHealthLinks_DataLinksEntry,
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

  decode(input: _m0.Reader | Uint8Array, length?: number): BuilderConfig_BuilderHealthLinks_DataLinksEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuilderConfig_BuilderHealthLinks_DataLinksEntry() as any;
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

  fromJSON(object: any): BuilderConfig_BuilderHealthLinks_DataLinksEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: BuilderConfig_BuilderHealthLinks_DataLinksEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuilderConfig_BuilderHealthLinks_DataLinksEntry>, I>>(
    base?: I,
  ): BuilderConfig_BuilderHealthLinks_DataLinksEntry {
    return BuilderConfig_BuilderHealthLinks_DataLinksEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuilderConfig_BuilderHealthLinks_DataLinksEntry>, I>>(
    object: I,
  ): BuilderConfig_BuilderHealthLinks_DataLinksEntry {
    const message = createBaseBuilderConfig_BuilderHealthLinks_DataLinksEntry() as any;
    message.key = object.key ?? "";
    message.value = object.value ?? "";
    return message;
  },
};

function createBaseSwarming(): Swarming {
  return { builders: [], taskTemplateCanaryPercentage: undefined };
}

export const Swarming = {
  encode(message: Swarming, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.builders) {
      BuilderConfig.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    if (message.taskTemplateCanaryPercentage !== undefined) {
      UInt32Value.encode({ value: message.taskTemplateCanaryPercentage! }, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Swarming {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSwarming() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 4:
          if (tag !== 34) {
            break;
          }

          message.builders.push(BuilderConfig.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.taskTemplateCanaryPercentage = UInt32Value.decode(reader, reader.uint32()).value;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Swarming {
    return {
      builders: globalThis.Array.isArray(object?.builders)
        ? object.builders.map((e: any) => BuilderConfig.fromJSON(e))
        : [],
      taskTemplateCanaryPercentage: isSet(object.taskTemplateCanaryPercentage)
        ? Number(object.taskTemplateCanaryPercentage)
        : undefined,
    };
  },

  toJSON(message: Swarming): unknown {
    const obj: any = {};
    if (message.builders?.length) {
      obj.builders = message.builders.map((e) => BuilderConfig.toJSON(e));
    }
    if (message.taskTemplateCanaryPercentage !== undefined) {
      obj.taskTemplateCanaryPercentage = message.taskTemplateCanaryPercentage;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Swarming>, I>>(base?: I): Swarming {
    return Swarming.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Swarming>, I>>(object: I): Swarming {
    const message = createBaseSwarming() as any;
    message.builders = object.builders?.map((e) => BuilderConfig.fromPartial(e)) || [];
    message.taskTemplateCanaryPercentage = object.taskTemplateCanaryPercentage ?? undefined;
    return message;
  },
};

function createBaseBucket(): Bucket {
  return {
    name: "",
    acls: [],
    swarming: undefined,
    shadow: "",
    constraints: undefined,
    dynamicBuilderTemplate: undefined,
  };
}

export const Bucket = {
  encode(message: Bucket, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    for (const v of message.acls) {
      Acl.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.swarming !== undefined) {
      Swarming.encode(message.swarming, writer.uint32(26).fork()).ldelim();
    }
    if (message.shadow !== "") {
      writer.uint32(42).string(message.shadow);
    }
    if (message.constraints !== undefined) {
      Bucket_Constraints.encode(message.constraints, writer.uint32(50).fork()).ldelim();
    }
    if (message.dynamicBuilderTemplate !== undefined) {
      Bucket_DynamicBuilderTemplate.encode(message.dynamicBuilderTemplate, writer.uint32(58).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Bucket {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBucket() as any;
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

          message.acls.push(Acl.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.swarming = Swarming.decode(reader, reader.uint32());
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.shadow = reader.string();
          continue;
        case 6:
          if (tag !== 50) {
            break;
          }

          message.constraints = Bucket_Constraints.decode(reader, reader.uint32());
          continue;
        case 7:
          if (tag !== 58) {
            break;
          }

          message.dynamicBuilderTemplate = Bucket_DynamicBuilderTemplate.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Bucket {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      acls: globalThis.Array.isArray(object?.acls) ? object.acls.map((e: any) => Acl.fromJSON(e)) : [],
      swarming: isSet(object.swarming) ? Swarming.fromJSON(object.swarming) : undefined,
      shadow: isSet(object.shadow) ? globalThis.String(object.shadow) : "",
      constraints: isSet(object.constraints) ? Bucket_Constraints.fromJSON(object.constraints) : undefined,
      dynamicBuilderTemplate: isSet(object.dynamicBuilderTemplate)
        ? Bucket_DynamicBuilderTemplate.fromJSON(object.dynamicBuilderTemplate)
        : undefined,
    };
  },

  toJSON(message: Bucket): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.acls?.length) {
      obj.acls = message.acls.map((e) => Acl.toJSON(e));
    }
    if (message.swarming !== undefined) {
      obj.swarming = Swarming.toJSON(message.swarming);
    }
    if (message.shadow !== "") {
      obj.shadow = message.shadow;
    }
    if (message.constraints !== undefined) {
      obj.constraints = Bucket_Constraints.toJSON(message.constraints);
    }
    if (message.dynamicBuilderTemplate !== undefined) {
      obj.dynamicBuilderTemplate = Bucket_DynamicBuilderTemplate.toJSON(message.dynamicBuilderTemplate);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Bucket>, I>>(base?: I): Bucket {
    return Bucket.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Bucket>, I>>(object: I): Bucket {
    const message = createBaseBucket() as any;
    message.name = object.name ?? "";
    message.acls = object.acls?.map((e) => Acl.fromPartial(e)) || [];
    message.swarming = (object.swarming !== undefined && object.swarming !== null)
      ? Swarming.fromPartial(object.swarming)
      : undefined;
    message.shadow = object.shadow ?? "";
    message.constraints = (object.constraints !== undefined && object.constraints !== null)
      ? Bucket_Constraints.fromPartial(object.constraints)
      : undefined;
    message.dynamicBuilderTemplate =
      (object.dynamicBuilderTemplate !== undefined && object.dynamicBuilderTemplate !== null)
        ? Bucket_DynamicBuilderTemplate.fromPartial(object.dynamicBuilderTemplate)
        : undefined;
    return message;
  },
};

function createBaseBucket_Constraints(): Bucket_Constraints {
  return { pools: [], serviceAccounts: [] };
}

export const Bucket_Constraints = {
  encode(message: Bucket_Constraints, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.pools) {
      writer.uint32(10).string(v!);
    }
    for (const v of message.serviceAccounts) {
      writer.uint32(18).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Bucket_Constraints {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBucket_Constraints() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.pools.push(reader.string());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.serviceAccounts.push(reader.string());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Bucket_Constraints {
    return {
      pools: globalThis.Array.isArray(object?.pools) ? object.pools.map((e: any) => globalThis.String(e)) : [],
      serviceAccounts: globalThis.Array.isArray(object?.serviceAccounts)
        ? object.serviceAccounts.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: Bucket_Constraints): unknown {
    const obj: any = {};
    if (message.pools?.length) {
      obj.pools = message.pools;
    }
    if (message.serviceAccounts?.length) {
      obj.serviceAccounts = message.serviceAccounts;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Bucket_Constraints>, I>>(base?: I): Bucket_Constraints {
    return Bucket_Constraints.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Bucket_Constraints>, I>>(object: I): Bucket_Constraints {
    const message = createBaseBucket_Constraints() as any;
    message.pools = object.pools?.map((e) => e) || [];
    message.serviceAccounts = object.serviceAccounts?.map((e) => e) || [];
    return message;
  },
};

function createBaseBucket_DynamicBuilderTemplate(): Bucket_DynamicBuilderTemplate {
  return {};
}

export const Bucket_DynamicBuilderTemplate = {
  encode(_: Bucket_DynamicBuilderTemplate, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Bucket_DynamicBuilderTemplate {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBucket_DynamicBuilderTemplate() as any;
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

  fromJSON(_: any): Bucket_DynamicBuilderTemplate {
    return {};
  },

  toJSON(_: Bucket_DynamicBuilderTemplate): unknown {
    const obj: any = {};
    return obj;
  },

  create<I extends Exact<DeepPartial<Bucket_DynamicBuilderTemplate>, I>>(base?: I): Bucket_DynamicBuilderTemplate {
    return Bucket_DynamicBuilderTemplate.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Bucket_DynamicBuilderTemplate>, I>>(_: I): Bucket_DynamicBuilderTemplate {
    const message = createBaseBucket_DynamicBuilderTemplate() as any;
    return message;
  },
};

function createBaseBuildbucketCfg(): BuildbucketCfg {
  return { buckets: [], commonConfig: undefined };
}

export const BuildbucketCfg = {
  encode(message: BuildbucketCfg, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.buckets) {
      Bucket.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    if (message.commonConfig !== undefined) {
      BuildbucketCfg_CommonConfig.encode(message.commonConfig, writer.uint32(42).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildbucketCfg {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildbucketCfg() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buckets.push(Bucket.decode(reader, reader.uint32()));
          continue;
        case 5:
          if (tag !== 42) {
            break;
          }

          message.commonConfig = BuildbucketCfg_CommonConfig.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildbucketCfg {
    return {
      buckets: globalThis.Array.isArray(object?.buckets) ? object.buckets.map((e: any) => Bucket.fromJSON(e)) : [],
      commonConfig: isSet(object.commonConfig) ? BuildbucketCfg_CommonConfig.fromJSON(object.commonConfig) : undefined,
    };
  },

  toJSON(message: BuildbucketCfg): unknown {
    const obj: any = {};
    if (message.buckets?.length) {
      obj.buckets = message.buckets.map((e) => Bucket.toJSON(e));
    }
    if (message.commonConfig !== undefined) {
      obj.commonConfig = BuildbucketCfg_CommonConfig.toJSON(message.commonConfig);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildbucketCfg>, I>>(base?: I): BuildbucketCfg {
    return BuildbucketCfg.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildbucketCfg>, I>>(object: I): BuildbucketCfg {
    const message = createBaseBuildbucketCfg() as any;
    message.buckets = object.buckets?.map((e) => Bucket.fromPartial(e)) || [];
    message.commonConfig = (object.commonConfig !== undefined && object.commonConfig !== null)
      ? BuildbucketCfg_CommonConfig.fromPartial(object.commonConfig)
      : undefined;
    return message;
  },
};

function createBaseBuildbucketCfg_Topic(): BuildbucketCfg_Topic {
  return { name: "", compression: 0 };
}

export const BuildbucketCfg_Topic = {
  encode(message: BuildbucketCfg_Topic, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.compression !== 0) {
      writer.uint32(16).int32(message.compression);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildbucketCfg_Topic {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildbucketCfg_Topic() as any;
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
          if (tag !== 16) {
            break;
          }

          message.compression = reader.int32() as any;
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildbucketCfg_Topic {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      compression: isSet(object.compression) ? compressionFromJSON(object.compression) : 0,
    };
  },

  toJSON(message: BuildbucketCfg_Topic): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.compression !== 0) {
      obj.compression = compressionToJSON(message.compression);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildbucketCfg_Topic>, I>>(base?: I): BuildbucketCfg_Topic {
    return BuildbucketCfg_Topic.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildbucketCfg_Topic>, I>>(object: I): BuildbucketCfg_Topic {
    const message = createBaseBuildbucketCfg_Topic() as any;
    message.name = object.name ?? "";
    message.compression = object.compression ?? 0;
    return message;
  },
};

function createBaseBuildbucketCfg_CommonConfig(): BuildbucketCfg_CommonConfig {
  return { buildsNotificationTopics: [] };
}

export const BuildbucketCfg_CommonConfig = {
  encode(message: BuildbucketCfg_CommonConfig, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.buildsNotificationTopics) {
      BuildbucketCfg_Topic.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): BuildbucketCfg_CommonConfig {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseBuildbucketCfg_CommonConfig() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buildsNotificationTopics.push(BuildbucketCfg_Topic.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): BuildbucketCfg_CommonConfig {
    return {
      buildsNotificationTopics: globalThis.Array.isArray(object?.buildsNotificationTopics)
        ? object.buildsNotificationTopics.map((e: any) => BuildbucketCfg_Topic.fromJSON(e))
        : [],
    };
  },

  toJSON(message: BuildbucketCfg_CommonConfig): unknown {
    const obj: any = {};
    if (message.buildsNotificationTopics?.length) {
      obj.buildsNotificationTopics = message.buildsNotificationTopics.map((e) => BuildbucketCfg_Topic.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<BuildbucketCfg_CommonConfig>, I>>(base?: I): BuildbucketCfg_CommonConfig {
    return BuildbucketCfg_CommonConfig.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<BuildbucketCfg_CommonConfig>, I>>(object: I): BuildbucketCfg_CommonConfig {
    const message = createBaseBuildbucketCfg_CommonConfig() as any;
    message.buildsNotificationTopics =
      object.buildsNotificationTopics?.map((e) => BuildbucketCfg_Topic.fromPartial(e)) || [];
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
