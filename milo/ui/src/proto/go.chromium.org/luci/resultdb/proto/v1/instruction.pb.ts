/* eslint-disable */
import _m0 from "protobufjs/minimal";
import { StringPair } from "./common.pb";

export const protobufPackage = "luci.resultdb.v1";

export enum InstructionTarget {
  UNSPECIFIED = 0,
  /** LOCAL - For running in a local machine. */
  LOCAL = 1,
  /** REMOTE - For running remotely. */
  REMOTE = 2,
  /** PREBUILT - For prebuilt images. */
  PREBUILT = 3,
}

export function instructionTargetFromJSON(object: any): InstructionTarget {
  switch (object) {
    case 0:
    case "INSTRUCTION_TARGET_UNSPECIFIED":
      return InstructionTarget.UNSPECIFIED;
    case 1:
    case "LOCAL":
      return InstructionTarget.LOCAL;
    case 2:
    case "REMOTE":
      return InstructionTarget.REMOTE;
    case 3:
    case "PREBUILT":
      return InstructionTarget.PREBUILT;
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum InstructionTarget");
  }
}

export function instructionTargetToJSON(object: InstructionTarget): string {
  switch (object) {
    case InstructionTarget.UNSPECIFIED:
      return "INSTRUCTION_TARGET_UNSPECIFIED";
    case InstructionTarget.LOCAL:
      return "LOCAL";
    case InstructionTarget.REMOTE:
      return "REMOTE";
    case InstructionTarget.PREBUILT:
      return "PREBUILT";
    default:
      throw new globalThis.Error("Unrecognized enum value " + object + " for enum InstructionTarget");
  }
}

/**
 * A collection of instructions.
 * Used for step instruction.
 * This has a size limit of 1MB.
 */
export interface Instructions {
  readonly instructions: readonly Instruction[];
}

/**
 * Instruction is one failure reproduction instruction for a step or invocation.
 * Instruction can have different target, like "local" or "remote".
 * When converted to JSONPB format, it will look like below:
 * {
 *   "id" : "my_id",
 *   "targetedInstructions": [
 *     {
 *       "targets": [
 *         "LOCAL",
 *       ],
 *       "content": "my content",
 *       "dependency": [
 *         {
 *           "buildId": "80000",
 *           "stepName": "my step name",
 *           "stepTag": {
 *             "key": "my key",
 *             "value": "my value",
 *           },
 *         },
 *       ],
 *     },
 *   ],
 * }
 */
export interface Instruction {
  /**
   * ID of the instruction, used for step instruction.
   * It is consumer-defined and is unique within the build-level invocation.
   * For test instruction, we will ignore this field.
   * Included invocation may have the same instruction id with the parent invocation.
   */
  readonly id: string;
  /**
   * List of instruction for different targets.
   * There is at most 1 instruction per target.
   * If there is more than 1, an error will be returned.
   */
  readonly targetedInstructions: readonly TargetedInstruction[];
}

/**
 * Instruction for specific targets.
 * Instruction for different targets may have the same or different dependency
 * and content.
 */
export interface TargetedInstruction {
  /** The targets that this instruction is for, like "local", "remote" or "prebuilt" */
  readonly targets: readonly InstructionTarget[];
  /**
   * Another instruction that this instruction depends on.
   * At the moment, one instruction can have at most 1 dependency.
   * Make this repeated for forward compatibility.
   */
  readonly dependency: readonly InstructionDependency[];
  /**
   * The content of the instruction, in markdown format.
   * Placeholders may be used and will be populated with real
   * information when displayed in the UI.
   * This will be limit to 10KB. If the content is longer than 10KB,
   * an error will be returned.
   * See go/luci-failure-reproduction-instructions-dd for details.
   */
  readonly content: string;
}

/**
 * Specifies a dependency for instruction.
 * An instruction being depended on needs to be step instruction, not test result instruction.
 * If the dependency cannot be found, or the user does not have the ACL,
 * the dependency chain will stop and Milo will not display the dependency.
 * If a dependency cycle is detected, we will stop showing dependency once we detected the cycle.
 */
export interface InstructionDependency {
  /**
   * The build ID of the instruction being depended on.
   * This can be a build id or a templated string with placeholders.
   * Because test results instructions are stored in leaf invocation,
   * we can use placeholders to refer to the top-level build.
   * For example, "{{build_tags.parent_build_id}}" to refer to the parent build.
   * If not specified, assuming to be of the same build.
   * Limit: 100 bytes
   */
  readonly buildId: string;
  /**
   * The step name of the instruction being depended on.
   * If this is a nested step, this field should contain both
   * parent and child step names, separated by "|".
   * For example "parent_step_name|child_step_name".
   * Limit: 1024 bytes
   */
  readonly stepName: string;
  /**
   * Optional: In case there are more than one step with the same name
   * in the build, the step_tag is used to select the exact step to depend on.
   * This have the same size limit as step tag, 256 bytes for the key,
   * and 1024 bytes for the value.
   */
  readonly stepTag: StringPair | undefined;
}

function createBaseInstructions(): Instructions {
  return { instructions: [] };
}

