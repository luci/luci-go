// Code generated by protoc-gen-ts_proto. DO NOT EDIT.
// versions:
//   protoc-gen-ts_proto  v2.7.5
//   protoc               v6.31.1
// source: go.chromium.org/luci/buildbucket/proto/step.proto

/* eslint-disable */
import { BinaryReader, BinaryWriter } from "@bufbuild/protobuf/wire";
import { Timestamp } from "../../../../google/protobuf/timestamp.pb";
import { Log, Status, statusFromJSON, statusToJSON, StringPair } from "./common.pb";

export const protobufPackage = "buildbucket.v2";

/**
 * A build step.
 *
 * A step may have children, see name field.
 */
export interface Step {
  /**
   * Name of the step, unique within the build.
   * Identifies the step.
   *
   * Pipe character ("|") is reserved to separate parent and child step names.
   * For example, value "a|b" indicates step "b" under step "a".
   * If this is a child step, a parent MUST exist and MUST precede this step in
   * the list of steps.
   * All step names, including child and parent names recursively,
   * MUST NOT be an empty string.
   * For example, all of the below names are invalid.
   * - |a
   * - a|
   * - a||b
   */
  readonly name: string;
  /**
   * The timestamp when the step started.
   *
   * MUST NOT be specified, if status is SCHEDULED.
   * MUST be specified, if status is STARTED, SUCCESS, FAILURE, or INFRA_FAILURE
   * MAY be specified, if status is CANCELED.
   */
  readonly startTime:
    | string
    | undefined;
  /**
   * The timestamp when the step ended.
   * Present iff status is terminal.
   * MUST NOT be before start_time.
   */
  readonly endTime:
    | string
    | undefined;
  /**
   * Status of the step.
   * Must be specified, i.e. not STATUS_UNSPECIFIED.
   */
  readonly status: Status;
  /**
   * Logs produced by the step.
   * Log order is up to the step.
   *
   * BigQuery: excluded from rows.
   */
  readonly logs: readonly Log[];
  /**
   * MergeBuild is used for go.chromium.org/luci/luciexe to indicate to the
   * luciexe host process if some Build stream should be merged under this step.
   *
   * BigQuery: excluded from rows.
   */
  readonly mergeBuild:
    | Step_MergeBuild
    | undefined;
  /**
   * Human-readable summary of the step provided by the step itself,
   * in Markdown format (https://spec.commonmark.org/0.28/).
   *
   * V1 equivalent: combines and supersedes Buildbot's step_text and step links and also supports
   * other formatted text.
   *
   * BigQuery: excluded from rows.
   */
  readonly summaryMarkdown: string;
  /**
   * Arbitrary annotations for the step.
   *
   * One key may have multiple values, which is why this is not a map<string,string>.
   *
   * These are NOT interpreted by Buildbucket.
   *
   * Tag keys SHOULD indicate the domain/system that interprets them, e.g.:
   *
   *   my_service.category = COMPILE
   *
   * Rather than
   *
   *   is_compile = true
   *
   * This will help contextualize the tag values when looking at a build (who
   * set this tag? who will interpret this tag?))
   *
   * The 'luci.' key prefix is reserved for LUCI's own usage.
   *
   * The Key may not exceed 256 bytes.
   * The Value may not exceed 1024 bytes.
   *
   * Key and Value may not be empty.
   */
  readonly tags: readonly StringPair[];
}