export const Instructions = {
  encode(message: Instructions, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.instructions) {
      Instruction.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Instructions {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInstructions() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.instructions.push(Instruction.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Instructions {
    return {
      instructions: globalThis.Array.isArray(object?.instructions)
        ? object.instructions.map((e: any) => Instruction.fromJSON(e))
        : [],
    };
  },

  toJSON(message: Instructions): unknown {
    const obj: any = {};
    if (message.instructions?.length) {
      obj.instructions = message.instructions.map((e) => Instruction.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Instructions>, I>>(base?: I): Instructions {
    return Instructions.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Instructions>, I>>(object: I): Instructions {
    const message = createBaseInstructions() as any;
    message.instructions = object.instructions?.map((e) => Instruction.fromPartial(e)) || [];
    return message;
  },
};

function createBaseInstruction(): Instruction {
  return { id: "", targetedInstructions: [] };
}

export const Instruction = {
  encode(message: Instruction, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    for (const v of message.targetedInstructions) {
      TargetedInstruction.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Instruction {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInstruction() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.id = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.targetedInstructions.push(TargetedInstruction.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): Instruction {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : "",
      targetedInstructions: globalThis.Array.isArray(object?.targetedInstructions)
        ? object.targetedInstructions.map((e: any) => TargetedInstruction.fromJSON(e))
        : [],
    };
  },

  toJSON(message: Instruction): unknown {
    const obj: any = {};
    if (message.id !== "") {
      obj.id = message.id;
    }
    if (message.targetedInstructions?.length) {
      obj.targetedInstructions = message.targetedInstructions.map((e) => TargetedInstruction.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Instruction>, I>>(base?: I): Instruction {
    return Instruction.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<Instruction>, I>>(object: I): Instruction {
    const message = createBaseInstruction() as any;
    message.id = object.id ?? "";
    message.targetedInstructions = object.targetedInstructions?.map((e) => TargetedInstruction.fromPartial(e)) || [];
    return message;
  },
};

function createBaseTargetedInstruction(): TargetedInstruction {
  return { targets: [], dependency: [], content: "" };
}

export const TargetedInstruction = {
  encode(message: TargetedInstruction, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    writer.uint32(10).fork();
    for (const v of message.targets) {
      writer.int32(v);
    }
    writer.ldelim();
    for (const v of message.dependency) {
      InstructionDependency.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    if (message.content !== "") {
      writer.uint32(26).string(message.content);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TargetedInstruction {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseTargetedInstruction() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag === 8) {
            message.targets.push(reader.int32() as any);

            continue;
          }

          if (tag === 10) {
            const end2 = reader.uint32() + reader.pos;
            while (reader.pos < end2) {
              message.targets.push(reader.int32() as any);
            }

            continue;
          }

          break;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.dependency.push(InstructionDependency.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.content = reader.string();
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): TargetedInstruction {
    return {
      targets: globalThis.Array.isArray(object?.targets)
        ? object.targets.map((e: any) => instructionTargetFromJSON(e))
        : [],
      dependency: globalThis.Array.isArray(object?.dependency)
        ? object.dependency.map((e: any) => InstructionDependency.fromJSON(e))
        : [],
      content: isSet(object.content) ? globalThis.String(object.content) : "",
    };
  },

  toJSON(message: TargetedInstruction): unknown {
    const obj: any = {};
    if (message.targets?.length) {
      obj.targets = message.targets.map((e) => instructionTargetToJSON(e));
    }
    if (message.dependency?.length) {
      obj.dependency = message.dependency.map((e) => InstructionDependency.toJSON(e));
    }
    if (message.content !== "") {
      obj.content = message.content;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<TargetedInstruction>, I>>(base?: I): TargetedInstruction {
    return TargetedInstruction.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<TargetedInstruction>, I>>(object: I): TargetedInstruction {
    const message = createBaseTargetedInstruction() as any;
    message.targets = object.targets?.map((e) => e) || [];
    message.dependency = object.dependency?.map((e) => InstructionDependency.fromPartial(e)) || [];
    message.content = object.content ?? "";
    return message;
  },
};

function createBaseInstructionDependency(): InstructionDependency {
  return { buildId: "", stepName: "", stepTag: undefined };
}

export const InstructionDependency = {
  encode(message: InstructionDependency, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.buildId !== "") {
      writer.uint32(10).string(message.buildId);
    }
    if (message.stepName !== "") {
      writer.uint32(18).string(message.stepName);
    }
    if (message.stepTag !== undefined) {
      StringPair.encode(message.stepTag, writer.uint32(26).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): InstructionDependency {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseInstructionDependency() as any;
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.buildId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.stepName = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.stepTag = StringPair.decode(reader, reader.uint32());
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  fromJSON(object: any): InstructionDependency {
    return {
      buildId: isSet(object.buildId) ? globalThis.String(object.buildId) : "",
      stepName: isSet(object.stepName) ? globalThis.String(object.stepName) : "",
      stepTag: isSet(object.stepTag) ? StringPair.fromJSON(object.stepTag) : undefined,
    };
  },

  toJSON(message: InstructionDependency): unknown {
    const obj: any = {};
    if (message.buildId !== "") {
      obj.buildId = message.buildId;
    }
    if (message.stepName !== "") {
      obj.stepName = message.stepName;
    }
    if (message.stepTag !== undefined) {
      obj.stepTag = StringPair.toJSON(message.stepTag);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<InstructionDependency>, I>>(base?: I): InstructionDependency {
    return InstructionDependency.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<InstructionDependency>, I>>(object: I): InstructionDependency {
    const message = createBaseInstructionDependency() as any;
    message.buildId = object.buildId ?? "";
    message.stepName = object.stepName ?? "";
    message.stepTag = (object.stepTag !== undefined && object.stepTag !== null)
      ? StringPair.fromPartial(object.stepTag)
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