export interface Step_MergeBuild {
  /**
   * If set, then this stream is expected to be a datagram stream
   * containing Build messages.
   *
   * This should be the stream name relative to the current build's
   * $LOGDOG_NAMESPACE.
   */
  readonly fromLogdogStream: string;
  /**
   * If set, then this stream will be merged "in line" with this step.
   *
   * Properties emitted by the merge build stream will overwrite global
   * outputs with the same top-level key. This is the same as setting
   * `merge_output_properties_to=["."]`.
   *
   * Steps emitted by the merge build stream will NOT have their names
   * namespaced (though the log stream names are still expected to
   * adhere to the regular luciexe rules).
   *
   * Because this is a legacy feature, this intentionally omits other fields
   * which "could be" merged, because there was no affordance to emit them
   * under the legacy annotator scheme:
   *   * output.gitiles_commit will not be merged.
   *   * output.logs will not be merged.
   *   * summary_markdown will not be merged.
   *
   * This is NOT a recommended mode of operation, but legacy ChromeOS
   * builders rely on this behavior.
   *
   * See crbug.com/1310155.
   *
   * If set in conjunction with merge_output_properties_to,
   * merge_output_properties_to will take precedence for output property
   * merging.
   */
  readonly legacyGlobalNamespace: boolean;
  /**
   * If set, this indicates that the output properties from this subbuild will
   * be merged into the parent's output properties at this given path.
   *
   * If this is exactly `[""]` it means "merge the sub build's output
   * properties directly into the top-level output properties".
   *
   * If this is empty (i.e. `[]` or nil), the sub build's properties will not
   * be merged at all.
   *
   * Otherwise, this will be a path from the root of the parent's output
   * properties to the place to add this build's output properties. For
   * example, the value `["a", "b"]` would mean to merge properties like:
   *
   *    {
   *      "a": {
   *        "b": <this is where the properties will be merged>
   *      }
   *    }
   *
   * Merging works as follows:
   *   * Start with the current Build.output.properties.
   *   * We walk Build.Steps from the top to bottom.
   *   * When we encounter a Step with merge_build set, we set a pointer in
   *     the output properties to this path, possibly creating new Structs
   *     along the way.
   *     * If non-Struct property values are encountered, they are replaced
   *       with Structs.
   *     * If the leaf object is a pre-existing Struct, then this functionally
   *       performs a python `dict.update()` of the sub-build's properties on
   *       top of the parent's properties at that location.
   *   * Recurse into the sub-build, possibly applying further merge_build
   *     directives.
   *
   * This means that if you have:
   *
   *    parent
   *      child1 (merge_output_properties_to=['a'])
   *        child2 (merge_output_properties_to=['b'])
   *
   * Then the output properties of child2 will be at the parent build's
   * ['a', 'b'] path.
   *
   * It is STRONGLY DISCOURAGED to merge into [""] - This creates
   * composability issues and increases the likelihood of unintentional
   * overwrites. However, sometimes this is the appropriate course of action
   * (e.g. when writing thin wrappers over the 'real' implementation as
   * a sub-build).
   */
  readonly mergeOutputPropertiesTo: readonly string[];
}

function createBaseStep(): Step {
  return {
    name: "",
    startTime: undefined,
    endTime: undefined,
    status: 0,
    logs: [],
    mergeBuild: undefined,
    summaryMarkdown: "",
    tags: [],
  };
}

export const Step: MessageFns<Step> = {
  encode(message: Step, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.name !== "") {
      writer.uint32(10).string(message.name);
    }
    if (message.startTime !== undefined) {
      Timestamp.encode(toTimestamp(message.startTime), writer.uint32(18).fork()).join();
    }
    if (message.endTime !== undefined) {
      Timestamp.encode(toTimestamp(message.endTime), writer.uint32(26).fork()).join();
    }
    if (message.status !== 0) {
      writer.uint32(32).int32(message.status);
    }
    for (const v of message.logs) {
      Log.encode(v!, writer.uint32(42).fork()).join();
    }
    if (message.mergeBuild !== undefined) {
      Step_MergeBuild.encode(message.mergeBuild, writer.uint32(50).fork()).join();
    }
    if (message.summaryMarkdown !== "") {
      writer.uint32(58).string(message.summaryMarkdown);
    }
    for (const v of message.tags) {
      StringPair.encode(v!, writer.uint32(66).fork()).join();
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): Step {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStep() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.name = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 18) {
            break;
          }

          message.startTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.endTime = fromTimestamp(Timestamp.decode(reader, reader.uint32()));
          continue;
        }
        case 4: {
          if (tag !== 32) {
            break;
          }

          message.status = reader.int32() as any;
          continue;
        }
        case 5: {
          if (tag !== 42) {
            break;
          }

          message.logs.push(Log.decode(reader, reader.uint32()));
          continue;
        }
        case 6: {
          if (tag !== 50) {
            break;
          }

          message.mergeBuild = Step_MergeBuild.decode(reader, reader.uint32());
          continue;
        }
        case 7: {
          if (tag !== 58) {
            break;
          }

          message.summaryMarkdown = reader.string();
          continue;
        }
        case 8: {
          if (tag !== 66) {
            break;
          }

          message.tags.push(StringPair.decode(reader, reader.uint32()));
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Step {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : "",
      startTime: isSet(object.startTime) ? globalThis.String(object.startTime) : undefined,
      endTime: isSet(object.endTime) ? globalThis.String(object.endTime) : undefined,
      status: isSet(object.status) ? statusFromJSON(object.status) : 0,
      logs: globalThis.Array.isArray(object?.logs) ? object.logs.map((e: any) => Log.fromJSON(e)) : [],
      mergeBuild: isSet(object.mergeBuild) ? Step_MergeBuild.fromJSON(object.mergeBuild) : undefined,
      summaryMarkdown: isSet(object.summaryMarkdown) ? globalThis.String(object.summaryMarkdown) : "",
      tags: globalThis.Array.isArray(object?.tags) ? object.tags.map((e: any) => StringPair.fromJSON(e)) : [],
    };
  },

  toJSON(message: Step): unknown {
    const obj: any = {};
    if (message.name !== "") {
      obj.name = message.name;
    }
    if (message.startTime !== undefined) {
      obj.startTime = message.startTime;
    }
    if (message.endTime !== undefined) {
      obj.endTime = message.endTime;
    }
    if (message.status !== 0) {
      obj.status = statusToJSON(message.status);
    }
    if (message.logs?.length) {
      obj.logs = message.logs.map((e) => Log.toJSON(e));
    }
    if (message.mergeBuild !== undefined) {
      obj.mergeBuild = Step_MergeBuild.toJSON(message.mergeBuild);
    }
    if (message.summaryMarkdown !== "") {
      obj.summaryMarkdown = message.summaryMarkdown;
    }
    if (message.tags?.length) {
      obj.tags = message.tags.map((e) => StringPair.toJSON(e));
    }
    return obj;
  },

  create(base?: DeepPartial<Step>): Step {
    return Step.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<Step>): Step {
    const message = createBaseStep() as any;
    message.name = object.name ?? "";
    message.startTime = object.startTime ?? undefined;
    message.endTime = object.endTime ?? undefined;
    message.status = object.status ?? 0;
    message.logs = object.logs?.map((e) => Log.fromPartial(e)) || [];
    message.mergeBuild = (object.mergeBuild !== undefined && object.mergeBuild !== null)
      ? Step_MergeBuild.fromPartial(object.mergeBuild)
      : undefined;
    message.summaryMarkdown = object.summaryMarkdown ?? "";
    message.tags = object.tags?.map((e) => StringPair.fromPartial(e)) || [];
    return message;
  },
};

function createBaseStep_MergeBuild(): Step_MergeBuild {
  return { fromLogdogStream: "", legacyGlobalNamespace: false, mergeOutputPropertiesTo: [] };
}

export const Step_MergeBuild: MessageFns<Step_MergeBuild> = {
  encode(message: Step_MergeBuild, writer: BinaryWriter = new BinaryWriter()): BinaryWriter {
    if (message.fromLogdogStream !== "") {
      writer.uint32(10).string(message.fromLogdogStream);
    }
    if (message.legacyGlobalNamespace !== false) {
      writer.uint32(16).bool(message.legacyGlobalNamespace);
    }
    for (const v of message.mergeOutputPropertiesTo) {
      writer.uint32(26).string(v!);
    }
    return writer;
  },

  decode(input: BinaryReader | Uint8Array, length?: number): Step_MergeBuild {
    const reader = input instanceof BinaryReader ? input : new BinaryReader(input);
    const end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseStep_MergeBuild() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1: {
          if (tag !== 10) {
            break;
          }

          message.fromLogdogStream = reader.string();
          continue;
        }
        case 2: {
          if (tag !== 16) {
            break;
          }

          message.legacyGlobalNamespace = reader.bool();
          continue;
        }
        case 3: {
          if (tag !== 26) {
            break;
          }

          message.mergeOutputPropertiesTo.push(reader.string());
          continue;
        }
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skip(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Step_MergeBuild {
    return {
      fromLogdogStream: isSet(object.fromLogdogStream) ? globalThis.String(object.fromLogdogStream) : "",
      legacyGlobalNamespace: isSet(object.legacyGlobalNamespace)
        ? globalThis.Boolean(object.legacyGlobalNamespace)
        : false,
      mergeOutputPropertiesTo: globalThis.Array.isArray(object?.mergeOutputPropertiesTo)
        ? object.mergeOutputPropertiesTo.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: Step_MergeBuild): unknown {
    const obj: any = {};
    if (message.fromLogdogStream !== "") {
      obj.fromLogdogStream = message.fromLogdogStream;
    }
    if (message.legacyGlobalNamespace !== false) {
      obj.legacyGlobalNamespace = message.legacyGlobalNamespace;
    }
    if (message.mergeOutputPropertiesTo?.length) {
      obj.mergeOutputPropertiesTo = message.mergeOutputPropertiesTo;
    }
    return obj;
  },

  create(base?: DeepPartial<Step_MergeBuild>): Step_MergeBuild {
    return Step_MergeBuild.fromPartial(base ?? {});
  },
  fromPartial(object: DeepPartial<Step_MergeBuild>): Step_MergeBuild {
    const message = createBaseStep_MergeBuild() as any;
    message.fromLogdogStream = object.fromLogdogStream ?? "";
    message.legacyGlobalNamespace = object.legacyGlobalNamespace ?? false;
    message.mergeOutputPropertiesTo = object.mergeOutputPropertiesTo?.map((e) => e) || [];
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

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

export interface MessageFns<T> {
  encode(message: T, writer?: BinaryWriter): BinaryWriter;
  decode(input: BinaryReader | Uint8Array, length?: number): T;
  fromJSON(object: any): T;
  toJSON(message: T): unknown;
  create(base?: DeepPartial<T>): T;
  fromPartial(object: DeepPartial<T>): T;
}
